package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-ping/ping"
	"github.com/spf13/viper"
)

type Config struct {
	ReportURL     string
	PingHost      string
	ReportPeriod  time.Duration
	MaxRetries    int
	RetryDelay    time.Duration
	PingCount     int
	PingTimeout   time.Duration
	HTTPTimeout   time.Duration
	StatusMessage string
	UseIPv4       bool
	UseIPv6       bool
	UseSystemPing bool
}

func loadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")

	viper.SetDefault("ping_host", "oss-cn-beijing.aliyuncs.com")
	viper.SetDefault("report_period_seconds", 40)
	viper.SetDefault("max_retries", 3)
	viper.SetDefault("retry_delay_seconds", 5)
	viper.SetDefault("ping_count", 4)
	viper.SetDefault("ping_timeout_seconds", 10)
	viper.SetDefault("http_timeout_seconds", 15)
	viper.SetDefault("status_message", "OK")
	viper.SetDefault("use_ipv4", true)
	viper.SetDefault("use_ipv6", false)
	viper.SetDefault("use_system_ping", runtime.GOOS == "darwin")

	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			log.Printf("Config file not found, using defaults")
		}
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("UPTIME")

	return &Config{
		ReportURL:     viper.GetString("report_url"),
		PingHost:      viper.GetString("ping_host"),
		ReportPeriod:  time.Duration(viper.GetInt("report_period_seconds")) * time.Second,
		MaxRetries:    viper.GetInt("max_retries"),
		RetryDelay:    time.Duration(viper.GetInt("retry_delay_seconds")) * time.Second,
		PingCount:     viper.GetInt("ping_count"),
		PingTimeout:   time.Duration(viper.GetInt("ping_timeout_seconds")) * time.Second,
		HTTPTimeout:   time.Duration(viper.GetInt("http_timeout_seconds")) * time.Second,
		StatusMessage: viper.GetString("status_message"),
		UseIPv4:       viper.GetBool("use_ipv4"),
		UseIPv6:       viper.GetBool("use_ipv6"),
		UseSystemPing: viper.GetBool("use_system_ping"),
	}, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if cfg.ReportURL == "" {
		panic("Missing 'report_url'")
	}

	log.Printf("Uptime Kuma Reporter starting with configuration:")
	log.Printf("  Report URL: %s", cfg.ReportURL)
	log.Printf("  Ping Host: %s", cfg.PingHost)
	log.Printf("  Report Period: %v", cfg.ReportPeriod)
	log.Printf("  Max Retries: %d", cfg.MaxRetries)
	log.Printf("  Use IPv4: %v, Use IPv6: %v", cfg.UseIPv4, cfg.UseIPv6)
	log.Printf("  Use System Ping: %v", cfg.UseSystemPing)

	if cfg.UseSystemPing && runtime.GOOS == "darwin" {
		log.Println("macOS detected: Using system ping command to avoid permission issues")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	go func() {
		if err := reportWithRetry(ctx, cfg); err != nil {
			log.Printf("Initial report failed: %v", err)
		}
	}()

	ticker := time.NewTicker(cfg.ReportPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go func(c *Config) {
				if err := reportWithRetry(ctx, c); err != nil {
					log.Printf("Periodic report failure: %v", err)
				}
			}(cfg)
		case <-ctx.Done():
			log.Println("Service stopped")
			return
		}
	}
}

func reportWithRetry(ctx context.Context, cfg *Config) error {
	var lastErr error

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			pingTime, err := getPingTime(cfg)
			if err != nil {
				lastErr = fmt.Errorf("ping failed (attempt %d/%d): %w", attempt, cfg.MaxRetries, err)
				log.Println(lastErr)
				time.Sleep(cfg.RetryDelay)
				continue
			}

			if err := sendReport(cfg, pingTime); err != nil {
				lastErr = fmt.Errorf("report failed (attempt %d/%d): %w", attempt, cfg.MaxRetries, err)
				log.Println(lastErr)
				time.Sleep(cfg.RetryDelay)
				continue
			}

			log.Printf("Report successful! Ping: %.2f ms", pingTime)
			return nil
		}
	}

	return lastErr
}

