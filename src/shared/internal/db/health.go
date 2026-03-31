package db

import (
	"context"
	"fmt"
	"time"
)

// HealthStatus represents the result of a database health check.
type HealthStatus struct {
	Healthy  bool          `json:"healthy"`
	Latency  time.Duration `json:"latency_ms"`
	Error    string        `json:"error,omitempty"`
	PoolStat PoolStats     `json:"pool"`
}

// PoolStats exposes connection pool statistics for monitoring.
type PoolStats struct {
	TotalConns     int32 `json:"total_conns"`
	IdleConns      int32 `json:"idle_conns"`
	AcquiredConns  int32 `json:"acquired_conns"`
	MaxConns       int32 `json:"max_conns"`
	AcquireCount   int64 `json:"acquire_count"`
	EmptyAcquires  int64 `json:"empty_acquires"`
}

// HealthCheck performs a ping against the database and returns a HealthStatus
// with latency and pool statistics. The provided context controls the
// timeout of the ping operation.
func (p *Pool) HealthCheck(ctx context.Context) HealthStatus {
	start := time.Now()
	err := p.pool.Ping(ctx)
	latency := time.Since(start)

	stat := p.pool.Stat()
	ps := PoolStats{
		TotalConns:    stat.TotalConns(),
		IdleConns:     stat.IdleConns(),
		AcquiredConns: stat.AcquiredConns(),
		MaxConns:      stat.MaxConns(),
		AcquireCount:  stat.AcquireCount(),
		EmptyAcquires: stat.EmptyAcquireCount(),
	}

	if err != nil {
		return HealthStatus{
			Healthy:  false,
			Latency:  latency,
			Error:    fmt.Sprintf("ping failed: %v", err),
			PoolStat: ps,
		}
	}

	return HealthStatus{
		Healthy:  true,
		Latency:  latency,
		PoolStat: ps,
	}
}
