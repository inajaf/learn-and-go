//Package capstone is the final project that combines all the patterns.
//
//This is a minimal microservice with:
//- Repository Pattern (interface + in-memory implementation)
//- DTO layer (transport/domain/database separation)
//- Event publishing (via Publisher interface)
//- gRPC handler (transport layer)
package capstone

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ═══════════════════════════════════════════════════════════════
//DOMAIN - domain model
// ═══════════════════════════════════════════════════════════════

var ErrOrderNotFound = errors.New("order not found")
var ErrInvalidInput = errors.New("invalid input")

type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusCancelled OrderStatus = "cancelled"
)

//Order - domain model. No tags, no dependencies.
type Order struct {
	ID          string
	CustomerID  string
	Items       []OrderItem
	TotalAmount float64
	Status      OrderStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

//OrderItem - position in the order.
type OrderItem struct {
	ProductID string
	Name      string
	Quantity  int
	Price     float64
}

//TotalPrice calculates the order amount.
//👉 Business logic on the domain model is not in the service!
func (o *Order) TotalPrice() float64 {
	var total float64
	for _, item := range o.Items {
		total += item.Price * float64(item.Quantity)
	}
	return total
}

// ═══════════════════════════════════════════════════════════════
//INTERFACES - dependency contracts
// ═══════════════════════════════════════════════════════════════

//OrderRepository - order storage.
//👉 context.Context is the first parameter of all methods (Go standard).
type OrderRepository interface {
	Save(ctx context.Context, order *Order) error
	FindByID(ctx context.Context, id string) (*Order, error)
	FindAll(ctx context.Context) ([]*Order, error)
}

//EventPublisher - publishing events.
type EventPublisher interface {
	Publish(eventType string, payload any) error
}

// ═══════════════════════════════════════════════════════════════
//DTOs - transport objects
// ═══════════════════════════════════════════════════════════════

//CreateOrderRequest - DTO of the incoming request.
type CreateOrderRequest struct {
	CustomerID string              `json:"customer_id"`
	Items      []CreateItemRequest `json:"items"`
}

type CreateItemRequest struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

//OrderResponse - DTO of the outgoing response.
type OrderResponse struct {
	ID          string  `json:"id"`
	CustomerID  string  `json:"customer_id"`
	TotalAmount float64 `json:"total_amount"`
	Status      string  `json:"status"`
	ItemCount   int     `json:"item_count"`
	CreatedAt   string  `json:"created_at"`
}

// ═══════════════════════════════════════════════════════════════
//MAPPER - conversion between layers
// ═══════════════════════════════════════════════════════════════

//requestToDomain converts DTO → domain model.
func requestToDomain(req CreateOrderRequest) (*Order, error) {
	if req.CustomerID == "" {
		return nil, fmt.Errorf("%w: customer_id is required", ErrInvalidInput)
	}
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("%w: items cannot be empty", ErrInvalidInput)
	}

	items := make([]OrderItem, len(req.Items))
	for i, item := range req.Items {
		if item.Quantity <= 0 {
			return nil, fmt.Errorf("%w: item quantity must be positive", ErrInvalidInput)
		}
		items[i] = OrderItem{
			ProductID: item.ProductID,
			Name:      item.Name,
			Quantity:  item.Quantity,
			Price:     item.Price,
		}
	}

	return &Order{
		ID:         fmt.Sprintf("order-%d", time.Now().UnixNano()),
		CustomerID: req.CustomerID,
		Items:      items,
		Status:     StatusPending,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

//domainToResponse converts domain model → response DTO.
func domainToResponse(order *Order) OrderResponse {
	return OrderResponse{
		ID:          order.ID,
		CustomerID:  order.CustomerID,
		TotalAmount: order.TotalPrice(),
		Status:      string(order.Status),
		ItemCount:   len(order.Items),
		CreatedAt:   order.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// ═══════════════════════════════════════════════════════════════
//SERVICE - business logic
// ═══════════════════════════════════════════════════════════════

//OrderService is a business logic layer.
//Depends only on interfaces - does not know about the database or gRPC.
type OrderService struct {
	repo      OrderRepository
	publisher EventPublisher
}

//NewOrderService - constructor (Dependency Injection).
func NewOrderService(repo OrderRepository, publisher EventPublisher) *OrderService {
	return &OrderService{repo: repo, publisher: publisher}
}

//CreateOrder - creating an order.
func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderRequest) (OrderResponse, error) {
	//1. Convert DTO → domain (with validation)
	order, err := requestToDomain(req)
	if err != nil {
		return OrderResponse{}, err
	}

	//2. We count the amount
	order.TotalAmount = order.TotalPrice()

	//3. Save
	if err := s.repo.Save(ctx, order); err != nil {
		return OrderResponse{}, fmt.Errorf("CreateOrder: %w", err)
	}

	//4. Publish the event (non-fatal)
	_ = s.publisher.Publish("order.created", map[string]any{
		"order_id":    order.ID,
		"customer_id": order.CustomerID,
		"total":       order.TotalAmount,
	})

	//5. Convert domain → response DTO
	return domainToResponse(order), nil
}

//GetOrder - receiving an order.
func (s *OrderService) GetOrder(ctx context.Context, id string) (OrderResponse, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("GetOrder: %w", err)
	}
	return domainToResponse(order), nil
}

//ListOrders - list of orders.
func (s *OrderService) ListOrders(ctx context.Context) ([]OrderResponse, error) {
	orders, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListOrders: %w", err)
	}
	responses := make([]OrderResponse, len(orders))
	for i, o := range orders {
		responses[i] = domainToResponse(o)
	}
	return responses, nil
}

// ═══════════════════════════════════════════════════════════════
//INFRASTRUCTURE - interface implementations
// ═══════════════════════════════════════════════════════════════

//InMemoryOrderRepository - in-memory implementation.
type InMemoryOrderRepository struct {
	orders map[string]*Order
}

func NewInMemoryOrderRepository() *InMemoryOrderRepository {
	return &InMemoryOrderRepository{orders: make(map[string]*Order)}
}

func (r *InMemoryOrderRepository) Save(_ context.Context, order *Order) error {
	r.orders[order.ID] = order
	return nil
}

func (r *InMemoryOrderRepository) FindByID(_ context.Context, id string) (*Order, error) {
	o, ok := r.orders[id]
	if !ok {
		return nil, fmt.Errorf("FindByID %q: %w", id, ErrOrderNotFound)
	}
	return o, nil
}

func (r *InMemoryOrderRepository) FindAll(_ context.Context) ([]*Order, error) {
	result := make([]*Order, 0, len(r.orders))
	for _, o := range r.orders {
		result = append(result, o)
	}
	return result, nil
}

//LogPublisher - publisher who writes to the log.
type LogPublisher struct{}

func (p *LogPublisher) Publish(eventType string, payload any) error {
	fmt.Printf("[EVENT] %s: %v\n", eventType, payload)
	return nil
}
