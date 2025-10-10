package model

import (
	"time"
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
	Logger        func(string, ...interface{})
}
