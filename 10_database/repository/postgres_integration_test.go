package repository_test

//Integration test of PostgreSQL repository.
//
//❗ REQUIRES POSTGRES TO BE RUNNING:
//   docker compose -f 10_database/docker-compose.yml up -d
//
//The test itself rolls out migrations, runs scripts and rolls back the scheme.
//
//Techniques demonstrated here:
//1. Connect to real PostgreSQL via sqlx
//2. Rolling over migrations before tests (golang-migrate)
//3. Transactional isolation of tests (each test = its own transaction + rollback)
//4. CRUD operations: Create, FindByID, FindByCustomerID, FindByStatus
//5. Soft delete - the record “disappears” for requests after SoftDelete
//6. Optimistic locking - ErrVersionConflict during parallel update
//7. JOIN request - ListWithItems returns orders with positions
//8. N+1 problem and how it is solved through an IN request

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	db "learning_path/10_database"
	"learning_path/10_database/repository"
)

// ─────────────────────────────────────────────────────────────────
//CONFIGURATION of connection to the test database.
//Read from env or use values ​​from docker-compose.yml.
// ─────────────────────────────────────────────────────────────────

func testDSN() string {
	if dsn := os.Getenv("TEST_DB_DSN"); dsn != "" {
		return dsn
	}
	//Values ​​from 10_database/docker-compose.yml
	return "host=localhost port=5433 user=orders_user password=orders_pass dbname=orders_db sslmode=disable"
}

// ─────────────────────────────────────────────────────────────────
//RepositorySuite - Test Suite for integration tests.
//
//SetupSuite: connection + migrations (one time)
//SetupTest: start a transaction (each test)
//TeardownTest: ROLLBACK transactions (every test is a blank slate!)
//TeardownSuite: rolling back migrations
// ─────────────────────────────────────────────────────────────────

type RepositorySuite struct {
	suite.Suite

	sqlxDB   *sqlx.DB
	migrator *migrate.Migrate
	tx       *sqlx.Tx //current test transaction

	//Repositories running inside a transaction
	orders    *repository.PostgresOrderRepository
	customers *repository.PostgresCustomerRepository
}

//SetupSuite - Called ONCE before all tests.
func (s *RepositorySuite) SetupSuite() {
	t := s.T()

	//Connecting to PostgreSQL
	sqlxDB, err := sqlx.Connect("postgres", testDSN())
	if err != nil {
		t.Skipf("PostgreSQL is unavailable (%v). Run: docker compose -f 10_database/docker-compose.yml up -d", err)
		return
	}
	s.sqlxDB = sqlxDB

	//Setting up a connection pool
	sqlxDB.SetMaxOpenConns(5)
	sqlxDB.SetMaxIdleConns(2)
	sqlxDB.SetConnMaxLifetime(5 * time.Minute)

	//Roll-up of migrations
	s.runMigrations(t)
	t.Log("✓ Connection to PostgreSQL has been established, migrations have been completed")
}

//TearDownSuite - Called ONCE after all tests.
func (s *RepositorySuite) TearDownSuite() {
	if s.migrator != nil {
		//Rolling back the database schema
		_ = s.migrator.Down()
	}
	if s.sqlxDB != nil {
		_ = s.sqlxDB.Close()
	}
}

//SetupTest - Called BEFORE each test.
//👉 We start a transaction - each test runs in an isolated transaction.
func (s *RepositorySuite) SetupTest() {
	var err error
	s.tx, err = s.sqlxDB.Beginx()
	s.Require().NoError(err, "failed to start transaction")

	//Repositories receive tx instead of db - all operations go into a transaction
	s.orders = repository.NewPostgresOrderRepository(s.tx)
	s.customers = repository.NewPostgresCustomerRepository(s.tx)
}

//TearDownTest - Called AFTER each test.
//👉 ROLLBACK - all test changes are rolled back! The next test is a blank slate.
func (s *RepositorySuite) TearDownTest() {
	if s.tx != nil {
		_ = s.tx.Rollback()
	}
}

