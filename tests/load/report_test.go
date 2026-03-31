package load

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/garudax-platform/tests/load/engine"
)

// TestBenchmarkSummaryReport runs a fixed set of benchmarks and prints a
// formatted summary table. This is not a benchmark itself but a convenient
// way to get a single-run performance snapshot.
//
// Run with: go test -run TestBenchmarkSummaryReport -v -timeout 120s
func TestBenchmarkSummaryReport(t *testing.T) {
	results := make([]benchResult, 0)

	// --- Matching Engine Benchmarks ---

	// 1. Single instrument throughput
	{
		const ops = 10000
		var seq uint64
		idGen := &engine.SeqIDGen{}
		book := engine.NewOrderBook("WHEAT-BENCH", idGen, &seq)
		price := engine.MustParseDecimal("425.50")

		start := time.Now()
		trades := 0
		for i := 0; i < ops; i++ {
			sell := &engine.Order{
				OrderID: fmt.Sprintf("s-%d", i), InstrumentID: "WHEAT-BENCH",
				AccountID: fmt.Sprintf("seller-%d", i), ParticipantID: fmt.Sprintf("P-S-%d", i),
				Side: engine.SideSell, OrderType: engine.OrderTypeLimit,
				TimeInForce: engine.TIFDay, Price: price, Quantity: 10,
			}
			book.SubmitOrder(sell)
			buy := &engine.Order{
				OrderID: fmt.Sprintf("b-%d", i), InstrumentID: "WHEAT-BENCH",
				AccountID: fmt.Sprintf("buyer-%d", i), ParticipantID: fmt.Sprintf("P-B-%d", i),
				Side: engine.SideBuy, OrderType: engine.OrderTypeLimit,
				TimeInForce: engine.TIFDay, Price: price, Quantity: 10,
			}
			result := book.SubmitOrder(buy)
			trades += len(result.Trades)
		}
		elapsed := time.Since(start)
		opsPerSec := float64(ops) / elapsed.Seconds()
		results = append(results, benchResult{
			Name: "Matching: single instrument throughput", Ops: ops,
			Duration: elapsed, OpsPerSec: opsPerSec,
			AvgLatency: elapsed / time.Duration(ops),
		})
		if trades != ops {
			t.Errorf("expected %d trades, got %d", ops, trades)
		}
	}

	// 2. Deep book sweep (10K orders)
	{
		var seq uint64
		idGen := &engine.SeqIDGen{}
		book := engine.NewOrderBook("DEEP-BENCH", idGen, &seq)
		basePrice := engine.MustParseDecimal("1200.00")
		totalQty := uint64(0)
		for level := 0; level < 100; level++ {
			lp := engine.DecimalFromRaw(basePrice.Raw() + int64(level*100))
			for j := 0; j < 100; j++ {
				sell := &engine.Order{
					OrderID: fmt.Sprintf("d-%d-%d", level, j), InstrumentID: "DEEP-BENCH",
					AccountID: fmt.Sprintf("mm-%d-%d", level, j), ParticipantID: fmt.Sprintf("P-%d-%d", level, j),
					Side: engine.SideSell, OrderType: engine.OrderTypeLimit,
					TimeInForce: engine.TIFGTC, Price: lp, Quantity: 10,
				}
				book.SubmitOrder(sell)
				totalQty += 10
			}
		}

		start := time.Now()
		sweep := &engine.Order{
			OrderID: "sweep-0", InstrumentID: "DEEP-BENCH",
			AccountID: "aggressor", ParticipantID: "P-AGGR",
			Side: engine.SideBuy, OrderType: engine.OrderTypeMarket,
			TimeInForce: engine.TIFDay, Quantity: totalQty,
		}
		result := book.SubmitOrder(sweep)
		elapsed := time.Since(start)
		results = append(results, benchResult{
			Name: "Matching: 10K order deep book sweep", Ops: len(result.Trades),
			Duration: elapsed, OpsPerSec: float64(len(result.Trades)) / elapsed.Seconds(),
			AvgLatency: elapsed / time.Duration(len(result.Trades)),
		})
	}

	// 3. Cancel storm (5000 orders)
	{
		const n = 5000
		var seq uint64
		idGen := &engine.SeqIDGen{}
		book := engine.NewOrderBook("CANCEL-BENCH", idGen, &seq)
		orderIDs := make([]string, n)
		for j := 0; j < n; j++ {
			oid := fmt.Sprintf("c-%d", j)
			orderIDs[j] = oid
			price := engine.DecimalFromRaw(int64(1000000 + j*100))
			order := &engine.Order{
				OrderID: oid, InstrumentID: "CANCEL-BENCH",
				AccountID: fmt.Sprintf("acc-%d", j), ParticipantID: fmt.Sprintf("P-%d", j),
				Side: engine.SideBuy, OrderType: engine.OrderTypeLimit,
				TimeInForce: engine.TIFGTC, Price: price, Quantity: 10,
			}
			book.SubmitOrder(order)
		}
		start := time.Now()
		for _, oid := range orderIDs {
			if _, err := book.CancelOrder(oid); err != nil {
				t.Fatalf("cancel failed: %v", err)
			}
		}
		elapsed := time.Since(start)
		results = append(results, benchResult{
			Name: "Matching: 5K cancel storm", Ops: n,
			Duration: elapsed, OpsPerSec: float64(n) / elapsed.Seconds(),
			AvgLatency: elapsed / time.Duration(n),
		})
	}

	// --- Clearing Engine Benchmarks ---

	// 4. Novation throughput
	{
		const ops = 10000
		idGen := &engine.SeqIDGen{}
		novSvc := engine.NewNovationService(idGen)
		price := engine.MustParseDecimal("425.50")

		start := time.Now()
		for i := 0; i < ops; i++ {
			trade := engine.Trade{
				TradeID: fmt.Sprintf("t-%d", i), InstrumentID: "WHEAT-2026Q3",
				BuyOrderID: fmt.Sprintf("bo-%d", i), SellOrderID: fmt.Sprintf("so-%d", i),
				BuyerParticipantID: fmt.Sprintf("P-%d", i%50), SellerParticipantID: fmt.Sprintf("P-%d", (i+25)%50),
				Price: price, Quantity: 10, TradeValue: price.MulUint64(10),
				AggressorSide: engine.SideBuy, SequenceNumber: uint64(i), ExecutedAt: time.Now(),
			}
			if _, err := novSvc.Novate(trade); err != nil {
				t.Fatalf("novation failed: %v", err)
			}
		}
		elapsed := time.Since(start)
		results = append(results, benchResult{
			Name: "Clearing: novation throughput", Ops: ops,
			Duration: elapsed, OpsPerSec: float64(ops) / elapsed.Seconds(),
			AvgLatency: elapsed / time.Duration(ops),
		})
	}

	// 5. Netting (10K obligations)
	{
		const n = 10000
		obligations := generateObligations(n)
		nettingSvc := engine.NewNettingService()

		start := time.Now()
		nettingResults := nettingSvc.Net(obligations)
		elapsed := time.Since(start)

		results = append(results, benchResult{
			Name: fmt.Sprintf("Clearing: netting %d obligations", n), Ops: n,
			Duration: elapsed, OpsPerSec: float64(n) / elapsed.Seconds(),
			AvgLatency: elapsed / time.Duration(len(nettingResults)),
		})
	}

	// 6. Full pipeline (matching + novation + netting)
	{
		const ops = 5000
		var seq uint64
		idGen := &engine.SeqIDGen{}
		book := engine.NewOrderBook("PIPELINE-BENCH", idGen, &seq)
		novSvc := engine.NewNovationService(idGen)
		nettingSvc := engine.NewNettingService()
		price := engine.MustParseDecimal("425.50")
		allObligations := make([]engine.ClearingObligation, 0, ops*2)

		start := time.Now()
		for j := 0; j < ops; j++ {
			sell := &engine.Order{
				OrderID: fmt.Sprintf("s-%d", j), InstrumentID: "PIPELINE-BENCH",
				AccountID: fmt.Sprintf("seller-%d", j%20), ParticipantID: fmt.Sprintf("P-S-%d", j%20),
				Side: engine.SideSell, OrderType: engine.OrderTypeLimit,
				TimeInForce: engine.TIFDay, Price: price, Quantity: 10,
			}
			book.SubmitOrder(sell)
			buy := &engine.Order{
				OrderID: fmt.Sprintf("b-%d", j), InstrumentID: "PIPELINE-BENCH",
				AccountID: fmt.Sprintf("buyer-%d", j%20), ParticipantID: fmt.Sprintf("P-B-%d", j%20),
				Side: engine.SideBuy, OrderType: engine.OrderTypeLimit,
				TimeInForce: engine.TIFDay, Price: price, Quantity: 10,
			}
			matchResult := book.SubmitOrder(buy)
			for _, trade := range matchResult.Trades {
				novResult, err := novSvc.Novate(trade)
				if err != nil {
					t.Fatalf("novation failed: %v", err)
				}
				allObligations = append(allObligations, novResult.BuyerObligation, novResult.SellerObligation)
			}
		}
		nettingSvc.Net(allObligations)
		elapsed := time.Since(start)

		results = append(results, benchResult{
			Name: "Full pipeline: match+novate+net", Ops: ops,
			Duration: elapsed, OpsPerSec: float64(ops) / elapsed.Seconds(),
			AvgLatency: elapsed / time.Duration(ops),
		})
	}

	// --- Print Summary Table ---
	printSummaryTable(t, results)
}

