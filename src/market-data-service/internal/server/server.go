// Package server provides the gRPC-like server and health check for market-data-service.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/garudax-platform/market-data-service/internal/candle"
	"github.com/garudax-platform/market-data-service/internal/retention"
	"github.com/garudax-platform/market-data-service/internal/store"
	"github.com/garudax-platform/market-data-service/internal/streaming"
	"github.com/garudax-platform/market-data-service/internal/ticker"
	"github.com/garudax-platform/market-data-service/internal/types"
)

// Server is the market data service server.
type Server struct {
	config        Config
	tradeStore    store.TradeRepository
	candleStore   store.CandleRepository
	tickerStore   store.TickerRepository
	candleBuilder *candle.Builder
	tickerEngine  *ticker.Engine
	hub           *streaming.Hub
	retention     *retention.Policy
	ready         int32
}

// NewServer creates a new market data server with in-memory stores.
func NewServer(cfg Config) *Server {
	return NewServerWithStores(cfg, nil, nil, nil)
}

// NewServerWithStores creates a new market data server with the given store implementations.
// Pass nil for any store to use the default in-memory implementation.
func NewServerWithStores(cfg Config, tradeRepo store.TradeRepository, candleRepo store.CandleRepository, tickerRepo store.TickerRepository) *Server {
	hub := streaming.NewHub()
	tickerEng := ticker.NewEngine()
	retPolicy := retention.DefaultPolicy()

	if tradeRepo == nil {
		tradeRepo = store.NewTradeStore()
	}
	if candleRepo == nil {
		candleRepo = store.NewCandleStore()
	}
	if tickerRepo == nil {
		tickerRepo = store.NewTickerStore()
	}

	s := &Server{
		config:       cfg,
		tradeStore:   tradeRepo,
		candleStore:  candleRepo,
		tickerStore:  tickerRepo,
		tickerEngine: tickerEng,
		hub:          hub,
		retention:    retPolicy,
	}

	// Wire candle builder to publish updates via hub and persist closed candles
	s.candleBuilder = candle.NewBuilder(func(c types.Candle) {
		hub.PublishCandle(c)
		if c.IsClosed {
			candleRepo.Store(c)
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

	// Persist ticker snapshot to store after each trade
	if tick, ok := s.tickerEngine.GetTicker(trade.InstrumentID); ok {
		s.tickerStore.Upsert(tick)
	}
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

// StartHealthServer starts the HTTP health check and data query server.
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

	// --- Data query endpoints ---

	mux.HandleFunc("/candles", func(w http.ResponseWriter, r *http.Request) {
		instrumentID := r.URL.Query().Get("instrument_id")
		if instrumentID == "" {
			http.Error(w, `{"error":"instrument_id required"}`, http.StatusBadRequest)
			return
		}
		intervalStr := r.URL.Query().Get("interval")
		interval := types.Interval1m
		switch intervalStr {
		case "5m":
			interval = types.Interval5m
		case "15m":
			interval = types.Interval15m
		case "1h":
			interval = types.Interval1h
		case "4h":
			interval = types.Interval4h
		case "1d":
			interval = types.Interval1d
		}
		limitStr := r.URL.Query().Get("limit")
		limit := 100
		if limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
				limit = v
			}
		}
		candles := s.GetCandles(instrumentID, interval, time.Time{}, time.Time{}, limit)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instrument_id": instrumentID,
			"interval":      interval.String(),
			"candles":       candles,
			"count":         len(candles),
		})
	})

	mux.HandleFunc("/ticker", func(w http.ResponseWriter, r *http.Request) {
		instrumentID := r.URL.Query().Get("instrument_id")
		if instrumentID == "" {
			http.Error(w, `{"error":"instrument_id required"}`, http.StatusBadRequest)
			return
		}
		tick, ok := s.GetTicker(instrumentID)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"instrument_id": instrumentID,
				"symbol":        "",
				"last_price":    "0",
				"volume_24h":    0,
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tick)
	})

	mux.HandleFunc("/trades", func(w http.ResponseWriter, r *http.Request) {
		instrumentID := r.URL.Query().Get("instrument_id")
		if instrumentID == "" {
			http.Error(w, `{"error":"instrument_id required"}`, http.StatusBadRequest)
			return
		}
		limitStr := r.URL.Query().Get("limit")
		limit := 50
		if limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
				limit = v
			}
		}
		trades := s.GetTrades(instrumentID, limit, 0, time.Time{}, time.Time{})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"instrument_id": instrumentID,
			"trades":        trades,
			"count":         len(trades),
		})
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