//runMigrations runs SQL migrations.
func (s *RepositorySuite) runMigrations(t *testing.T) {
	//Find the path to the migrations folder relative to this file
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "migrations")
	migrationsURL := fmt.Sprintf("file://%s", migrationsPath)

	driver, err := migratepg.WithInstance(s.sqlxDB.DB, &migratepg.Config{})
	require.NoError(t, err, "failed to create migrate driver")

	m, err := migrate.NewWithDatabaseInstance(migrationsURL, "postgres", driver)
	require.NoError(t, err, "failed to create migrator")

	s.migrator = m

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		require.NoError(t, err, "failed to roll out migrations")
	}
}

// ─────────────────────────────────────────────────────────────────
//Helpers - create test data
// ─────────────────────────────────────────────────────────────────

func (s *RepositorySuite) createTestCustomer(name, email string) *db.Customer {
	s.T().Helper()
	c := &db.Customer{
		ID:    fmt.Sprintf("cust-%d", time.Now().UnixNano()),
		Name:  name,
		Email: email,
	}
	s.Require().NoError(s.customers.Create(c))
	return c
}

func (s *RepositorySuite) createTestOrder(customerID string, items ...db.OrderItem) *db.Order {
	s.T().Helper()
	if len(items) == 0 {
		items = []db.OrderItem{
			{ID: fmt.Sprintf("item-%d", time.Now().UnixNano()), ProductID: "prod-1", Name: "Widget", Quantity: 2, UnitPrice: 10.50},
		}
	}
	order := &db.Order{
		ID:         fmt.Sprintf("ord-%d", time.Now().UnixNano()),
		CustomerID: customerID,
		Status:     db.StatusPending,
		Items:      items,
	}
	order.TotalAmount = order.Total()
	s.Require().NoError(s.orders.Create(order))
	return order
}

// ═══════════════════════════════════════════════════════════════
//CUSTOMERS TESTS
// ═══════════════════════════════════════════════════════════════

func (s *RepositorySuite) TestCustomer_CreateAndFind() {
	// Create
	c := s.createTestCustomer("Ivan Ivanov", "ivan@test.com")
	s.NotEmpty(c.ID)
	s.False(c.CreatedAt.IsZero(), "The database must fill created_at")

	// FindByID
	found, err := s.customers.FindByID(c.ID)
	s.Require().NoError(err)
	s.Equal(c.ID, found.ID)
	s.Equal("Ivan Ivanov", found.Name)
	s.Equal("ivan@test.com", found.Email)
}

func (s *RepositorySuite) TestCustomer_FindByEmail() {
	c := s.createTestCustomer("Maria", "maria@test.com")

	found, err := s.customers.FindByEmail("maria@test.com")
	s.Require().NoError(err)
	s.Equal(c.ID, found.ID)
}

func (s *RepositorySuite) TestCustomer_NotFound() {
	_, err := s.customers.FindByID("non-existent-id")
	s.Require().Error(err)
	s.True(errors.Is(err, db.ErrCustomerNotFound))
}

func (s *RepositorySuite) TestCustomer_List() {
	s.createTestCustomer("Alice", "alice@test.com")
	s.createTestCustomer("Bob", "bob@test.com")
	s.createTestCustomer("Charlie", "charlie@test.com")

	customers, err := s.customers.List()
	s.Require().NoError(err)
	s.GreaterOrEqual(len(customers), 3, "must have at least 3 clients")
}

// ═══════════════════════════════════════════════════════════════
//ORDERS - CRUD TESTS
// ═══════════════════════════════════════════════════════════════

