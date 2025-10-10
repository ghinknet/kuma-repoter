package main

import (
	"context"
	"errors"
	kumaRepoter "git.ghink.net/ghink/kuma-repoter"
	"git.ghink.net/ghink/kuma-repoter/internal/method"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/viper"
)

func loadConfig() (kumaRepoter.Config, error) {
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
			method.DefaultLogger("WARN", "Config file not found, using defaults")
		}
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("UPTIME")

	return kumaRepoter.Config{
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
		method.DefaultLogger("FATAL", "Failed to load configuration: %v", err)
		panic(err)
	}

	if cfg.ReportURL == "" {
		method.DefaultLogger("FATAL", "Missing 'report_url'")
		panic("Missing 'report_url'")
	}

	method.DefaultLogger("INFO", "Uptime Kuma Reporter starting with configuration:")
	method.DefaultLogger("INFO", "  Report URL: ", cfg.ReportURL)
	method.DefaultLogger("INFO", "  Ping Host: ", cfg.PingHost)
	method.DefaultLogger("INFO", "  Report Period: ", cfg.ReportPeriod)
	method.DefaultLogger("INFO", "  Max Retries: ", cfg.MaxRetries)
	method.DefaultLogger("INFO", "  Use IPv4: ", cfg.UseIPv4, ", Use IPv6: ", cfg.UseIPv6)
	method.DefaultLogger("INFO", "  Use System Ping: ", cfg.UseSystemPing)

	if cfg.UseSystemPing && runtime.GOOS == "darwin" {
		method.DefaultLogger("WARN", "macOS detected: Using system ping command to avoid permission issues")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		method.DefaultLogger("INFO", "Shutting down...")
		cancel()
	}()

	kumaRepoter.Daemon(ctx, cfg)
}
