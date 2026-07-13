package link

import (
	"time"

	"github.com/linkmessengerme/console-sdk-go/internal/transport"
)

type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func defaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
	}
}

func (c RetryConfig) toTransport() transport.RetryConfig {
	if c.InitialBackoff <= 0 {
		c.InitialBackoff = 100 * time.Millisecond
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 2 * time.Second
	}
	return transport.RetryConfig{
		MaxRetries:     c.MaxRetries,
		InitialBackoff: c.InitialBackoff,
		MaxBackoff:     c.MaxBackoff,
	}
}
