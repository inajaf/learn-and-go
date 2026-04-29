//Package repository contains the PostgreSQL implementation of repositories.
//
//All SQL queries live here. The service layer does not know about SQL -
//it only works through interfaces from domain.go.
//
//Techniques used:
//- sqlx.NamedExec - INSERT/UPDATE with named parameters (not positional $1, $2)
//- sqlx.Get/Select - scanning rows into structures using `db:` tags
//- sql.Tx - transactions (CREATE order + items atomically)
//- Soft delete — deleted_at IS NULL in WHERE
//- Optimistic lock — WHERE version = $N, update version+1
//- JOIN + spread - one SELECT for orders + positions
package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	db "learning_path/10_database"
)

//sqlxQuerier is a common interface for *sqlx.DB and *sqlx.Tx.
//Allows you to pass a transaction instead of a connection in tests.
type sqlxQuerier interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Select(dest interface{}, query string, args ...interface{}) error
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRowx(query string, args ...interface{}) *sqlx.Row
	Rebind(query string) string
}

// ═══════════════════════════════════════════════════════════════
//DB MODELS - structures corresponding to rows in tables.
//
//👉 Module 2: Persistence layer. `db:` tags for sqlx.
//They differ from domain models:
//- snake_case instead of CamelCase
//- nullable fields = pointer (DeletedAt *time.Time)
//- version for optimistic locking
// ═══════════════════════════════════════════════════════════════

//orderRow — row of the orders table.
type orderRow struct {
	ID          string         `db:"id"`
	CustomerID  string         `db:"customer_id"`
	Status      string         `db:"status"`
	TotalAmount float64        `db:"total_amount"`
	Notes       sql.NullString `db:"notes"`      //NULL in DB → NullString in Go
	DeletedAt   *time.Time     `db:"deleted_at"` // pointer: NULL = nil
	Version     int            `db:"version"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

//orderItemRow — row of the order_items table.
type orderItemRow struct {
	ID        string  `db:"id"`
	OrderID   string  `db:"order_id"`
	ProductID string  `db:"product_id"`
	Name      string  `db:"name"`
	Quantity  int     `db:"quantity"`
	UnitPrice float64 `db:"unit_price"`
}

//customerRow — row of the customers table.
type customerRow struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// ═══════════════════════════════════════════════════════════════
//MAPPERS - conversion DB model ↔ Domain model (Module 2)
// ═══════════════════════════════════════════════════════════════

func rowToOrder(row orderRow, items []orderItemRow) *db.Order {
	domainItems := make([]db.OrderItem, len(items))
	for i, item := range items {
		domainItems[i] = db.OrderItem{
			ID:        item.ID,
			OrderID:   item.OrderID,
			ProductID: item.ProductID,
			Name:      item.Name,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
		}
	}
	return &db.Order{
		ID:          row.ID,
		CustomerID:  row.CustomerID,
		Status:      db.OrderStatus(row.Status),
		TotalAmount: row.TotalAmount,
		Notes:       row.Notes.String,
		Items:       domainItems,
		Version:     row.Version,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func rowToCustomer(row customerRow) *db.Customer {
	return &db.Customer{
		ID:        row.ID,
		Name:      row.Name,
		Email:     row.Email,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// ═══════════════════════════════════════════════════════════════
//PostgresOrderRepository is an implementation of db.OrderRepository.
// ═══════════════════════════════════════════════════════════════

//PostgresOrderRepository - working with orders via PostgreSQL.
type PostgresOrderRepository struct {
	db sqlxQuerier
}

//NewPostgresOrderRepository creates a repository.
//Accepts sqlxQuerier - works with both *sqlx.DB and *sqlx.Tx.
func NewPostgresOrderRepository(database sqlxQuerier) *PostgresOrderRepository {
	return &PostgresOrderRepository{db: database}
}

//Compile-time check - make sure that we implement the interface (Module 1).
var _ db.OrderRepository = (*PostgresOrderRepository)(nil)

// ─────────────────────────────────────────────────────────────────
//Create - saves the order + positions.
//
//👉 TRANSACTION: When r.db is *sqlx.DB, we manage the transaction ourselves.
//
//When r.db is *sqlx.Tx (from the test), the transaction already exists outside.
//
//For production use NewPostgresOrderRepository(db) - auto transaction.
//For tests - NewPostgresOrderRepository(tx) - isolation via ROLLBACK.
// ─────────────────────────────────────────────────────────────────
func (r *PostgresOrderRepository) Create(order *db.Order) error {
	row := orderRow{
		ID:          order.ID,
		CustomerID:  order.CustomerID,
		Status:      string(order.Status),
		TotalAmount: order.TotalAmount,
		Notes:       sql.NullString{String: order.Notes, Valid: order.Notes != ""},
		Version:     1,
	}

	// ─── INSERT orders ────────────────────────────────────────
	//QueryRowx + Scan: get generated timestamps back from the database
	if err := r.db.QueryRowx(
		`INSERT INTO orders (id, customer_id, status, total_amount, notes, version)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING created_at, updated_at`,
		row.ID, row.CustomerID, row.Status, row.TotalAmount, row.Notes, row.Version,
	).Scan(&order.CreatedAt, &order.UpdatedAt); err != nil {
		return fmt.Errorf("Create: insert order: %w", err)
	}
	order.Version = 1

	// ─── INSERT order_items ────────────────────────────────────
	//👉 Atomicity is guaranteed by the fact that both INSERTs go through the same
	//connection (db or tx). If tx, they are in the same transaction.
	for i := range order.Items {
		item := &order.Items[i]
		if err := r.db.QueryRowx(
			`INSERT INTO order_items (id, order_id, product_id, name, quantity, unit_price)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING id`,
			item.ID, order.ID, item.ProductID, item.Name, item.Quantity, item.UnitPrice,
		).Scan(&item.ID); err != nil {
			return fmt.Errorf("Create: insert item %s: %w", item.ProductID, err)
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────
//FindByID - receives an order with positions.
//
//👉 Two queries instead of JOIN: the first for the order, the second for the positions.
//
//It's easier to understand. JOIN option - in ListWithItems.
//
// ─────────────────────────────────────────────────────────────────
func (r *PostgresOrderRepository) FindByID(id string) (*db.Order, error) {
	var row orderRow
	err := r.db.Get(&row,
		//👉 deleted_at IS NULL - filters softly deleted records
		`SELECT id, customer_id, status, total_amount, notes, deleted_at, version, created_at, updated_at
		 FROM orders
		 WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("FindByID %q: %w", id, db.ErrOrderNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("FindByID: %w", err)
	}

	items, err := r.findItemsByOrderID(id)
	if err != nil {
		return nil, err
	}

	return rowToOrder(row, items), nil
}

