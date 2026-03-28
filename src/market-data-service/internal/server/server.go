// Package server provides the gRPC-like server and health check for market-data-service.
package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/ace-platform/market-data-service/internal/candle"
	"github.com/ace-platform/market-data-service/internal/retention"
	"github.com/ace-platform/market-data-service/internal/store"
	"github.com/ace-platform/market-data-service/internal/streaming"
	"github.com/ace-platform/market-data-service/internal/ticker"
	"github.com/ace-platform/market-data-service/internal/types"
)

// Server is the market data service server.
type Server struct {
	config       Config
	tradeStore   *store.TradeStore
	candleStore  *store.CandleStore
	candleBuilder *candle.Builder
	tickerEngine *ticker.Engine
	hub          *streaming.Hub
	retention    *retention.Policy
	ready        int32
}

// NewServer creates a new market data server with all components wired together.
func NewServer(cfg Config) *Server {
	hub := streaming.NewHub()
	candleStore := store.NewCandleStore()
	tickerEng := ticker.NewEngine()
	retPolicy := retention.DefaultPolicy()

	s := &Server{
		config:      cfg,
		tradeStore:  store.NewTradeStore(),
		candleStore: candleStore,
		tickerEngine: tickerEng,
		hub:         hub,
		retention:   retPolicy,
	}

	// Wire candle builder to publish updates via hub and persist closed candles
	s.candleBuilder = candle.NewBuilder(func(c types.Candle) {
		hub.PublishCandle(c)
		if c.IsClosed {
			candleStore.Store(c)
		}
	})

	return s
}

// IngestTrade processes a trade through all subsystems.
func (s *Server) IngestTrade(trade types.Trade) {
	s.tradeStore.Append(trade)
	s.candleBuilder.IngestTrade(trade)
	s.tickerEngine.IngestTrade(trade)
	s.hub.PublishTrade(trade)
}

// GetCandles returns historical candles for an instrument.
func (s *Server) GetCandles(instrumentID string, interval types.CandleInterval, start, end time.Time, limit int) []types.Candle {
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	if end.IsZero() {
		end = time.Now().UTC()
	}

	// Fetch from candle store (closed candles)
	candles := s.candleStore.Query(instrumentID, interval, start, end, limit)

	// Append current in-progress candle if it falls within the range
	if current, ok := s.candleBuilder.GetCandle(instrumentID, interval); ok {
		if !current.Bucket.Before(start) && current.Bucket.Before(end) {
			candles = append(candles, current)
		}
	}

	if len(candles) > limit {
		candles = candles[:limit]
	}
	return candles
}

// SubscribeCandles creates a subscription for real-time candle updates.
func (s *Server) SubscribeCandles(instrumentID string, interval types.CandleInterval) *streaming.CandleSubscription {
	return s.hub.SubscribeCandles(instrumentID, 64)
}

// GetTicker returns the ticker for an instrument.
func (s *Server) GetTicker(instrumentID string) (types.Ticker, bool) {
	return s.tickerEngine.GetTicker(instrumentID)
}

// GetTickers returns tickers for multiple instruments.
func (s *Server) GetTickers(instrumentIDs []string) []types.Ticker {
	return s.tickerEngine.GetTickers(instrumentIDs)
}

// GetTrades returns recent trades for an instrument.
func (s *Server) GetTrades(instrumentID string, limit int, sinceSequence uint64, start, end time.Time) []types.Trade {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	if sinceSequence > 0 {
		trades := s.tradeStore.SinceSequence(instrumentID, sinceSequence)
		if len(trades) > limit {
			trades = trades[:limit]
		}
		return trades
	}

	if !start.IsZero() {
		if end.IsZero() {
			end = time.Now().UTC()
		}
		return s.tradeStore.InTimeRange(instrumentID, start, end, limit)
	}

	return s.tradeStore.LastN(instrumentID, limit)
}

// SubscribeTrades creates a subscription for real-time trade updates.
func (s *Server) SubscribeTrades(instrumentID string) *streaming.TradeSubscription {
	return s.hub.SubscribeTrades(instrumentID, 64)
}

// SetSymbol registers a symbol name for an instrument.
func (s *Server) SetSymbol(instrumentID, symbol string) {
	s.tickerEngine.SetSymbol(instrumentID, symbol)
}

// RunRetention runs data retention enforcement once.
func (s *Server) RunRetention() {
	s.retention.Enforce(s.candleStore)
}

// FlushCandles forces flush of all expired candles from builder to store.
func (s *Server) FlushCandles() {
	flushed := s.candleBuilder.FlushClosed(time.Now().UTC())
	for _, c := range flushed {
		s.candleStore.Store(c)
		s.hub.PublishCandle(c)
	}
}

// SetReady marks the server as ready.
func (s *Server) SetReady() {
	atomic.StoreInt32(&s.ready, 1)
}

// IsReady returns true if the server is ready.
func (s *Server) IsReady() bool {
	return atomic.LoadInt32(&s.ready) == 1
}

// StartHealthServer starts the HTTP health check server.
func (s *Server) StartHealthServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	addr := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.HealthPort)
	log.Printf("Health server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// ListenGRPC creates a TCP listener for the gRPC server.
func (s *Server) ListenGRPC() (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", s.config.BindAddress, s.config.GRPCPort)
	log.Printf("gRPC server listening on %s", addr)
	return net.Listen("tcp", addr)
}

// Hub returns the streaming hub (for testing).
func (s *Server) Hub() *streaming.Hub { return s.hub }

// CandleBuilder returns the candle builder (for testing).
func (s *Server) CandleBuilder() *candle.Builder { return s.candleBuilder }
