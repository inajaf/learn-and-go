package productionpatterns

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDevelopmentLogger(t *testing.T) {
	logger := NewDevelopmentLogger()
	//👉 Just checking that he’s not panicking
	logger.Debug("test message")
	logger.Info("test message")
}

func TestNewProductionLogger(t *testing.T) {
	logger := NewProductionLogger()
	logger.Info("test message")
}

func TestNewServiceLogger_AddsFields(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))

	logger := NewServiceLogger(base, "order-svc", "1.2.3")
	logger.Info("test")

	output := buf.String()
	assert.Contains(t, output, "order-svc")
	assert.Contains(t, output, "1.2.3")
}

func TestRequestLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	//👉 Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"123"}`))
	})

	//Wrapping the middleware
	logged := RequestLoggingMiddleware(logger)(handler)

	//Making a test request
	req := httptest.NewRequest("POST", "/api/v1/orders", nil)
	rec := httptest.NewRecorder()

	logged.ServeHTTP(rec, req)

	//Check that the log contains the required fields
	output := buf.String()
	assert.Contains(t, output, "POST")
	assert.Contains(t, output, "/api/v1/orders")
	assert.Contains(t, output, "201")
	assert.Contains(t, output, "duration")
}

func TestRequestLoggingMiddleware_ErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	logged := RequestLoggingMiddleware(logger)(handler)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	logged.ServeHTTP(rec, req)

	output := buf.String()
	//👉 500th errors are logged at the ERROR level
	assert.Contains(t, output, "ERROR")
}

func TestLoggerFromContext_Default(t *testing.T) {
	//👉 Without logger in context - returns default
	logger := LoggerFromContext(context.Background())
	require.NotNil(t, logger)
}

func TestLoggerFromContext_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewJSONHandler(&buf, nil))

	ctx := WithLogger(context.Background(), custom)
	logger := LoggerFromContext(ctx)

	logger.Info("test from context")

	assert.True(t, strings.Contains(buf.String(), "test from context"))
}

func TestContextualLoggerMiddleware(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewJSONHandler(&buf, nil))

	var capturedLogger *slog.Logger

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//👉 Inside the handler we get a logger with request_id
		capturedLogger = LoggerFromContext(r.Context())
		capturedLogger.Info("order processing")
		w.WriteHeader(http.StatusOK)
	})

	//Add request ID and contextual logger
	withCtx := ContextualLoggerMiddleware(baseLogger)(handler)

	req := httptest.NewRequest("GET", "/api/v1/orders/123", nil)
	ctx := WithRequestID(req.Context(), "req-test-777")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	withCtx.ServeHTTP(rec, req)

	output := buf.String()
	assert.Contains(t, output, "req-test-777") //request_id forwarded
	assert.Contains(t, output, "order processing")
}
