package repository

import (
	"context"
	"fmt"
	"time"
)

// =============================================================================
//Health Check - checking the status of the connection to the database
// =============================================================================
//
//In production, Kubernetes checks for two types of health:
//
// 1. Liveness Probe (/healthz):
//"Is the application alive?" → if not, Kubernetes restarts the pod.
//DOES NOT check dependencies (DB, Redis). It's just "the process works."
//
// 2. Readiness Probe (/readyz):
//"Is the application ready to receive traffic?" → if not, removes it from balancing.
//CHECKS dependencies: database, Redis, external services.
//
//Example Kubernetes configuration:
//
//	livenessProbe:
//	  httpGet:
//	    path: /healthz
//	    port: 8080
//	  periodSeconds: 10
//
//	readinessProbe:
//	  httpGet:
//	    path: /readyz
//	    port: 8080
//	  periodSeconds: 5
//
//❌ Typical error: liveness checks the database.
//DB crashed → liveness fail → Kubernetes restarts pod →
//pod starts → DB still doesn’t work → restart again → crash loop.
//Correct: readiness checks the database, liveness does not.

//Pinger is an interface for checking the connection to the database.
//👉 *sqlx.DB and *sql.DB implement this interface.
type Pinger interface {
	PingContext(ctx context.Context) error
}

//HealthStatus - the result of a health check.
type HealthStatus struct {
	Healthy  bool          `json:"healthy"`
	Latency  time.Duration `json:"latency_ms"`
	Error    string        `json:"error,omitempty"`
	CheckedAt time.Time   `json:"checked_at"`
}

//HealthChecker checks the state of the database.
type HealthChecker struct {
	db      Pinger
	timeout time.Duration
}

//NewHealthChecker creates a health checker.
//
//	hc := NewHealthChecker(db, 2*time.Second)
//
//// In HTTP handler:
//	status := hc.Check(r.Context())
//	if !status.Healthy {
//	    w.WriteHeader(http.StatusServiceUnavailable)
//	}
func NewHealthChecker(db Pinger, timeout time.Duration) *HealthChecker {
	return &HealthChecker{db: db, timeout: timeout}
}

//Check checks the connection to the database.
//Uses context with a timeout - does not freeze if the database does not respond.
func (hc *HealthChecker) Check(ctx context.Context) HealthStatus {
	ctx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	start := time.Now()
	err := hc.db.PingContext(ctx)
	latency := time.Since(start)

	status := HealthStatus{
		Healthy:   err == nil,
		Latency:   latency,
		CheckedAt: time.Now(),
	}

	if err != nil {
		status.Error = fmt.Sprintf("db ping failed: %v", err)
	}

	return status
}

// =============================================================================
// Connection Pool Configuration
// =============================================================================
//
//database/sql supports connection pooling out of the box.
//The settings are critical for production:
//
//db.SetMaxOpenConns(25) // Max open connections
//db.SetMaxIdleConns(10) // Max idle (ready to use)
//db.SetConnMaxLifetime(5m) // Max connection lifetime
//db.SetConnMaxIdleTime(1m) // Max idle time before closing
//
//Rules:
//- MaxOpenConns < PostgreSQL limit (default: 100)
//- MaxIdleConns ≤ MaxOpenConns (otherwise meaningless)
//- ConnMaxLifetime: to keep connections updated (DNS changes, failover)
//- ConnMaxIdleTime: to prevent idle connections from hanging forever
//
//❌ Typical error: MaxOpenConns = 0 (unlimited).
//Hundreds of connections open under load → PostgreSQL crashes.
//
//🏭 In production: settings from env variables or config file.

//PoolConfig - connection pool settings.
type PoolConfig struct {
	MaxOpenConns    int           //Maximum open connections to the database
	MaxIdleConns    int           //Maximum idle connections in the pool
	ConnMaxLifetime time.Duration //Maximum connection lifetime
	ConnMaxIdleTime time.Duration //Maximum connection downtime
}

//DefaultPoolConfig - Reasonable default values ​​for a typical service.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}
}
