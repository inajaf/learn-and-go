package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// =============================================================================
//Middleware - intermediate HTTP request processors
// =============================================================================
//
//Middleware in Go is a function:
//   func(next http.Handler) http.Handler
//
//It wraps the handler by adding BEFORE and/or AFTER logic.
//
//Middleware chain (execution order):
//
//   Request → RequestID → Logging → Recovery → Handler → Response
//                 1          2          3         4
//
//Each middleware can:
//- Modify the request (add header, context value)
//- Modify the answer (add header, change status)
//- Break the chain (return an error without calling next)

// --- Request ID Middleware ---------------------------------------------------

type requestIDKey struct{}

//RequestIDMiddleware generates a unique request ID for each request.
//
//👉 Request ID is used for:
//- Logging (all logs of one request are linked)
//- Tracing (you can find a request in Grafana/Jaeger)
//- Debugging (the client can send a request ID in a bug report)
func RequestIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//Take it from the header or generate a new one
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
			}

			//Place it in context (available to all handlers and services below)
			ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)

			//We return it to the response header (the client can use it for debugging)
			w.Header().Set("X-Request-ID", requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

//GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return "unknown"
}

// --- Request Logging Middleware -----------------------------------------------

//statusRecorder intercepts the status code of the response.
//👉 The standard http.ResponseWriter does not allow you to find out the recorded status.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

//RequestLoggingMiddleware logs every HTTP request.
//
//Outputs: method, path, status code, duration, request ID.
//
//Log example:
//
//	{
//"msg": "HTTP request",
//	  "method": "POST", "path": "/api/v1/orders",
//	  "status": 201, "duration": "15.2ms",
//	  "request_id": "req-abc-123"
//	}
func RequestLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sr := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(sr, r)

			duration := time.Since(start)
			level := slog.LevelInfo
			if sr.statusCode >= 500 {
				level = slog.LevelError
			} else if sr.statusCode >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(r.Context(), level, "HTTP request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sr.statusCode),
				slog.Duration("duration", duration),
				slog.String("request_id", GetRequestID(r.Context())),
			)
		})
	}
}

// --- Recovery Middleware (panic → 500) ----------------------------------------

//RecoveryMiddleware intercepts panic and returns 500.
//
//👉 Without this middleware, one panic crashes the ENTIRE server.
//With it, only one request receives 500, the rest continue to work.
//
//In production, panic is a bug. Recovery is needed to:
//1. Don’t crash the entire server because of one request
//2. Log stack trace for debugging
//3. Return a clear error to the client
func RecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					//👉 Log panic with stack trace
					logger.ErrorContext(r.Context(), "PANIC restored",
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
						slog.String("request_id", GetRequestID(r.Context())),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
					)

					writeError(w, &APIError{
						Code:    http.StatusInternalServerError,
						Message: "Internal Server Error",
					})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// --- CORS Middleware ----------------------------------------------------------

//CORSMiddleware adds CORS headers.
//
//👉 CORS is needed when the frontend (localhost:3000) accesses the API (localhost:8080).
//Without CORS headers, the browser blocks the request.
func CORSMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigins)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")

			//👉 Preflight request (OPTIONS) - respond immediately, do not call handler
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

//--- Auth Middleware (example) ------------------------------------------------

//AuthMiddleware checks the Bearer token in the Authorization header.
//
//👉 In production: JWT validation, OIDC, API key - depends on the project.
//This shows a pattern, not a specific implementation.
func AuthMiddleware(validateToken func(token string) (userID string, err error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			//👉 Checking the format "Bearer <token>"
			if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
				writeError(w, &APIError{
					Code:    http.StatusUnauthorized,
					Message: "authorization required",
					Details: []string{"header Authorization: Bearer <token>"},
				})
				return
			}

			token := authHeader[7:]
			userID, err := validateToken(token)
			if err != nil {
				writeError(w, &APIError{
					Code:    http.StatusUnauthorized,
					Message: "invalid token",
				})
				return
			}

			//👉 We put userID in the context - available in handlers
			ctx := context.WithValue(r.Context(), "user_id", userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
