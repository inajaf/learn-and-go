package server_test

// 👉 Testing gRPC without a real network connection.
//    We use bufconn — an in-memory "pipe".
//    The server and client run in the same program, without a real TCP port.
//
//    Why is this better than "bring up a server on :50051"?
//    1. The test doesn't depend on busy ports
//    2. Runs in parallel (t.Parallel())
//    3. Fast — no network latency
//    4. No port cleanup needed after the test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	orderpb "learning_path/03_grpc/gen"
	grpcserver "learning_path/03_grpc/server"

	"net"
)

const bufSize = 1024 * 1024

// startTestServer starts a gRPC server via bufconn and returns a client.
// 👉 Helper function — to avoid duplicating setup in every test.
func startTestServer(t *testing.T) orderpb.OrderServiceClient {
	t.Helper()

	// Create an in-memory listener instead of a real TCP port
	lis := bufconn.Listen(bufSize)

	// Create and register the gRPC server
	srv := grpc.NewServer()
	orderpb.RegisterOrderServiceServer(srv, grpcserver.NewOrderServer())

	// Run the server in a goroutine
	go func() {
		if err := srv.Serve(lis); err != nil {
			// Acceptable in tests — the server stops when the listener is closed
		}
	}()

	// Stop the server when the test finishes
	t.Cleanup(func() {
		srv.GracefulStop()
		lis.Close()
	})

	// Connect the client through bufconn
	// 👉 grpc.Dial (or DialContext) — creates a connection to the server.
	//    We pass a custom Dialer that uses bufconn instead of TCP.
	conn, err := grpc.DialContext( //nolint:staticcheck // grpc.Dial kept for v1.62 compatibility
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

// TestCreateOrder_Success — creating an order via gRPC.
func TestCreateOrder_Success(t *testing.T) {
	client := startTestServer(t)

	resp, err := client.CreateOrder(context.Background(), &orderpb.CreateOrderRequest{
		CustomerID: "cust-grpc-1",
		Amount:     299.99,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
	assert.Equal(t, "cust-grpc-1", resp.GetCustomerId())
	assert.Equal(t, 299.99, resp.GetAmount())
	assert.Equal(t, "pending", resp.GetStatus())
}

// TestCreateOrder_Validation — invalid data → gRPC error.
func TestCreateOrder_Validation(t *testing.T) {
	client := startTestServer(t)

	tests := []struct {
		name     string
		req      *orderpb.CreateOrderRequest
		wantCode codes.Code
	}{
		{
			name:     "empty customer_id",
			req:      &orderpb.CreateOrderRequest{CustomerID: "", Amount: 100},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "zero amount",
			req:      &orderpb.CreateOrderRequest{CustomerID: "cust-1", Amount: 0},
			wantCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateOrder(context.Background(), tt.req)
			require.Error(t, err)

			// 👉 status.Code() extracts the gRPC code from the error
			assert.Equal(t, tt.wantCode, status.Code(err))
		})
	}
}

// TestGetOrder_NotFound — fetching a non-existent order.
func TestGetOrder_NotFound(t *testing.T) {
	client := startTestServer(t)

	_, err := client.GetOrder(context.Background(), &orderpb.GetOrderRequest{
		Id: "non-existent-order",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// TestListOrders_Streaming — server-side streaming test.
func TestListOrders_Streaming(t *testing.T) {
	client := startTestServer(t)

	// Create a few orders
	for i := 0; i < 3; i++ {
		_, err := client.CreateOrder(context.Background(), &orderpb.CreateOrderRequest{
			CustomerID: "cust-stream",
			Amount:     float64(100 * (i + 1)),
		})
		require.NoError(t, err)
	}

	// Open the streaming connection
	stream, err := client.ListOrders(context.Background(), &orderpb.ListOrdersRequest{})
	require.NoError(t, err)

	// Read every message from the stream
	var received []*orderpb.OrderResponse
	for {
		resp, err := stream.Recv()
		if err != nil {
			break // io.EOF — the stream is finished
		}
		received = append(received, resp)
	}

	// 👉 We received 3 orders via streaming, not as one big response
	assert.Len(t, received, 3)
}
