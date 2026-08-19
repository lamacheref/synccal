package retry

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"time"
)

type Config struct {
	MaxAttempts      int
	BaseDelay        time.Duration
	MaxDelay         time.Duration
	Multiplier       float64
	JitterFactor     float64
	RetryableStatus  []int
	RetryableErrors  []error
}

func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		BaseDelay:    500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.2,
		RetryableStatus: []int{
			http.StatusRequestTimeout,      // 408
			http.StatusTooManyRequests,     // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
		},
	}
}

func Do(ctx context.Context, cfg Config, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if !isRetryable(err, cfg) {
			return err
		}

		if attempt == cfg.MaxAttempts-1 {
			break
		}

		delay := calculateDelay(attempt, cfg)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

func DoWithRetryAfter(ctx context.Context, cfg Config, fn func() (*http.Response, error)) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		resp, err := fn()
		if err == nil {
			if resp != nil && isStatusRetryable(resp.StatusCode, cfg) {
				retryAfter := parseRetryAfter(resp)
				if retryAfter > 0 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(retryAfter):
						continue
					}
				}
			}
			if resp != nil && resp.StatusCode < 400 {
				return nil
			}
			lastErr = err
		} else {
			lastErr = err
			if !isErrorRetryable(err, cfg) {
				return err
			}
		}

		if attempt == cfg.MaxAttempts-1 {
			break
		}

		delay := calculateDelay(attempt, cfg)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

func calculateDelay(attempt int, cfg Config) time.Duration {
	delay := float64(cfg.BaseDelay) * math.Pow(cfg.Multiplier, float64(attempt))
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}

	jitter := delay * cfg.JitterFactor * (rand.Float64()*2 - 1)
	delay += jitter

	if delay < 0 {
		delay = 0
	}
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}

	return time.Duration(delay)
}

func isRetryable(err error, cfg Config) bool {
	if isErrorRetryable(err, cfg) {
		return true
	}
	return false
}

func isErrorRetryable(err error, cfg Config) bool {
	for _, re := range cfg.RetryableErrors {
		if err == re {
			return true
		}
	}
	return false
}

func isStatusRetryable(status int, cfg Config) bool {
	for _, s := range cfg.RetryableStatus {
		if status == s {
			return true
		}
	}
	return false
}

func parseRetryAfter(resp *http.Response) time.Duration {
	ra := resp.Header.Get("Retry-After")
	if ra == "" {
		return 0
	}

	if seconds, err := time.ParseDuration(ra + "s"); err == nil {
		return seconds
	}

	if t, err := http.ParseTime(ra); err == nil {
		return time.Until(t)
	}

	return 0
}

type RetryableFunc func() error

func (f RetryableFunc) Do(ctx context.Context, cfg Config) error {
	return Do(ctx, cfg, f)
}

func WithConfig(cfg Config) RetryableFunc {
	return func() error {
		return nil
	}
}