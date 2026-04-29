//Package service contains the business logic for working with orders.
//The service depends only on interfaces - it does not know about SQL or PostgreSQL.
package service

import (
	"fmt"
	"time"

	db "learning_path/10_database"
)

// ═══════════════════════════════════════════════════════════════
//DTOs - transport objects (Module 2)
// ═══════════════════════════════════════════════════════════════

//CreateCustomerRequest - request to create a client.
type CreateCustomerRequest struct {
	Name  string
	Email string
}

//CreateOrderRequest - request to create an order.
type CreateOrderRequest struct {
	CustomerID string
	Items      []CreateItemRequest
	Notes      string
}

//CreateItemRequest - position in the request.
type CreateItemRequest struct {
	ProductID string
	Name      string
	Quantity  int
	UnitPrice float64
}

//OrderResponse - response with order data.
type OrderResponse struct {
	ID          string
	CustomerID  string
	Status      string
	TotalAmount float64
	Notes       string
	Items       []ItemResponse
	Version     int
	CreatedAt   string
	UpdatedAt   string
}

//ItemResponse - position in the response.
type ItemResponse struct {
	ProductID string
	Name      string
	Quantity  int
	UnitPrice float64
	Subtotal  float64
}

//CustomerResponse - response with customer data.
type CustomerResponse struct {
	ID        string
	Name      string
	Email     string
	CreatedAt string
}

// ═══════════════════════════════════════════════════════════════
// MAPPERS
// ═══════════════════════════════════════════════════════════════