//findItemsByOrderID - auxiliary query for order items.
func (r *PostgresOrderRepository) findItemsByOrderID(orderID string) ([]orderItemRow, error) {
	var items []orderItemRow
	//👉 sqlx.Select: scans multiple rows into slice structures
	err := r.db.Select(&items,
		`SELECT id, order_id, product_id, name, quantity, unit_price
		 FROM order_items
		 WHERE order_id = $1
		 ORDER BY name`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("findItemsByOrderID %q: %w", orderID, err)
	}
	return items, nil
}

// ─────────────────────────────────────────────────────────────────
//FindByCustomerID - orders for a specific customer.
// ─────────────────────────────────────────────────────────────────
func (r *PostgresOrderRepository) FindByCustomerID(customerID string) ([]*db.Order, error) {
	var rows []orderRow
	err := r.db.Select(&rows,
		`SELECT id, customer_id, status, total_amount, notes, deleted_at, version, created_at, updated_at
		 FROM orders
		 WHERE customer_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC`,
		customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("FindByCustomerID: %w", err)
	}
	return r.loadItemsForOrders(rows)
}

// ─────────────────────────────────────────────────────────────────
//FindByStatus - orders by status.
// ─────────────────────────────────────────────────────────────────
func (r *PostgresOrderRepository) FindByStatus(status db.OrderStatus) ([]*db.Order, error) {
	var rows []orderRow
	err := r.db.Select(&rows,
		`SELECT id, customer_id, status, total_amount, notes, deleted_at, version, created_at, updated_at
		 FROM orders
		 WHERE status = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC`,
		string(status),
	)
	if err != nil {
		return nil, fmt.Errorf("FindByStatus: %w", err)
	}
	return r.loadItemsForOrders(rows)
}

