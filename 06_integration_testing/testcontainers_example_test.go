//go:build integration

package integration_test

// =============================================================================
// Testcontainers — a real DB from your test via Docker
// =============================================================================
//
// How to run (requires Docker):
//   go test -tags=integration ./06_integration_testing/... -v
//
// 🏭 In production: testcontainers-go boots PostgreSQL inside a Docker container,
//    runs migrations, runs tests, stops the container.
//
// Benefits:
//   - Test real SQL (not an in-memory imitation)
//   - Every run uses a clean DB
//   - CI/CD: Docker is available → tests work
//   - Isolation: each test can have its own container
//
// The `integration` build tag means these tests do NOT run under `go test ./...`.
// Only explicit: go test -tags=integration
//
// Example layout (pseudocode — requires testcontainers-go in go.mod):
//
//   func TestWithPostgres(t *testing.T) {
//       ctx := context.Background()
//
//       // 1. Start PostgreSQL in Docker
//       container, err := postgres.Run(ctx,
//           "postgres:15-alpine",
//           postgres.WithDatabase("testdb"),
//           postgres.WithUsername("test"),
//           postgres.WithPassword("test"),
//           testcontainers.WithWaitStrategy(
//               wait.ForLog("database system is ready to accept connections").
//                   WithOccurrence(2).WithStartupTimeout(5*time.Second),
//           ),
//       )
//       require.NoError(t, err)
//       t.Cleanup(func() { container.Terminate(ctx) })
//
//       // 2. Get the connection string
//       connStr, err := container.ConnectionString(ctx, "sslmode=disable")
//       require.NoError(t, err)
//
//       // 3. Connect
//       db, err := sqlx.Connect("postgres", connStr)
//       require.NoError(t, err)
//       t.Cleanup(func() { db.Close() })
//
//       // 4. Apply migrations
//       runMigrations(t, db)
//
//       // 5. Test!
//       repo := repository.NewPostgresOrderRepository(db)
//       // ... ordinary tests against the real DB ...
//   }
//
// Isolation strategies:
//
//   1. A container per Suite:
//      SetupSuite() → start the container
//      SetupTest()  → BEGIN a transaction
//      TearDownTest() → ROLLBACK
//      TearDownSuite() → stop the container
//      👉 Fast, but tests can see each other's uncommitted data
//
//   2. A container per test:
//      Slow, but full isolation.
//      Fine for < 10 tests.
//
//   3. TRUNCATE between tests (the sweet spot):
//      SetupTest() → TRUNCATE ALL TABLES
//      Faster than rollback when there are many INSERTs.

import "testing"

// TestPlaceholder_IntegrationTagWorks verifies the build tag is in effect.
// This test is only visible with: go test -tags=integration
func TestPlaceholder_IntegrationTagWorks(t *testing.T) {
	t.Log("✓ Build tag 'integration' works! This test does not run under go test ./...")
	t.Log("  For real usage add testcontainers-go to go.mod")
	t.Log("  and replace this placeholder with a real PostgreSQL-backed test.")
}
