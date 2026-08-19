package retry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDo_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 3

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestDo_RetryOnError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 3
	cfg.BaseDelay = 10 * time.Millisecond

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestDo_MaxAttemptsExceeded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 3
	cfg.BaseDelay = 10 * time.Millisecond

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		return errors.New("persistent error")
	})

	assert.Error(t, err)
	assert.Equal(t, 3, callCount)
}

func TestDo_NonRetryableError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 3
	cfg.BaseDelay = 10 * time.Millisecond
	cfg.RetryableErrors = []error{errors.New("retryable")}

	callCount := 0
	err := Do(context.Background(), cfg, func() error {
		callCount++
		return errors.New("non-retryable")
	})

	assert.Error(t, err)
	assert.Equal(t, 1, callCount)
}

func TestDo_ContextCancellation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 10
	cfg.BaseDelay = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, cfg, func() error {
		callCount++
		return errors.New("temporary error")
	})

	assert.Error(t, err)
	assert.True(t, callCount >= 1)
}

func TestCalculateDelay_Backoff(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 100 * time.Millisecond
	cfg.Multiplier = 2.0
	cfg.JitterFactor = 0

	d1 := calculateDelay(0, cfg)
	d2 := calculateDelay(1, cfg)
	d3 := calculateDelay(2, cfg)

	assert.Equal(t, 100*time.Millisecond, d1)
	assert.Equal(t, 200*time.Millisecond, d2)
	assert.Equal(t, 400*time.Millisecond, d3)
}

func TestCalculateDelay_MaxDelay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 100 * time.Millisecond
	cfg.Multiplier = 2.0
	cfg.MaxDelay = 250 * time.Millisecond
	cfg.JitterFactor = 0

	d := calculateDelay(10, cfg)
	assert.Equal(t, 250*time.Millisecond, d)
}

func TestIsStatusRetryable(t *testing.T) {
	cfg := DefaultConfig()

	assert.True(t, isStatusRetryable(http.StatusTooManyRequests, cfg))
	assert.True(t, isStatusRetryable(http.StatusServiceUnavailable, cfg))
	assert.False(t, isStatusRetryable(http.StatusNotFound, cfg))
	assert.False(t, isStatusRetryable(http.StatusBadRequest, cfg))
}

func TestDoWithRetryAfter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 3
	cfg.BaseDelay = 10 * time.Millisecond

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	callCount := 0
	err := DoWithRetryAfter(context.Background(), cfg, func() (*http.Response, error) {
		callCount++
		req, _ := http.NewRequest("GET", server.URL, nil)
		return http.DefaultClient.Do(req)
	})

	assert.Error(t, err)
	assert.Equal(t, 3, callCount)
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Retry-After": {"5"}}}
	d := parseRetryAfter(resp)
	assert.Equal(t, 5*time.Second, d)
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().Add(10 * time.Second).Format(http.TimeFormat)
	resp := &http.Response{Header: http.Header{"Retry-After": {future}}}
	d := parseRetryAfter(resp)
	assert.True(t, d > 0)
	assert.True(t, d <= 11*time.Second)
}