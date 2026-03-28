// Package streaming provides a pub/sub hub for real-time candle and trade updates.
package streaming

import (
	"sync"

	"github.com/ace-platform/market-data-service/internal/types"
)

// CandleSubscription receives candle updates.
type CandleSubscription struct {
	Ch     chan types.Candle
	cancel func()
}

// Close unsubscribes and closes the channel.
func (s *CandleSubscription) Close() {
	s.cancel()
}

// TradeSubscription receives trade updates.
type TradeSubscription struct {
	Ch     chan types.Trade
	cancel func()
}

// Close unsubscribes and closes the channel.
func (s *TradeSubscription) Close() {
	s.cancel()
}

// Hub manages subscriptions for candle and trade updates.
type Hub struct {
	mu             sync.RWMutex
	candleSubs     map[string]map[*CandleSubscription]struct{} // instrumentID -> subs
	tradeSubs      map[string]map[*TradeSubscription]struct{}  // instrumentID -> subs
}

// NewHub creates a new streaming hub.
func NewHub() *Hub {
	return &Hub{
		candleSubs: make(map[string]map[*CandleSubscription]struct{}),
		tradeSubs:  make(map[string]map[*TradeSubscription]struct{}),
	}
}

// SubscribeCandles creates a candle subscription for an instrument.
func (h *Hub) SubscribeCandles(instrumentID string, bufSize int) *CandleSubscription {
	h.mu.Lock()
	defer h.mu.Unlock()

	if bufSize <= 0 {
		bufSize = 64
	}

	sub := &CandleSubscription{
		Ch: make(chan types.Candle, bufSize),
	}
	sub.cancel = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs, ok := h.candleSubs[instrumentID]; ok {
			delete(subs, sub)
		}
		close(sub.Ch)
	}

	if _, ok := h.candleSubs[instrumentID]; !ok {
		h.candleSubs[instrumentID] = make(map[*CandleSubscription]struct{})
	}
	h.candleSubs[instrumentID][sub] = struct{}{}

	return sub
}

// SubscribeTrades creates a trade subscription for an instrument.
func (h *Hub) SubscribeTrades(instrumentID string, bufSize int) *TradeSubscription {
	h.mu.Lock()
	defer h.mu.Unlock()

	if bufSize <= 0 {
		bufSize = 64
	}

	sub := &TradeSubscription{
		Ch: make(chan types.Trade, bufSize),
	}
	sub.cancel = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs, ok := h.tradeSubs[instrumentID]; ok {
			delete(subs, sub)
		}
		close(sub.Ch)
	}

	if _, ok := h.tradeSubs[instrumentID]; !ok {
		h.tradeSubs[instrumentID] = make(map[*TradeSubscription]struct{})
	}
	h.tradeSubs[instrumentID][sub] = struct{}{}

	return sub
}

// PublishCandle sends a candle update to all subscribers for the instrument.
func (h *Hub) PublishCandle(c types.Candle) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subs := h.candleSubs[c.InstrumentID]
	for sub := range subs {
		select {
		case sub.Ch <- c:
		default:
			// Drop if subscriber is slow — prevents blocking the ingestion pipeline
		}
	}
}

// PublishTrade sends a trade update to all subscribers for the instrument.
func (h *Hub) PublishTrade(t types.Trade) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subs := h.tradeSubs[t.InstrumentID]
	for sub := range subs {
		select {
		case sub.Ch <- t:
		default:
			// Drop if subscriber is slow
		}
	}
}
