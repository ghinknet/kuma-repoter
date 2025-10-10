package method

import (
	"context"
	"fmt"
	"git.ghink.net/ghink/kuma-repoter/internal/model"
	"github.com/go-ping/ping"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func reportWithRetry(ctx context.Context, cfg model.Config) error {
	var lastErr error

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			pingTime, err := getPingTime(cfg)
			if err != nil {
				Logger("ERROR", fmt.Errorf("ping failed (attempt %d/%d): %w", attempt, cfg.MaxRetries, err).Error())
				time.Sleep(cfg.RetryDelay)
				continue
			}

			if err = sendReport(cfg, pingTime); err != nil {
				Logger("ERROR", fmt.Errorf("report failed (attempt %d/%d): %w", attempt, cfg.MaxRetries, err))
				time.Sleep(cfg.RetryDelay)
				continue
			}

			Logger("INFO", fmt.Sprintf("Report successful! Ping: %.2f ms", pingTime))
			return nil
		}
	}

	return lastErr
}

func getPingTime(cfg model.Config) (float64, error) {
	ips, err := resolveIP(cfg.PingHost, cfg.UseIPv4, cfg.UseIPv6)
	if err != nil {
		err = fmt.Errorf("DNS resolution failed: %w", err)
		Logger("ERROR")
		return 0, err
	}

	if len(ips) == 0 {
		err = fmt.Errorf("no valid IP addresses found for %s", cfg.PingHost)
		Logger("ERROR", err)
		return 0, err
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
		Logger("ERROR", "Ping failed for ", ip, ": ", err, ", trying next IP")
	}

	return 0, lastErr
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
		err = fmt.Errorf("pinger creation failed: %w", err)
		Logger("ERROR", err)
		return 0, err
	}

	pinger.Count = count
	pinger.Timeout = timeout
	pinger.SetPrivileged(true)

	if err := pinger.Run(); err != nil {
		err = fmt.Errorf("ping failed: %w", err)
		Logger("ERROR", err)
		return 0, err
	}

	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		err = fmt.Errorf("no response from %s", ip)
		Logger("ERROR", err)
		return 0, err
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
		err = fmt.Errorf("system ping command failed: %w, output: %s", err, string(output))
		Logger("ERROR", err)
		return 0, err
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

	err := fmt.Errorf("could not parse ping output: %s", output)
	Logger("ERROR", err)
	return 0, err
}

func sendReport(cfg model.Config, pingTime float64) error {
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

	resp, err := client.Get(reportUrl.String())
	if err != nil {
		err = fmt.Errorf("HTTP request failed: %w", err)
		Logger("ERROR", err)
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status: %s", resp.Status)
		Logger("ERROR", err)
		return err
	}

	return nil
}
