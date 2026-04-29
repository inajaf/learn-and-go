package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	orderpb "learning_path/03_grpc/gen"
	grpcserver "learning_path/03_grpc/server"
)

// =============================================================================
//Interceptors tests via bufconn (real gRPC stack)
// =============================================================================

func TestLoggingInterceptor_LogsSuccessfulCall(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcserver.LoggingInterceptor(logger)),
	)
	orderpb.RegisterOrderServiceServer(srv, grpcserver.NewOrderServer())

	client := startInterceptorTestServer(t, srv)

	resp, err := client.CreateOrder(context.Background(), &orderpb.CreateOrderRequest{
		CustomerID: "cust-1",
		Amount:     100,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
}

func TestLoggingInterceptor_WithRequestID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcserver.LoggingInterceptor(logger)),
	)
	orderpb.RegisterOrderServiceServer(srv, grpcserver.NewOrderServer())

	client := startInterceptorTestServer(t, srv)

	//Sending request-id via metadata
	md := metadata.Pairs("x-request-id", "req-abc-123")
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	resp, err := client.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		CustomerID: "cust-1",
		Amount:     50,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
}

func TestRecoveryInterceptor_HandlesPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcserver.RecoveryInterceptor(logger)),
	)
	orderpb.RegisterOrderServiceServer(srv, &panicServer{})

	client := startInterceptorTestServer(t, srv)

	//👉 Instead of a server crash, we get an Internal error
	_, err := client.CreateOrder(context.Background(), &orderpb.CreateOrderRequest{
		CustomerID: "cust-1",
		Amount:     100,
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

func TestAuthInterceptor_ValidToken(t *testing.T) {
	authFn := func(ctx context.Context, token string) error {
		if token == "valid-secret" {
			return nil
		}
		return errors.New("bad token")
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcserver.AuthInterceptor(authFn)),
	)
	orderpb.RegisterOrderServiceServer(srv, grpcserver.NewOrderServer())

	client := startInterceptorTestServer(t, srv)

	md := metadata.Pairs("authorization", "Bearer valid-secret")
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	resp, err := client.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		CustomerID: "cust-1",
		Amount:     100,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
}

func TestAuthInterceptor_MissingToken(t *testing.T) {
	authFn := func(ctx context.Context, token string) error { return nil }

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcserver.AuthInterceptor(authFn)),
	)
	orderpb.RegisterOrderServiceServer(srv, grpcserver.NewOrderServer())

	client := startInterceptorTestServer(t, srv)

	_, err := client.CreateOrder(context.Background(), &orderpb.CreateOrderRequest{
		CustomerID: "cust-1",
		Amount:     100,
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestAuthInterceptor_InvalidToken(t *testing.T) {
	authFn := func(ctx context.Context, token string) error {
		return errors.New("token expired")
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcserver.AuthInterceptor(authFn)),
	)
	orderpb.RegisterOrderServiceServer(srv, grpcserver.NewOrderServer())

	client := startInterceptorTestServer(t, srv)

	md := metadata.Pairs("authorization", "Bearer expired-token")
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	_, err := client.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		CustomerID: "cust-1",
		Amount:     100,
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
	assert.Contains(t, st.Message(), "token expired")
}

func TestChainedInterceptors_RecoveryThenLoggingThenAuth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authFn := func(ctx context.Context, token string) error {
		if token == "ok" {
			return nil
		}
		return errors.New("denied")
	}

	//👉 Order: recovery → logging → auth → handler
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcserver.RecoveryInterceptor(logger),
			grpcserver.LoggingInterceptor(logger),
			grpcserver.AuthInterceptor(authFn),
		),
	)
	orderpb.RegisterOrderServiceServer(srv, grpcserver.NewOrderServer())

	client := startInterceptorTestServer(t, srv)

	md := metadata.Pairs("authorization", "Bearer ok")
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	resp, err := client.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		CustomerID: "cust-chain",
		Amount:     200,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
}

// --- helpers ---------------------------------------------------------------

//panicServer - the server that is panicking (for testing recovery).
type panicServer struct {
	orderpb.UnimplementedOrderServiceServer
}

func (s *panicServer) CreateOrder(ctx context.Context, req *orderpb.CreateOrderRequest) (*orderpb.OrderResponse, error) {
	panic("unexpected nil pointer")
}

//startInterceptorTestServer starts the gRPC server via bufconn and returns a client.
func startInterceptorTestServer(t *testing.T, srv *grpc.Server) orderpb.OrderServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	go srv.Serve(lis)
	t.Cleanup(func() {
		srv.GracefulStop()
		lis.Close()
	})

	conn, err := grpc.DialContext( //nolint:staticcheck
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return orderpb.NewOrderServiceClient(conn)
}