func getPingTime(cfg *Config) (float64, error) {
	ips, err := resolveIP(cfg.PingHost, cfg.UseIPv4, cfg.UseIPv6)
	if err != nil {
		return 0, fmt.Errorf("DNS resolution failed: %w", err)
	}

	if len(ips) == 0 {
		return 0, fmt.Errorf("no valid IP addresses found for %s", cfg.PingHost)
	}

	var lastErr error
	for _, ip := range ips {
		var pingTime float64
		var err error

		if cfg.UseSystemPing {
			pingTime, err = pingWithSystem(ip, cfg.PingCount, cfg.PingTimeout)
		} else {
			pingTime, err = pingWithGoPing(ip, cfg.PingCount, cfg.PingTimeout)
		}

		if err == nil {
			return pingTime, nil
		}
		lastErr = err
		log.Printf("Ping failed for %s: %v, trying next IP", ip, err)
	}

	return 0, fmt.Errorf("all ping attempts failed: %w", lastErr)
}

func resolveIP(host string, useIPv4, useIPv6 bool) ([]string, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	var validIPs []string
	for _, ip := range ips {
		if useIPv4 && ip.To4() != nil {
			validIPs = append(validIPs, ip.String())
		} else if useIPv6 && ip.To4() == nil {
			validIPs = append(validIPs, ip.String())
		}
	}

	return validIPs, nil
}

func pingWithGoPing(ip string, count int, timeout time.Duration) (float64, error) {
	pinger, err := ping.NewPinger(ip)
	if err != nil {
		return 0, fmt.Errorf("pinger creation failed: %w", err)
	}

	pinger.Count = count
	pinger.Timeout = timeout
	pinger.SetPrivileged(true)

	if err := pinger.Run(); err != nil {
		return 0, fmt.Errorf("ping failed: %w", err)
	}

	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return 0, fmt.Errorf("no response from %s", ip)
	}

	return stats.AvgRtt.Seconds() * 1000, nil
}

func pingWithSystem(ip string, count int, timeout time.Duration) (float64, error) {
	cmdName := "ping"
	var args []string

	switch runtime.GOOS {
	case "darwin": // macOS
		args = []string{"-c", strconv.Itoa(count), "-t", strconv.Itoa(int(timeout.Seconds())), ip}
	case "windows":
		args = []string{"-n", strconv.Itoa(count), "-w", strconv.Itoa(int(timeout.Milliseconds())), ip}
	default: // Linux and other unix-like system
		args = []string{"-c", strconv.Itoa(count), "-W", strconv.Itoa(int(timeout.Seconds())), ip}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdName, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("system ping command failed: %w, output: %s", err, string(output))
	}

	return parseSystemPingOutput(string(output))
}

func parseSystemPingOutput(output string) (float64, error) {
	lines := strings.Split(output, "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]

		// "round-trip min/avg/max/stddev = 1.234/2.345/3.456/0.123 ms"
		if strings.Contains(line, "round-trip") || strings.Contains(line, "rtt") {
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, "/") {
					stats := strings.Split(part, "/")
					if len(stats) >= 4 {
						avg, err := strconv.ParseFloat(stats[1], 64)
						if err == nil {
							return avg, nil
						}
					}
				}
			}
		}

		// "Minimum = 1ms, Maximum = 2ms, Average = 3ms"
		if strings.Contains(line, "Average =") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "Average" && i+2 < len(parts) {
					avgStr := strings.TrimSuffix(parts[i+2], "ms")
					avg, err := strconv.ParseFloat(avgStr, 64)
					if err == nil {
						return avg, nil
					}
				}
			}
		}
	}

	return 0, fmt.Errorf("could not parse ping output: %s", output)
}

func sendReport(cfg *Config, pingTime float64) error {
	reportUrl, err := url.Parse(cfg.ReportURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	params := url.Values{}
	params.Add("status", "up")
	params.Add("msg", cfg.StatusMessage)
	params.Add("ping", fmt.Sprintf("%.2f", pingTime))
	reportUrl.RawQuery = params.Encode()

	client := &http.Client{
		Timeout: cfg.HTTPTimeout,
	}

	fmt.Println(reportUrl.String())
	resp, err := client.Get(reportUrl.String())
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	return nil
}