func printSummaryTable(t *testing.T, results []benchResult) {
	t.Helper()

	sep := strings.Repeat("-", 100)
	header := fmt.Sprintf("%-45s %8s %12s %14s %12s", "Benchmark", "Ops", "Duration", "Ops/sec", "Avg Latency")

	t.Log("")
	t.Log("=== GarudaX Performance Benchmark Summary ===")
	t.Log(sep)
	t.Log(header)
	t.Log(sep)

	for _, r := range results {
		t.Logf("%-45s %8d %12v %14.0f %12v",
			r.Name, r.Ops, r.Duration.Round(time.Microsecond),
			r.OpsPerSec, r.AvgLatency.Round(time.Nanosecond))
	}
	t.Log(sep)

	// Performance targets check
	t.Log("")
	t.Log("=== Performance Target Verification ===")
	for _, r := range results {
		status := "PASS"
		notes := ""
		switch {
		case strings.Contains(r.Name, "single instrument throughput"):
			if r.OpsPerSec < 1000 {
				status = "FAIL"
				notes = fmt.Sprintf("target >1000 ops/sec, got %.0f", r.OpsPerSec)
			} else {
				notes = fmt.Sprintf("%.0fx above 1000 ops/sec target", r.OpsPerSec/1000)
			}
		case strings.Contains(r.Name, "Full pipeline"):
			if r.AvgLatency > 10*time.Millisecond {
				status = "FAIL"
				notes = fmt.Sprintf("target <10ms avg, got %v", r.AvgLatency)
			} else {
				notes = fmt.Sprintf("avg latency %v (target <10ms)", r.AvgLatency)
			}
		}
		if notes != "" {
			t.Logf("  [%s] %s — %s", status, r.Name, notes)
		}
	}
}

type benchResult struct {
	Name       string
	Ops        int
	Duration   time.Duration
	OpsPerSec  float64
	AvgLatency time.Duration
}
