package proxy

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// ConnPool manages a pool of TCP connections to a backend service.
type ConnPool struct {
	mu      sync.Mutex
	addr    string
	conns   []net.Conn
	maxIdle int
	timeout time.Duration
}

// NewConnPool creates a connection pool for a backend address.
func NewConnPool(addr string, maxIdle int, timeout time.Duration) *ConnPool {
	return &ConnPool{
		addr:    addr,
		maxIdle: maxIdle,
		timeout: timeout,
	}
}

// Get retrieves a connection from the pool or dials a new one.
func (p *ConnPool) Get() (net.Conn, error) {
	p.mu.Lock()
	if len(p.conns) > 0 {
		conn := p.conns[len(p.conns)-1]
		p.conns = p.conns[:len(p.conns)-1]
		p.mu.Unlock()
		return conn, nil
	}
	p.mu.Unlock()

	return net.DialTimeout("tcp", p.addr, p.timeout)
}

// Put returns a connection to the pool. If the pool is full, the connection is closed.
func (p *ConnPool) Put(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.conns) >= p.maxIdle {
		conn.Close()
		return
	}
	p.conns = append(p.conns, conn)
}

// Close closes all pooled connections.
func (p *ConnPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = nil
}

// BackendPool manages connection pools for all backend services.
type BackendPool struct {
	pools map[string]*ConnPool
}

// NewBackendPool creates pools for all configured backends.
func NewBackendPool(backends map[string]string, maxIdlePerBackend int, dialTimeout time.Duration) *BackendPool {
	bp := &BackendPool{
		pools: make(map[string]*ConnPool, len(backends)),
	}
	for name, addr := range backends {
		bp.pools[name] = NewConnPool(addr, maxIdlePerBackend, dialTimeout)
	}
	return bp
}

// Get returns a connection from the named backend pool.
func (bp *BackendPool) Get(backend string) (net.Conn, error) {
	pool, ok := bp.pools[backend]
	if !ok {
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
	return pool.Get()
}

// Put returns a connection to the named backend pool.
func (bp *BackendPool) Put(backend string, conn net.Conn) {
	if pool, ok := bp.pools[backend]; ok {
		pool.Put(conn)
	} else {
		conn.Close()
	}
}

// Close closes all backend pools.
func (bp *BackendPool) Close() {
	for _, pool := range bp.pools {
		pool.Close()
	}
}