func (s *RepositorySuite) TestOrder_CreateAndFindByID() {
	cust := s.createTestCustomer("Test of Tests", fmt.Sprintf("test-%d@example.com", time.Now().UnixNano()))

	//Create an order with two items
	items := []db.OrderItem{
		{ID: fmt.Sprintf("item-a-%d", time.Now().UnixNano()), ProductID: "iphone", Name: "iPhone 15", Quantity: 1, UnitPrice: 99900},
		{ID: fmt.Sprintf("item-b-%d", time.Now().UnixNano()), ProductID: "case", Name: "Case", Quantity: 2, UnitPrice: 1500},
	}
	order := s.createTestOrder(cust.ID, items...)

	//FindByID - should return the order with positions
	found, err := s.orders.FindByID(order.ID)
	s.Require().NoError(err)

	//Checking the order
	s.Equal(order.ID, found.ID)
	s.Equal(cust.ID, found.CustomerID)
	s.Equal(db.StatusPending, found.Status)
	s.Equal(102900.0, found.TotalAmount) // 99900 + 2*1500
	s.Equal(1, found.Version)
	s.False(found.CreatedAt.IsZero())

	//Checking positions
	s.Len(found.Items, 2)
	//Items sorted by name
	names := []string{found.Items[0].Name, found.Items[1].Name}
	s.Contains(names, "iPhone 15")
	s.Contains(names, "Case")
}

func (s *RepositorySuite) TestOrder_NotFound() {
	_, err := s.orders.FindByID("totally-fake-order-id")
	s.Require().Error(err)
	s.True(errors.Is(err, db.ErrOrderNotFound))
}

func (s *RepositorySuite) TestOrder_FindByCustomerID() {
	cust1 := s.createTestCustomer("Client 1", fmt.Sprintf("c1-%d@test.com", time.Now().UnixNano()))
	cust2 := s.createTestCustomer("Client 2", fmt.Sprintf("c2-%d@test.com", time.Now().UnixNano()))

	//We create 2 orders for cust1 and 1 for cust2
	s.createTestOrder(cust1.ID)
	time.Sleep(time.Millisecond) //UnixNano can match
	s.createTestOrder(cust1.ID)
	s.createTestOrder(cust2.ID)

	orders1, err := s.orders.FindByCustomerID(cust1.ID)
	s.Require().NoError(err)
	s.Len(orders1, 2, "Customer 1 must have 2 orders")

	orders2, err := s.orders.FindByCustomerID(cust2.ID)
	s.Require().NoError(err)
	s.Len(orders2, 1, "Customer 2 must have 1 order")
}

func (s *RepositorySuite) TestOrder_FindByStatus() {
	cust := s.createTestCustomer("Status Test", fmt.Sprintf("st-%d@test.com", time.Now().UnixNano()))

	pending := s.createTestOrder(cust.ID)

	//We confirm one order
	pending.Status = db.StatusConfirmed
	s.Require().NoError(s.orders.Update(pending))

	//Create another pending
	s.createTestOrder(cust.ID)

	confirmed, err := s.orders.FindByStatus(db.StatusConfirmed)
	s.Require().NoError(err)
	s.GreaterOrEqual(len(confirmed), 1)
	for _, o := range confirmed {
		s.Equal(db.StatusConfirmed, o.Status)
	}
}

// ═══════════════════════════════════════════════════════════════
//ORDERS TESTS - UPDATE (Optimistic Locking)
// ═══════════════════════════════════════════════════════════════

func (s *RepositorySuite) TestOrder_Update_Success() {
	cust := s.createTestCustomer("Update User", fmt.Sprintf("upd-%d@test.com", time.Now().UnixNano()))
	order := s.createTestOrder(cust.ID)

	s.Equal(1, order.Version)
	s.Equal(db.StatusPending, order.Status)

	//Update the status
	order.Status = db.StatusConfirmed
	order.Notes = "Paid by card"
	err := s.orders.Update(order)
	s.Require().NoError(err)

	//Version should increase
	s.Equal(2, order.Version)

	//Checking in the database
	fetched, err := s.orders.FindByID(order.ID)
	s.Require().NoError(err)
	s.Equal(db.StatusConfirmed, fetched.Status)
	s.Equal("Paid by card", fetched.Notes)
	s.Equal(2, fetched.Version)
}

