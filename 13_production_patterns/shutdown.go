package productionpatterns

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// =============================================================================
//Graceful Shutdown - graceful shutdown of the service
// =============================================================================
//
//Why: if the service is simply killed (kill -9), then:
//- Current HTTP requests are terminated (the client receives an error)
//- Transactions in the database remain uncommitted
//- Messages in the queue are lost
//- Files may be damaged
//
// Graceful shutdown:
//1. Stop accepting NEW requests
//2. Wait for the completion of CURRENT requests (with a timeout)
//3. Close connections (DB, Kafka, Redis)
//4. Exit with code 0
//
//Signals:
//SIGINT (Ctrl+C) - normal stop
//SIGTERM — Kubernetes/Docker sends before kill

//--- Components that need to be closed correctly -------------------------

//Closeable - interface for components with graceful shutdown.
//👉 In production: DB pool, Kafka producer, Redis client, gRPC server
type Closeable interface {
	Close() error
}

//--- GracefulServer - HTTP server with graceful completion ------------------

//GracefulServer wraps http.Server and manages the lifecycle.
type GracefulServer struct {
	httpServer *http.Server
	logger     *slog.Logger
	closeables []Closeable //DB, Kafka, Redis - everything that needs to be closed
	wg         sync.WaitGroup
}

//NewGracefulServer creates a server with graceful shutdown.
func NewGracefulServer(addr string, handler http.Handler, logger *slog.Logger) *GracefulServer {
	return &GracefulServer{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  15 * time.Second, //👉 Always set timeouts!
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		logger: logger,
	}
}

//RegisterCloseable adds a component that needs to be closed during shutdown.
func (s *GracefulServer) RegisterCloseable(c Closeable) {
	s.closeables = append(s.closeables, c)
}

//Run starts the server and blocks until it receives a stop signal.
//
//Typical usage in main():
//
//	func main() {
//	    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	    server := NewGracefulServer(":8080", mux, logger)
//	    server.RegisterCloseable(dbPool)
//	    server.RegisterCloseable(kafkaProducer)
//	    if err := server.Run(); err != nil {
//logger.Error("server failed with an error", slog.String("error", err.Error()))
//	        os.Exit(1)
//	    }
//	}
func (s *GracefulServer) Run() error {
	//👉 signal.NotifyContext - Go 1.16+, the cleanest way
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	//Starting an HTTP server in a goroutine
	errCh := make(chan error, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("HTTP server is running", slog.String("addr", s.httpServer.Addr))

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	//We are waiting: either a signal or a server error
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		s.logger.Info("I received a stop signal, I begin a graceful shutdown...")
	}

	return s.shutdown()
}

//shutdown shuts down all components gracefully.
func (s *GracefulServer) shutdown() error {
	//👉 We give 30 seconds to complete in-flight requests
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//1. Stop the HTTP server (stops accepting new ones, waits for current ones)
	s.logger.Info("stopping the HTTP server...")
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("HTTP server stop error", slog.String("error", err.Error()))
	}

	//2. Close all registered components
	for i := len(s.closeables) - 1; i >= 0; i-- {
		//👉 Closing in reverse order (LIFO)
		//If Kafka depends on DB, first close Kafka
		s.logger.Info("closing the component", slog.Int("index", i))
		if err := s.closeables[i].Close(); err != nil {
			s.logger.Error("component closing error",
				slog.Int("index", i),
				slog.String("error", err.Error()),
			)
		}
	}

	//3. Wait for all goroutines to complete
	s.wg.Wait()

	s.logger.Info("graceful shutdown completed")
	return nil
}

// =============================================================================
//Pattern: Shutdown Hook - coordination of several components
// =============================================================================

//ShutdownManager coordinates the graceful shutdown of multiple components.
//👉 In production, it is often used for complex systems with many dependencies.
type ShutdownManager struct {
	hooks  []shutdownHook
	logger *slog.Logger
	mu     sync.Mutex
}

type shutdownHook struct {
	name    string
	closeFn func(ctx context.Context) error
}

func NewShutdownManager(logger *slog.Logger) *ShutdownManager {
	return &ShutdownManager{logger: logger}
}

//Register adds a component for graceful completion.
func (sm *ShutdownManager) Register(name string, closeFn func(ctx context.Context) error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.hooks = append(sm.hooks, shutdownHook{name: name, closeFn: closeFn})
}

//Shutdown calls all hooks in reverse order with a timeout.
func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	sm.mu.Lock()
	hooks := make([]shutdownHook, len(sm.hooks))
	copy(hooks, sm.hooks)
	sm.mu.Unlock()

	var errs []error

	//👉 LIFO: last registered - first closed
	for i := len(hooks) - 1; i >= 0; i-- {
		hook := hooks[i]
		sm.logger.Info("closing the component", slog.String("name", hook.name))

		if err := hook.closeFn(ctx); err != nil {
			sm.logger.Error("closing error",
				slog.String("name", hook.name),
				slog.String("error", err.Error()),
			)
			errs = append(errs, fmt.Errorf("%s: %w", hook.name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	return nil
}
