package server

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// =============================================================================
// gRPC Interceptors — the equivalent of HTTP middleware
// =============================================================================
//
// gRPC Interceptor = a function that wraps an RPC call.
// HTTP analogy:
//   HTTP middleware    → gRPC interceptor
//   func(http.Handler) http.Handler → grpc.UnaryServerInterceptor
//
// Two types:
//   - Unary interceptor:  for regular RPCs (request → response)
//   - Stream interceptor: for streaming RPCs (a stream of messages)
//
// Interceptor chain:
//   grpc.ChainUnaryInterceptor(recovery, logging, auth)
//   Order: recovery → logging → auth → handler → auth → logging → recovery
//
// 🏭 In production: grpc-ecosystem/go-grpc-middleware — ready-made interceptors.

// --- Logging Interceptor ---------------------------------------------------

// LoggingInterceptor logs every RPC call: method, duration, response code.
//
// Usage:
//
//	srv := grpc.NewServer(
//	    grpc.ChainUnaryInterceptor(LoggingInterceptor(logger)),
//	)
func LoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()

		// Extract the request-id from metadata (HTTP headers analog)
		requestID := extractRequestID(ctx)

		// Call the next handler/interceptor
		resp, err := handler(ctx, req)

		duration := time.Since(start)
		code := status.Code(err)

		// Log the result
		attrs := []any{
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
			slog.String("code", code.String()),
		}
		if requestID != "" {
			attrs = append(attrs, slog.String("request_id", requestID))
		}
		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
			logger.Error("gRPC call failed", attrs...)
		} else {
			logger.Info("gRPC call", attrs...)
		}

		return resp, err
	}
}

// extractRequestID extracts the request-id from gRPC metadata.
// 👉 gRPC metadata = the HTTP headers equivalent.
func extractRequestID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-request-id")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// --- Recovery Interceptor --------------------------------------------------

// RecoveryInterceptor catches panics in handlers and returns an Internal error.
//
// Without recovery: a panic in a handler → the gRPC server crashes → all clients disconnect.
// With recovery: panic → Internal error → the server keeps running.
//
// ❌ A common mistake: not installing a recovery interceptor.
//    One panic in one handler takes down the whole server.
func RecoveryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("gRPC panic recovered",
					slog.String("method", info.FullMethod),
					slog.Any("panic", r),
					slog.String("stack", string(debug.Stack())),
				)
				err = status.Errorf(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// --- Auth Interceptor ------------------------------------------------------

// AuthFunc — token verification function. Returns nil if the token is valid.
type AuthFunc func(ctx context.Context, token string) error

// AuthInterceptor verifies the Bearer token from gRPC metadata.
//
// 👉 In gRPC the token travels through metadata (HTTP Authorization header analog):
//
//	md := metadata.Pairs("authorization", "Bearer <token>")
//	ctx := metadata.NewOutgoingContext(ctx, md)
//
// Usage:
//
//	authFn := func(ctx context.Context, token string) error {
//	    if token != "valid-token" {
//	        return errors.New("invalid token")
//	    }
//	    return nil
//	}
//	srv := grpc.NewServer(
//	    grpc.ChainUnaryInterceptor(AuthInterceptor(authFn)),
//	)
func AuthInterceptor(authFn AuthFunc) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		token, err := extractBearerToken(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		if err := authFn(ctx, token); err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "auth failed: %v", err)
		}

		return handler(ctx, req)
	}
}

// extractBearerToken extracts the Bearer token from metadata.
func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "no authorization header")
	}

	token := values[0]
	const prefix = "Bearer "
	if len(token) < len(prefix) || token[:len(prefix)] != prefix {
		return "", status.Error(codes.Unauthenticated, "invalid authorization format, expected 'Bearer <token>'")
	}

	return token[len(prefix):], nil
}