// ─────────────────────────────────────────────────────────────────
//Update—updates an order with optimistic locking.
//
// 👉 OPTIMISTIC LOCKING:
//
//	WHERE version = $current_version AND id = $id
//	SET version = version + 1
//
//If someone has already updated the entry (version has changed) -
//UPDATE will not find the row (affected rows = 0) → return ErrVersionConflict.
//The client must re-read the data and try again.
//
// ─────────────────────────────────────────────────────────────────
func (r *PostgresOrderRepository) Update(order *db.Order) error {
	result, err := r.db.Exec(
		`UPDATE orders
		 SET status       = $1,
		     total_amount = $2,
		     notes        = $3,
		     version      = version + 1,
		     updated_at   = NOW()
		 WHERE id = $4
		   AND version = $5       -- optimistic lock check
		   AND deleted_at IS NULL`,
		string(order.Status),
		order.TotalAmount,
		sql.NullString{String: order.Notes, Valid: order.Notes != ""},
		order.ID,
		order.Version, //current version - must match
	)
	if err != nil {
		return fmt.Errorf("Update: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		//Either the entry was not found or the version did not match
		return fmt.Errorf("Update %q: %w", order.ID, db.ErrVersionConflict)
	}

	order.Version++ //updating the local copy
	return nil
}

// ─────────────────────────────────────────────────────────────────
//SoftDelete - soft delete: sets deleted_at.
//
// 👉 SOFT DELETE:
//
//The row is NOT physically deleted. Only deleted_at = NOW().
//All queries filter WHERE deleted_at IS NULL.
//Advantages:
//- History is saved (audit)
//- You can restore the recording
//- Links from other tables do not break
//
// ─────────────────────────────────────────────────────────────────
func (r *PostgresOrderRepository) SoftDelete(id string) error {
	result, err := r.db.Exec(
		`UPDATE orders SET deleted_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("SoftDelete: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("SoftDelete %q: %w", id, db.ErrOrderNotFound)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────
//ListWithItems - list of all orders with positions via JOIN.
//
// 👉 JOIN QUERY:
//
//One query returns orders + positions via LEFT JOIN.
//Then we “expand” the strings into objects with nested slices.
//This is more efficient than N+1 queries (one query for each order).
//
// ─────────────────────────────────────────────────────────────────
func (r *PostgresOrderRepository) ListWithItems() ([]*db.Order, error) {
	//👉 JOIN: each line = order + one of its positions
	//If an order has 3 positions, there will be 3 lines with one order
	type joinRow struct {
		//orders fields:
		OrderID      string         `db:"order_id"`
		CustomerID   string         `db:"customer_id"`
		Status       string         `db:"status"`
		TotalAmount  float64        `db:"total_amount"`
		Notes        sql.NullString `db:"notes"`
		OrderVersion int            `db:"version"`
		OrderCreated time.Time      `db:"order_created_at"`
		OrderUpdated time.Time      `db:"order_updated_at"`
		//order_items fields (NULL if there are no items):
		ItemID        sql.NullString  `db:"item_id"`
		ItemProductID sql.NullString  `db:"product_id"`
		ItemName      sql.NullString  `db:"item_name"`
		ItemQuantity  sql.NullInt32   `db:"quantity"`
		ItemUnitPrice sql.NullFloat64 `db:"unit_price"`
	}

	var rows []joinRow
	err := r.db.Select(&rows, `
		SELECT
			o.id          AS order_id,
			o.customer_id,
			o.status,
			o.total_amount,
			o.notes,
			o.version,
			o.created_at  AS order_created_at,
			o.updated_at  AS order_updated_at,
			oi.id         AS item_id,
			oi.product_id,
			oi.name       AS item_name,
			oi.quantity,
			oi.unit_price
		FROM orders o
		LEFT JOIN order_items oi ON oi.order_id = o.id
		WHERE o.deleted_at IS NULL
		ORDER BY o.created_at DESC, oi.name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("ListWithItems: %w", err)
	}

	//─── Expand JOIN rows → orderID map → Order ────
	//👉 This is the classic "denormalize JOIN rows" operation
	ordersMap := make(map[string]*db.Order)
	ordersOrder := make([]string, 0) //keep order

	for _, row := range rows {
		order, exists := ordersMap[row.OrderID]
		if !exists {
			order = &db.Order{
				ID:          row.OrderID,
				CustomerID:  row.CustomerID,
				Status:      db.OrderStatus(row.Status),
				TotalAmount: row.TotalAmount,
				Notes:       row.Notes.String,
				Version:     row.OrderVersion,
				CreatedAt:   row.OrderCreated,
				UpdatedAt:   row.OrderUpdated,
				Items:       []db.OrderItem{},
			}
			ordersMap[row.OrderID] = order
			ordersOrder = append(ordersOrder, row.OrderID)
		}

		//Add a position if there is one (LEFT JOIN may return NULL)
		if row.ItemID.Valid {
			order.Items = append(order.Items, db.OrderItem{
				ID:        row.ItemID.String,
				OrderID:   row.OrderID,
				ProductID: row.ItemProductID.String,
				Name:      row.ItemName.String,
				Quantity:  int(row.ItemQuantity.Int32),
				UnitPrice: row.ItemUnitPrice.Float64,
			})
		}
	}

	//We collect the results in the same order
	result := make([]*db.Order, 0, len(ordersOrder))
	for _, id := range ordersOrder {
		result = append(result, ordersMap[id])
	}
	return result, nil
}

//loadItemsForOrders - Helper method: Loads items for multiple orders.
//👉 Uses IN query instead of N separate queries.
func (r *PostgresOrderRepository) loadItemsForOrders(rows []orderRow) ([]*db.Order, error) {
	if len(rows) == 0 {
		return []*db.Order{}, nil
	}

	//Collecting order IDs
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}

	//👉 sqlx.In expands slice in SQL IN ($1,$2,$3,...)
	query, args, err := sqlx.In(
		`SELECT id, order_id, product_id, name, quantity, unit_price
		 FROM order_items
		 WHERE order_id IN (?)
		 ORDER BY order_id, name`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("loadItemsForOrders: build query: %w", err)
	}
	//Postgres uses $1,$2 instead of ? - rebind
	query = r.db.Rebind(query)

	var itemRows []orderItemRow
	if err := r.db.Select(&itemRows, query, args...); err != nil {
		return nil, fmt.Errorf("loadItemsForOrders: %w", err)
	}

	//Grouping positions by orderID
	itemsByOrderID := make(map[string][]orderItemRow)
	for _, item := range itemRows {
		itemsByOrderID[item.OrderID] = append(itemsByOrderID[item.OrderID], item)
	}

	//Collecting the result
	orders := make([]*db.Order, len(rows))
	for i, row := range rows {
		orders[i] = rowToOrder(row, itemsByOrderID[row.ID])
	}
	return orders, nil
}

