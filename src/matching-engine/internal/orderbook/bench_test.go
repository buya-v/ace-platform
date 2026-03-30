package orderbook

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// benchIDGen is a fast ID generator for benchmarks.
type benchIDGen struct {
	counter uint64
}

func (g *benchIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("b-%d", n)
}

// BenchmarkMatchingThroughput measures orders/sec for the matching engine.
// Target: >10,000 orders/sec.
func BenchmarkMatchingThroughput(b *testing.B) {
	var seq uint64
	book := NewOrderBook("BENCH", &benchIDGen{}, &seq)

	price := mustParseDecimal("100.00")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Alternate: place sell, then match with buy
		sell := &types.Order{
			OrderID:      fmt.Sprintf("s-%d", i),
			InstrumentID: "BENCH",
			AccountID:    fmt.Sprintf("seller-%d", i),
			Side:         types.SideSell,
			OrderType:    types.OrderTypeLimit,
			TimeInForce:  types.TIFDay,
			Price:        price,
			Quantity:     1,
		}
		book.SubmitOrder(sell)

		buy := &types.Order{
			OrderID:      fmt.Sprintf("b-%d", i),
			InstrumentID: "BENCH",
			AccountID:    fmt.Sprintf("buyer-%d", i),
			Side:         types.SideBuy,
			OrderType:    types.OrderTypeLimit,
			TimeInForce:  types.TIFDay,
			Price:        price,
			Quantity:     1,
		}
		book.SubmitOrder(buy)
	}
}

// BenchmarkOrderPlacement measures the throughput of placing resting orders.
func BenchmarkOrderPlacement(b *testing.B) {
	var seq uint64
	book := NewOrderBook("BENCH", &benchIDGen{}, &seq)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Place orders at different prices to build a deep book
		priceRaw := int64(10000 + (i % 1000)) // 1.0000 to 1.0999
		order := &types.Order{
			OrderID:      fmt.Sprintf("o-%d", i),
			InstrumentID: "BENCH",
			AccountID:    "acc",
			Side:         types.SideBuy,
			OrderType:    types.OrderTypeLimit,
			TimeInForce:  types.TIFDay,
			Price:        types.DecimalFromRaw(priceRaw * 10000),
			Quantity:     1,
		}
		book.SubmitOrder(order)
	}
}

// BenchmarkMatchLatency measures per-match latency.
func BenchmarkMatchLatency(b *testing.B) {
	var seq uint64
	price := mustParseDecimal("100.00")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		book := NewOrderBook("BENCH", &benchIDGen{}, &seq)
		// Pre-populate with 100 sell orders
		for j := 0; j < 100; j++ {
			sell := &types.Order{
				OrderID:      fmt.Sprintf("s-%d-%d", i, j),
				InstrumentID: "BENCH",
				AccountID:    fmt.Sprintf("seller-%d", j),
				Side:         types.SideSell,
				OrderType:    types.OrderTypeLimit,
				TimeInForce:  types.TIFDay,
				Price:        price,
				Quantity:     1,
			}
			book.SubmitOrder(sell)
		}
		b.StartTimer()

		// Match against all
		buy := &types.Order{
			OrderID:      fmt.Sprintf("b-%d", i),
			InstrumentID: "BENCH",
			AccountID:    "buyer",
			Side:         types.SideBuy,
			OrderType:    types.OrderTypeMarket,
			TimeInForce:  types.TIFDay,
			Quantity:     100,
		}
		book.SubmitOrder(buy)
	}
}
