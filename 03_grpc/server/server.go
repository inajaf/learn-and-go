//Package server contains the gRPC server implementation.
package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	orderpb "learning_path/03_grpc/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var orderIDSeq uint64

// ─────────────────────────────────────────────────────────────────
//OrderServer is a gRPC server implementation.
//
//👉 Notice that it embeds orderpb.UnimplementedOrderServiceServer.
//This means: if a new method is added to proto, the server will not break -
//it will return "not implemented" by default. This is best practice.
// ─────────────────────────────────────────────────────────────────

type order struct {
	id         string
	customerID string
	amount     float64
	status     string
	createdAt  time.Time
}

//OrderServer implements OrderServiceServer.
type OrderServer struct {
	orderpb.UnimplementedOrderServiceServer //👉 built in for forward compatibility

	mu     sync.RWMutex
	orders map[string]*order
}

//NewOrderServer creates and returns a new server.
func NewOrderServer() *OrderServer {
	return &OrderServer{
		orders: make(map[string]*order),
	}
}

//CreateOrder is an RPC implementation of the CreateOrder method.
//👉 gRPC automatically deserializes the Protobuf → Go structure (*CreateOrderRequest).
//
//We work with regular Go types.
func (s *OrderServer) CreateOrder(ctx context.Context, req *orderpb.CreateOrderRequest) (*orderpb.OrderResponse, error) {
	//gRPC status codes are similar to HTTP codes, but for RPC.
	//👉 Instead of 400 → codes.InvalidArgument
	//Instead of 404 → codes.NotFound
	//Instead of 500 → codes.Internal
	if req.GetCustomerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "customer_id is required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive, got %v", req.GetAmount())
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	id := fmt.Sprintf("order-%d-%d", now.UnixNano(), atomic.AddUint64(&orderIDSeq, 1))
	o := &order{
		id:         id,
		customerID: req.GetCustomerId(),
		amount:     req.GetAmount(),
		status:     "pending",
		createdAt:  now,
	}
	s.orders[id] = o

	//Returning protobuf message
	return toProto(o), nil
}

//GetOrder is an RPC implementation of the GetOrder method.
func (s *OrderServer) GetOrder(ctx context.Context, req *orderpb.GetOrderRequest) (*orderpb.OrderResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	o, ok := s.orders[req.GetId()]
	if !ok {
		//👉 codes.NotFound - gRPC analogue of HTTP 404
		return nil, status.Errorf(codes.NotFound, "order %q not found", req.GetId())
	}

	return toProto(o), nil
}

//ListOrders is a Server-Side Streaming RPC implementation.
//👉 Instead of returning a single value, we send a stream of messages.
//
//The client reads them one at a time via stream.Recv().
//Useful for large lists - no need to load everything into memory at once.
func (s *OrderServer) ListOrders(req *orderpb.ListOrdersRequest, stream orderpb.OrderService_ListOrdersServer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, o := range s.orders {
		//👉 stream.Send() sends one message to the client
		if err := stream.Send(toProto(o)); err != nil {
			return status.Errorf(codes.Internal, "failed to send order: %v", err)
		}
	}
	return nil
}

//toProto converts the internal structure into a protobuf message.
func toProto(o *order) *orderpb.OrderResponse {
	return &orderpb.OrderResponse{
		Id:         o.id,
		CustomerID: o.customerID,
		Amount:     o.amount,
		Status:     o.status,
		CreatedAt:  o.createdAt.Format("2006-01-02 15:04:05"),
	}
}
