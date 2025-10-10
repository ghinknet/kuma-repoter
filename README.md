# kuma-repoter

Report status to uptime kuma

Why "repoter"? ...hummm, it's just a typo at first.

But I decided to keep it. :D

### Usage Example

1. Download a binary executable file in release for your architecture and system

 - Oversea:
```
cd /mnt
mkdir services
cd services
mkdir kuma-reporter
cd kuma-reporter
apt install wget -y
wget -O main https://github.com/ghinknet/kuma-repoter/releases/download/v1.0.0/kuma-reporter-linux-amd64
wget -O config.json https://github.com/ghinknet/kuma-repoter/releases/download/v1.0.0/config.json
chmod +x main
cd
```
 - Chinese Mainland: 
```
cd /mnt
mkdir services
cd services
mkdir kuma-reporter
cd kuma-reporter
apt install wget -y
wget -O main https://git.ghink.net/ghink/kuma-repoter/releases/download/v1.0.0/kuma-reporter-linux-amd64
wget -O config.json https://git.ghink.net/ghink/kuma-repoter/releases/download/v1.0.0/config.json
chmod +x main
cd
```

2. Create system deamon file
```
echo '[Unit]
Description=Kuma Reporter
After=network.target

[Service]
ExecStart=/mnt/services/kuma-reporter/main
Restart=always
User=
WorkingDirectory=/mnt/services/kuma-reporter
Environment=ENVIRONMENT=production

[Install]
WantedBy=multi-user.target' >> /etc/systemd/system/kuma-reporter.service
```

3. Edit the configuration file
```
vim /mnt/services/kuma-reporter/config.json
# :wq rip
# Or nano
# nano /mnt/services/kuma-reporter/config.json
```

4. Enable and start the daemon
```
systemctl start kuma-reporter
systemctl enable kuma-reporter
```

### Usage as Lib Example

1. Get kuma-repoter
```
go get git.ghink.net/ghink/kuma-repoter
```

2. Start the daemon
```
func InitKuma() {
	kumaConfig := kumaRepoter.Config{
		ReportURL:     config.C.GetString("uptime.kuma.report_url"),
		PingHost:      config.C.GetString("uptime.kuma.ping_host"),
		ReportPeriod:  time.Duration(config.C.GetInt("uptime.kuma.report_period_seconds")) * time.Second,
		MaxRetries:    config.C.GetInt("uptime.kuma.max_retries"),
		RetryDelay:    time.Duration(config.C.GetInt("uptime.kuma.retry_delay_seconds")) * time.Second,
		PingCount:     config.C.GetInt("uptime.kuma.ping_count"),
		PingTimeout:   time.Duration(config.C.GetInt("uptime.kuma.ping_timeout_seconds")) * time.Second,
		HTTPTimeout:   time.Duration(config.C.GetInt("uptime.kuma.http_timeout_seconds")) * time.Second,
		StatusMessage: config.C.GetString("uptime.kuma.status_message"),
		UseIPv4:       config.C.GetBool("uptime.kuma.use_ipv4"),
		UseIPv6:       config.C.GetBool("uptime.kuma.use_ipv6"),
		UseSystemPing: config.C.GetBool("uptime.kuma.use_system_ping"),
	}

	go kumaRepoter.Daemon(context.Background(), kumaConfig)
}
```

### Support Platforms

 - aix-ppc64
 - darwin-amd64
 - darwin-arm64
 - dragonfly-amd64
 - freebsd-386
 - freebsd-amd64
 - freebsd-arm
 - freebsd-arm64
 - freebsd-riscv64
 - illumos-amd64
 - linux-386
 - linux-amd64
 - linux-arm
 - linux-arm64
 - linux-loong64
 - linux-mips
 - linux-mips64
 - linux-mips64le
 - linux-mipsle
 - linux-ppc64
 - linux-ppc64le
 - linux-riscv64
 - linux-s390x
 - netbsd-386
 - netbsd-amd64
 - netbsd-arm
 - netbsd-arm64
 - openbsd-386
 - openbsd-amd64
 - openbsd-arm
 - openbsd-arm64
 - openbsd-ppc64
 - openbsd-riscv64
 - solaris-amd64
 - windows-386
 - windows-amd64
 - windows-arm
 - windows-arm64