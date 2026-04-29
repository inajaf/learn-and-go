package productionpatterns

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// =============================================================================
//Structured Logging with log/slog (Go 1.21+)
// =============================================================================
//
//Why NOT fmt.Println/log.Printf:
//- Cannot parse (Elasticsearch, Grafana Loki do not understand text)
//- No levels (DEBUG, INFO, WARN, ERROR)
//- No structured fields (request_id, user_id, duration)
//- No contextual meanings
//
//slog is the standard Go library since 1.21. Previously we used:
//- zerolog (fastest, zero-allocation)
//- zap (Uber, popular)
//- logrus (outdated, not recommended)
//
//👉 Recommendation: use slog in new projects, zerolog if needed
//maximum performance.

//--- Setting up a logger for different environments ---------------------------------

//NewDevelopmentLogger - for local development: text, readable, DEBUG.
func NewDevelopmentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true, //👉 Shows file:string - convenient for debugging
	}))
}

//NewProductionLogger - for production: JSON, INFO, without source.
//
//👉 JSON is needed because:
//- Elasticsearch/Loki parses it automatically
//- Grafana builds dashboards by field
//- You can filter: level=ERROR AND service=order-svc
func NewProductionLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

//--- slog.With - adding persistent fields --------------------------------
//
//👉 slog.With creates a NEW logger with additional fields.
//Use for fields that do not change within the scope:
//- service: "order-svc" (application level)
//- request_id: "abc-123" (at the request level)
//- order_id: "ord-456" (at the operation level)

//NewServiceLogger creates a logger for a specific service.
func NewServiceLogger(base *slog.Logger, serviceName, version string) *slog.Logger {
	return base.With(
		slog.String("service", serviceName),
		slog.String("version", version),
	)
}

//--- Example: logging a business transaction ------------------------------------

//LoggedOrderCreation demonstrates proper logging.
func LoggedOrderCreation(logger *slog.Logger) {
	//👉 DEBUG - for the developer, will not end up in production
	logger.Debug("I'm starting to create an order",
		slog.String("customer_id", "cust-123"),
	)

	//👉 INFO - key business events
	logger.Info("order created",
		slog.String("order_id", "ord-456"),
		slog.String("customer_id", "cust-123"),
		slog.Float64("total", 99.99),
		slog.Int("item_count", 3),
		slog.Duration("processing_time", 150*time.Millisecond),
	)

	//👉 WARN - something suspicious, but not an error
	logger.Warn("high delay when checking drain",
		slog.Duration("duration", 2*time.Second),
		slog.String("threshold", "1s"),
	)

	//👉 ERROR - error, reaction needed
	logger.Error("failed to send email",
		slog.String("order_id", "ord-456"),
		slog.String("error", "SMTP timeout"),
		slog.Int("retry_count", 3),
	)
}

// =============================================================================
// Middleware: Request Logging
// =============================================================================
//
//👉 Every production project has middleware for logging requests.
//Logs: method, path, status code, duration, request ID.

//statusWriter wraps http.ResponseWriter to intercept the status code.
//👉 The standard http.ResponseWriter does not let you know what code was written.
type statusWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.written {
		sw.statusCode = code
		sw.written = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if !sw.written {
		sw.statusCode = http.StatusOK
		sw.written = true
	}
	return sw.ResponseWriter.Write(b)
}

//RequestLoggingMiddleware logs every HTTP request.
//
//Example output (JSON):
//
//	{
//	  "level": "INFO",
//"msg": "HTTP request",
//	  "method": "POST",
//	  "path": "/api/v1/orders",
//	  "status": 201,
//	  "duration": "12.3ms",
//	  "request_id": "req-abc-123"
//	}
func RequestLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			//👉 We wrap writer to intercept the status code
			sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}

			//Call the next handler
			next.ServeHTTP(sw, r)

			//👉 We log AFTER processing - we know the status and duration
			duration := time.Since(start)

			//Selecting a level by status code
			logFn := logger.InfoContext
			if sw.statusCode >= 500 {
				logFn = logger.ErrorContext
			} else if sw.statusCode >= 400 {
				logFn = logger.WarnContext
			}

			logFn(r.Context(), "HTTP request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.statusCode),
				slog.Duration("duration", duration),
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// =============================================================================
//Pattern: Contextual Logger
// =============================================================================
//
//👉 Instead of passing request_id to each logger call, we create
//“enriched” logger at the middleware level and transmitted via context.

type loggerKey struct{}

//WithLogger puts the logger into context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

//LoggerFromContext Retrieves the logger from the context.
//If not, returns the default one (never nil).
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

//ContextualLoggerMiddleware adds a rich logger to the request context.
//
//👉 Now any handler/service can receive a logger with request_id:
//	   logger := LoggerFromContext(r.Context())
//logger.Info("operation completed") // automatically includes request_id
func ContextualLoggerMiddleware(baseLogger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//We enrich the logger with fields from the request
			requestLogger := baseLogger.With(
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)

			//Putting it in context
			ctx := WithLogger(r.Context(), requestLogger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
