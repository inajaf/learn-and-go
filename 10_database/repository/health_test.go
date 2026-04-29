package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"learning_path/10_database/repository"
)

// =============================================================================
//Health Check tests (without a real database - via Pinger mock)
// =============================================================================

//mockPinger - mock for health check testing.
type mockPinger struct {
	err   error
	delay time.Duration
}

func (m *mockPinger) PingContext(ctx context.Context) error {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.err
}

func TestHealthChecker_Healthy(t *testing.T) {
	pinger := &mockPinger{err: nil}
	hc := repository.NewHealthChecker(pinger, 2*time.Second)

	status := hc.Check(context.Background())

	assert.True(t, status.Healthy)
	assert.Empty(t, status.Error)
	assert.Greater(t, status.Latency, time.Duration(0))
	assert.False(t, status.CheckedAt.IsZero())
}

func TestHealthChecker_Unhealthy(t *testing.T) {
	pinger := &mockPinger{err: errors.New("connection refused")}
	hc := repository.NewHealthChecker(pinger, 2*time.Second)

	status := hc.Check(context.Background())

	assert.False(t, status.Healthy)
	assert.Contains(t, status.Error, "connection refused")
}

func TestHealthChecker_Timeout(t *testing.T) {
	//The database responds in 5 seconds, but the timeout is 50ms
	pinger := &mockPinger{delay: 5 * time.Second}
	hc := repository.NewHealthChecker(pinger, 50*time.Millisecond)

	status := hc.Check(context.Background())

	assert.False(t, status.Healthy)
	assert.Contains(t, status.Error, "context deadline exceeded")
}

func TestDefaultPoolConfig(t *testing.T) {
	cfg := repository.DefaultPoolConfig()

	require.Greater(t, cfg.MaxOpenConns, 0)
	require.Greater(t, cfg.MaxIdleConns, 0)
	assert.LessOrEqual(t, cfg.MaxIdleConns, cfg.MaxOpenConns,
		"MaxIdleConns must be ≤ MaxOpenConns")
	assert.Greater(t, cfg.ConnMaxLifetime, time.Duration(0))
	assert.Greater(t, cfg.ConnMaxIdleTime, time.Duration(0))
}