func orderToResponse(order *db.Order) OrderResponse {
	items := make([]ItemResponse, len(order.Items))
	for i, item := range order.Items {
		items[i] = ItemResponse{
			ProductID: item.ProductID,
			Name:      item.Name,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
			Subtotal:  float64(item.Quantity) * item.UnitPrice,
		}
	}
	return OrderResponse{
		ID:          order.ID,
		CustomerID:  order.CustomerID,
		Status:      string(order.Status),
		TotalAmount: order.TotalAmount,
		Notes:       order.Notes,
		Items:       items,
		Version:     order.Version,
		CreatedAt:   order.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:   order.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

func customerToResponse(c *db.Customer) CustomerResponse {
	return CustomerResponse{
		ID:        c.ID,
		Name:      c.Name,
		Email:     c.Email,
		CreatedAt: c.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// ═══════════════════════════════════════════════════════════════
//OrderService - business logic. Uses repositories.
// ═══════════════════════════════════════════════════════════════

//OrderService - order management service.
type OrderService struct {
	orders    db.OrderRepository
	customers db.CustomerRepository
}

//NewOrderService creates a service.
func NewOrderService(orders db.OrderRepository, customers db.CustomerRepository) *OrderService {
	return &OrderService{orders: orders, customers: customers}
}

//CreateCustomer creates a new customer.
func (s *OrderService) CreateCustomer(req CreateCustomerRequest) (CustomerResponse, error) {
	if req.Name == "" {
		return CustomerResponse{}, fmt.Errorf("%w: name is required", db.ErrInvalidInput)
	}
	if req.Email == "" {
		return CustomerResponse{}, fmt.Errorf("%w: email is required", db.ErrInvalidInput)
	}

	customer := &db.Customer{
		ID:    fmt.Sprintf("cust-%d", time.Now().UnixNano()),
		Name:  req.Name,
		Email: req.Email,
	}
	if err := s.customers.Create(customer); err != nil {
		return CustomerResponse{}, fmt.Errorf("CreateCustomer: %w", err)
	}
	return customerToResponse(customer), nil
}

//GetCustomer returns the customer by ID.
func (s *OrderService) GetCustomer(id string) (CustomerResponse, error) {
	c, err := s.customers.FindByID(id)
	if err != nil {
		return CustomerResponse{}, fmt.Errorf("GetCustomer: %w", err)
	}
	return customerToResponse(c), nil
}

//ListCustomers returns all customers.
func (s *OrderService) ListCustomers() ([]CustomerResponse, error) {
	customers, err := s.customers.List()
	if err != nil {
		return nil, fmt.Errorf("ListCustomers: %w", err)
	}
	result := make([]CustomerResponse, len(customers))
	for i, c := range customers {
		result[i] = customerToResponse(c)
	}
	return result, nil
}

//CreateOrder creates an order.
//Checks that the client exists - the foreign key at the database level will also check,
//but it's better to give an understandable error before INSERT.
func (s *OrderService) CreateOrder(req CreateOrderRequest) (OrderResponse, error) {
	if req.CustomerID == "" {
		return OrderResponse{}, fmt.Errorf("%w: customer_id is required", db.ErrInvalidInput)
	}
	if len(req.Items) == 0 {
		return OrderResponse{}, fmt.Errorf("%w: at least one item required", db.ErrInvalidInput)
	}

	//Checking that the client exists
	if _, err := s.customers.FindByID(req.CustomerID); err != nil {
		return OrderResponse{}, fmt.Errorf("CreateOrder: customer check: %w", err)
	}

	items := make([]db.OrderItem, 0, len(req.Items))
	for _, item := range req.Items {
		if item.Quantity <= 0 {
			return OrderResponse{}, fmt.Errorf("%w: quantity must be positive", db.ErrInvalidInput)
		}
		items = append(items, db.OrderItem{
			ID:        fmt.Sprintf("item-%d", time.Now().UnixNano()),
			ProductID: item.ProductID,
			Name:      item.Name,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
		})
	}

	order := &db.Order{
		ID:         fmt.Sprintf("ord-%d", time.Now().UnixNano()),
		CustomerID: req.CustomerID,
		Status:     db.StatusPending,
		Notes:      req.Notes,
		Items:      items,
	}
	order.TotalAmount = order.Total()

	if err := s.orders.Create(order); err != nil {
		return OrderResponse{}, fmt.Errorf("CreateOrder: %w", err)
	}
	return orderToResponse(order), nil
}

//GetOrder returns an order by ID.
func (s *OrderService) GetOrder(id string) (OrderResponse, error) {
	order, err := s.orders.FindByID(id)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("GetOrder: %w", err)
	}
	return orderToResponse(order), nil
}

//GetOrdersByCustomer returns customer orders.
func (s *OrderService) GetOrdersByCustomer(customerID string) ([]OrderResponse, error) {
	orders, err := s.orders.FindByCustomerID(customerID)
	if err != nil {
		return nil, fmt.Errorf("GetOrdersByCustomer: %w", err)
	}
	result := make([]OrderResponse, len(orders))
	for i, o := range orders {
		result[i] = orderToResponse(o)
	}
	return result, nil
}

//ConfirmOrder confirms the order (with optimistic locking).
func (s *OrderService) ConfirmOrder(id string) (OrderResponse, error) {
	order, err := s.orders.FindByID(id)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("ConfirmOrder: %w", err)
	}
	if err := order.Confirm(); err != nil {
		return OrderResponse{}, fmt.Errorf("ConfirmOrder: %w", err)
	}
	//Update uses optimistic locking internally
	if err := s.orders.Update(order); err != nil {
		return OrderResponse{}, fmt.Errorf("ConfirmOrder: %w", err)
	}
	return orderToResponse(order), nil
}

//CancelOrder cancels the order.
func (s *OrderService) CancelOrder(id string) (OrderResponse, error) {
	order, err := s.orders.FindByID(id)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("CancelOrder: %w", err)
	}
	if err := order.Cancel(); err != nil {
		return OrderResponse{}, fmt.Errorf("CancelOrder: %w", err)
	}
	if err := s.orders.Update(order); err != nil {
		return OrderResponse{}, fmt.Errorf("CancelOrder: %w", err)
	}
	return orderToResponse(order), nil
}

//DeleteOrder softly deletes an order.
func (s *OrderService) DeleteOrder(id string) error {
	if err := s.orders.SoftDelete(id); err != nil {
		return fmt.Errorf("DeleteOrder: %w", err)
	}
	return nil
}

//ListAllOrders returns all orders with positions via JOIN.
func (s *OrderService) ListAllOrders() ([]OrderResponse, error) {
	orders, err := s.orders.ListWithItems()
	if err != nil {
		return nil, fmt.Errorf("ListAllOrders: %w", err)
	}
	result := make([]OrderResponse, len(orders))
	for i, o := range orders {
		result[i] = orderToResponse(o)
	}
	return result, nil
}