// ═══════════════════════════════════════════════════════════════
//PostgresCustomerRepository is an implementation of db.CustomerRepository.
// ═══════════════════════════════════════════════════════════════

//PostgresCustomerRepository - working with clients via PostgreSQL.
type PostgresCustomerRepository struct {
	db sqlxQuerier
}

//NewPostgresCustomerRepository creates a repository.
//Accepts sqlxQuerier - works with both *sqlx.DB and *sqlx.Tx.
func NewPostgresCustomerRepository(database sqlxQuerier) *PostgresCustomerRepository {
	return &PostgresCustomerRepository{db: database}
}

var _ db.CustomerRepository = (*PostgresCustomerRepository)(nil)

func (r *PostgresCustomerRepository) Create(customer *db.Customer) error {
	return r.db.QueryRowx(
		`INSERT INTO customers (id, name, email)
		 VALUES ($1, $2, $3)
		 RETURNING created_at, updated_at`,
		customer.ID, customer.Name, customer.Email,
	).Scan(&customer.CreatedAt, &customer.UpdatedAt)
}

func (r *PostgresCustomerRepository) FindByID(id string) (*db.Customer, error) {
	var row customerRow
	err := r.db.Get(&row,
		`SELECT id, name, email, created_at, updated_at FROM customers WHERE id = $1`,
		id,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("FindByID %q: %w", id, db.ErrCustomerNotFound)
	}
	if err != nil {
		return nil, err
	}
	return rowToCustomer(row), nil
}

func (r *PostgresCustomerRepository) FindByEmail(email string) (*db.Customer, error) {
	var row customerRow
	err := r.db.Get(&row,
		`SELECT id, name, email, created_at, updated_at FROM customers WHERE email = $1`,
		email,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("FindByEmail %q: %w", email, db.ErrCustomerNotFound)
	}
	if err != nil {
		return nil, err
	}
	return rowToCustomer(row), nil
}

func (r *PostgresCustomerRepository) List() ([]*db.Customer, error) {
	var rows []customerRow
	if err := r.db.Select(&rows,
		`SELECT id, name, email, created_at, updated_at FROM customers ORDER BY name`,
	); err != nil {
		return nil, fmt.Errorf("List: %w", err)
	}
	customers := make([]*db.Customer, len(rows))
	for i, row := range rows {
		customers[i] = rowToCustomer(row)
	}
	return customers, nil
}