func (s *RepositorySuite) TestOrder_Update_OptimisticLock() {
	//👉 OPTIMISTIC LOCKING: if someone has already updated the entry,
	//the second UPDATE will fail with ErrVersionConflict.
	//
	//This is protection against loss of updates during parallel requests.
	cust := s.createTestCustomer("OL User", fmt.Sprintf("ol-%d@test.com", time.Now().UnixNano()))
	order := s.createTestOrder(cust.ID)

	//“Reading” the order twice—simulating two users
	order1, _ := s.orders.FindByID(order.ID)
	order2, _ := s.orders.FindByID(order.ID)

	//The first user updates - successfully
	order1.Status = db.StatusConfirmed
	err := s.orders.Update(order1)
	s.Require().NoError(err, "the first Update must pass")
	s.Equal(2, order1.Version) //version became 2

	//The second user tries to update with version=1 - it should crash!
	order2.Status = db.StatusCancelled
	err = s.orders.Update(order2)
	s.Require().Error(err)
	s.True(errors.Is(err, db.ErrVersionConflict),
		"there must be an optimistic lock error, got: %v", err)
}

// ═══════════════════════════════════════════════════════════════
//TESTS ORDERS - SOFT DELETE
// ═══════════════════════════════════════════════════════════════

func (s *RepositorySuite) TestOrder_SoftDelete() {
	//👉 SOFT DELETE: the record remains in the database but “disappears” for queries.
	cust := s.createTestCustomer("Del User", fmt.Sprintf("del-%d@test.com", time.Now().UnixNano()))
	order := s.createTestOrder(cust.ID)

	//Checking that the order exists
	_, err := s.orders.FindByID(order.ID)
	s.Require().NoError(err)

	//Gently remove
	err = s.orders.SoftDelete(order.ID)
	s.Require().NoError(err)

	//Now FindByID should return ErrOrderNotFound
	//(although there is still a line in the database - just deleted_at != NULL)
	_, err = s.orders.FindByID(order.ID)
	s.Require().Error(err)
	s.True(errors.Is(err, db.ErrOrderNotFound),
		"the deleted order must be 'invisible' to FindByID")

	//Repeated SoftDelete is also an error
	err = s.orders.SoftDelete(order.ID)
	s.Require().Error(err)
}

// ═══════════════════════════════════════════════════════════════
//TESTS ORDERS - JOIN QUERY
// ═══════════════════════════════════════════════════════════════

func (s *RepositorySuite) TestOrder_ListWithItems_JoinQuery() {
	//👉 JOIN: one SELECT returns orders + positions.
	//We check that the “reversal” of JOIN rows works correctly.
	cust := s.createTestCustomer("Join User", fmt.Sprintf("join-%d@test.com", time.Now().UnixNano()))

	//Order with 3 items
	items := []db.OrderItem{
		{ID: fmt.Sprintf("ji1-%d", time.Now().UnixNano()), ProductID: "p1", Name: "Product A", Quantity: 1, UnitPrice: 100},
		{ID: fmt.Sprintf("ji2-%d", time.Now().UnixNano()), ProductID: "p2", Name: "Product B", Quantity: 2, UnitPrice: 50},
		{ID: fmt.Sprintf("ji3-%d", time.Now().UnixNano()), ProductID: "p3", Name: "Product B", Quantity: 3, UnitPrice: 25},
	}
	order := s.createTestOrder(cust.ID, items...)

	orders, err := s.orders.ListWithItems()
	s.Require().NoError(err)

	//We find our order as a result
	var found *db.Order
	for _, o := range orders {
		if o.ID == order.ID {
			found = o
			break
		}
	}
	s.Require().NotNil(found, "our order should be on the list")
	s.Len(found.Items, 3, "there should be 3 positions")
	s.Equal(275.0, found.TotalAmount) // 1*100 + 2*50 + 3*25 = 275
}

// ─────────────────────────────────────────────────────────────────
//TestRepositorySuite - entry point
// ─────────────────────────────────────────────────────────────────

func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(RepositorySuite))
}

// ─────────────────────────────────────────────────────────────────
//Standalone tests without Suite - for quick connection check
// ─────────────────────────────────────────────────────────────────

func TestPostgresConnection(t *testing.T) {
	conn, err := sqlx.Connect("postgres", testDSN())
	if err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	defer conn.Close()

	var version string
	err = conn.Get(&version, "SELECT version()")
	require.NoError(t, err)
	assert.Contains(t, version, "PostgreSQL")
	t.Logf("✓ Connected to: %s", version[:30])
}
