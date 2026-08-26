package retry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	cfg.RetryableErrors = []error{errors.New("non-retryable")}

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
	future := time.Now().UTC().Add(10 * time.Second)
	resp := &http.Response{Header: http.Header{"Retry-After": {future.Format(http.TimeFormat)}}}
	d := parseRetryAfter(resp)
	assert.True(t, d > 0)
	assert.True(t, d <= 11*time.Second)
}

func TestDoWithRetryAfter_Seconds(t *testing.T) {
	cfg := DefaultConfig()
	attempts := 0
	err := DoWithRetryAfter(context.Background(), cfg, func() (*http.Response, error) {
		attempts++
		if attempts < 2 {
			return &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": []string{"0"}}}, nil
		}
		return &http.Response{StatusCode: 200}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestDoWithRetryAfter_HTTPDate(t *testing.T) {
	cfg := Config{MaxAttempts: 2, BaseDelay: time.Millisecond, RetryableStatus: []int{503}}
	attempts := 0
	err := DoWithRetryAfter(context.Background(), cfg, func() (*http.Response, error) {
		attempts++
		if attempts < 2 {
			h := http.Header{"Retry-After": []string{time.Now().UTC().Add(time.Second).Format(http.TimeFormat)}}
			return &http.Response{StatusCode: 503, Header: h}, nil
		}
		return &http.Response{StatusCode: 200}, nil
	})
	assert.NoError(t, err)
}

func TestDoWithRetryAfter_NonRetryable(t *testing.T) {
	cfg := DefaultConfig()
	attempts := 0
	err := DoWithRetryAfter(context.Background(), cfg, func() (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: 404}, nil
	})
	assert.Error(t, err)
	assert.Equal(t, 1, attempts)
}

func TestDoWithRetryAfter_TransportError(t *testing.T) {
	cfg := Config{MaxAttempts: 1, BaseDelay: time.Millisecond}
	err := DoWithRetryAfter(context.Background(), cfg, func() (*http.Response, error) {
		return nil, errors.New("boom")
	})
	assert.Error(t, err)
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	r := &http.Response{Header: http.Header{"Retry-After": []string{"garbage"}}}
	assert.Equal(t, time.Duration(0), parseRetryAfter(r))
	r2 := &http.Response{Header: http.Header{}}
	assert.Equal(t, time.Duration(0), parseRetryAfter(r2))
}

func TestIsErrorRetryable_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := DefaultConfig()
	assert.False(t, isRetryable(ctx.Err(), cfg))
	assert.True(t, isRetryable(errors.New("other"), cfg))
}

func TestStatusErrorAndHelpers(t *testing.T) {
	e := &statusError{status: 429}
	assert.Equal(t, "HTTP status 429", e.Error())
	assert.True(t, isStatusRetryable(429, DefaultConfig()))
	assert.False(t, isStatusRetryable(404, DefaultConfig()))

	// isErrorRetryable délègue à isRetryable
	assert.True(t, isErrorRetryable(errors.New("x"), DefaultConfig()))

	// RetryableFunc.Do + WithConfig
	called := 0
	fn := RetryableFunc(func() error { called++; return nil })
	assert.NoError(t, fn.Do(context.Background(), DefaultConfig()))
	assert.Equal(t, 1, called)

	var got int
	wc := WithConfig(DefaultConfig())
	_ = wc() // retourne toujours nil
	_ = got

	// Do avec erreur non retryable via RetryableErrors
	cfg2 := DefaultConfig()
	cfg2.RetryableErrors = []error{errors.New("fatal")}
	n := 0
	err := Do(context.Background(), cfg2, func() error { n++; return errors.New("fatal") })
	assert.Error(t, err)
	assert.Equal(t, 1, n)

	// Do : épuise les tentatives sur erreur retryable
	cfg3 := Config{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 1, JitterFactor: 0}
	m := 0
	err = Do(context.Background(), cfg3, func() error { m++; return errors.New("transient") })
	assert.Error(t, err)
	assert.Equal(t, 2, m)

	// Do : ctx annulé pendant l'attente
	ctx, cancel := context.WithCancel(context.Background())
	cfg4 := Config{MaxAttempts: 3, BaseDelay: time.Hour, Multiplier: 1, JitterFactor: 0}
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	start := time.Now()
	_ = Do(ctx, cfg4, func() error { return errors.New("retry me") })
	assert.Less(t, time.Since(start), time.Minute)
}
