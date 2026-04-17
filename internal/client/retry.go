package client

import (
	"context"
	"time"
)

const (
	DefaultMaxRetries = 3
	DefaultBaseDelay  = time.Second
	DefaultMaxDelay   = 30 * time.Second
)

// RetryConfig configures exponential backoff retries.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// Retry executes operation and retries retryable failures with exponential backoff.
func Retry(ctx context.Context, cfg RetryConfig, operation func(context.Context) (bool, error)) error {
	cfg = cfg.withDefaults()

	var lastErr error
	for attempt := 0; ; attempt++ {
		retryable, err := operation(ctx)
		if err == nil {
			return nil
		}

		lastErr = err
		if !retryable || attempt >= cfg.MaxRetries {
			return lastErr
		}

		delay := cfg.backoff(attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c RetryConfig) withDefaults() RetryConfig {
	if c.MaxRetries <= 0 {
		c.MaxRetries = DefaultMaxRetries
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = DefaultBaseDelay
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = DefaultMaxDelay
	}
	return c
}

func (c RetryConfig) backoff(attempt int) time.Duration {
	delay := c.BaseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= c.MaxDelay {
			return c.MaxDelay
		}
	}
	if delay > c.MaxDelay {
		return c.MaxDelay
	}
	return delay
}
