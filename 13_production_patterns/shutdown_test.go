package productionpatterns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShutdownManager_ClosesInReverseOrder(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sm := NewShutdownManager(logger)

	var order []string

	sm.Register("database", func(ctx context.Context) error {
		order = append(order, "database")
		return nil
	})
	sm.Register("kafka", func(ctx context.Context) error {
		order = append(order, "kafka")
		return nil
	})
	sm.Register("http_server", func(ctx context.Context) error {
		order = append(order, "http_server")
		return nil
	})

	err := sm.Shutdown(context.Background())

	require.NoError(t, err)
	//👉 Closing in reverse order: http → kafka → database
	assert.Equal(t, []string{"http_server", "kafka", "database"}, order)
}

func TestShutdownManager_CollectsErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	sm := NewShutdownManager(logger)

	sm.Register("healthy", func(ctx context.Context) error {
		return nil
	})
	sm.Register("broken", func(ctx context.Context) error {
		return fmt.Errorf("I can't close the connection")
	})

	err := sm.Shutdown(context.Background())

	//👉 An error when closing does not stop the others from closing
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken")
}

func TestShutdownManager_RespectsContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	sm := NewShutdownManager(logger)

	var called atomic.Bool

	sm.Register("slow", func(ctx context.Context) error {
		select {
		case <-time.After(5 * time.Second):
			called.Store(true)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	//👉 We give only 50ms - the component will not have time
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := sm.Shutdown(ctx)
	require.Error(t, err)
}
