package load

import (
	"fmt"
	"testing"
	"time"

	"github.com/garudax-platform/tests/load/engine"
)

// ---------------------------------------------------------------------------
// BenchmarkNovation
// Measure clearing novation throughput — converting bilateral trades into
// CCP-intermediated clearing obligations.
// ---------------------------------------------------------------------------

func BenchmarkNovation(b *testing.B) {
	idGen := &engine.SeqIDGen{}
	novSvc := engine.NewNovationService(idGen)
	price := engine.MustParseDecimal("425.50")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		trade := engine.Trade{
			TradeID:             fmt.Sprintf("trade-%d", i),
			InstrumentID:        "WHEAT-2026Q3",
			BuyOrderID:          fmt.Sprintf("buy-order-%d", i),
			SellOrderID:         fmt.Sprintf("sell-order-%d", i),
			BuyerParticipantID:  fmt.Sprintf("buyer-%d", i%50),
			SellerParticipantID: fmt.Sprintf("seller-%d", i%50),
			Price:               price,
			Quantity:            uint64(10 + i%100),
			TradeValue:          price.MulUint64(uint64(10 + i%100)),
			AggressorSide:       engine.SideBuy,
			TradeType:           engine.TradeTypeContinuous,
			SequenceNumber:      uint64(i),
			ExecutedAt:          time.Now(),
		}

		result, err := novSvc.Novate(trade)
		if err != nil {
			b.Fatalf("novation failed at iteration %d: %v", i, err)
		}
		if result.BuyerObligation.ObligationID == "" {
			b.Fatal("empty buyer obligation ID")
		}
		if result.SellerObligation.ObligationID == "" {
			b.Fatal("empty seller obligation ID")
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkNovationBatch
// Batch novation: process N trades through novation in sequence, simulating
// a clearing cycle for a trading session.
// ---------------------------------------------------------------------------

func BenchmarkNovationBatch(b *testing.B) {
	batchSizes := []int{100, 500, 2000}
	for _, size := range batchSizes {
		b.Run(fmt.Sprintf("batch_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				idGen := &engine.SeqIDGen{}
				novSvc := engine.NewNovationService(idGen)
				price := engine.MustParseDecimal("350.00")

				for j := 0; j < size; j++ {
					trade := engine.Trade{
						TradeID:             fmt.Sprintf("t-%d-%d", i, j),
						InstrumentID:        "CORN-2026Q4",
						BuyOrderID:          fmt.Sprintf("bo-%d-%d", i, j),
						SellOrderID:         fmt.Sprintf("so-%d-%d", i, j),
						BuyerParticipantID:  fmt.Sprintf("P-%d", j%20),
						SellerParticipantID: fmt.Sprintf("P-%d", (j+10)%20),
						Price:               price,
						Quantity:            uint64(5 + j%50),
						TradeValue:          price.MulUint64(uint64(5 + j%50)),
						AggressorSide:       engine.SideBuy,
						SequenceNumber:      uint64(j),
						ExecutedAt:          time.Now(),
					}
					_, err := novSvc.Novate(trade)
					if err != nil {
						b.Fatalf("novation failed: %v", err)
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkNetting
// Measure netting calculation time with N obligations.
// Simulates multilateral netting across multiple participants and instruments.
// ---------------------------------------------------------------------------

func BenchmarkNetting(b *testing.B) {
	obligationCounts := []int{100, 500, 2000, 10000}
	for _, n := range obligationCounts {
		b.Run(fmt.Sprintf("obligations_%d", n), func(b *testing.B) {
			b.ReportAllocs()

			// Pre-generate obligations
			obligations := generateObligations(n)
			nettingSvc := engine.NewNettingService()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				results := nettingSvc.Net(obligations)
				if len(results) == 0 {
					b.Fatal("netting produced zero results")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkNettingEfficiency
// Measure netting with high offsetting (many buys and sells for same
// participant/instrument) to verify near-100% netting efficiency.
// ---------------------------------------------------------------------------

func BenchmarkNettingEfficiency(b *testing.B) {
	// Create obligations where each participant has equal buy and sell quantities
	// to achieve maximum netting efficiency.
	numParticipants := 10
	tradesPerParticipant := 100
	n := numParticipants * tradesPerParticipant * 2 // buy + sell per trade

	obligations := make([]engine.ClearingObligation, 0, n)
	idGen := &engine.SeqIDGen{}
	price := engine.MustParseDecimal("500.00")

	for p := 0; p < numParticipants; p++ {
		for t := 0; t < tradesPerParticipant; t++ {
			// Buy obligation
			obligations = append(obligations, engine.ClearingObligation{
				ObligationID:  idGen.NewID(),
				TradeID:       fmt.Sprintf("trade-%d-%d", p, t),
				InstrumentID:  "SOYBEANS-2026Q2",
				ParticipantID: fmt.Sprintf("P-%d", p),
				Side:          engine.SideBuy,
				Price:         price,
				Quantity:      10,
				Value:         price.MulUint64(10),
				Status:        engine.ClearingStatusNovated,
				CreatedAt:     time.Now(),
				NovatedAt:     time.Now(),
			})
			// Sell obligation (same qty to achieve full offset)
			obligations = append(obligations, engine.ClearingObligation{
				ObligationID:  idGen.NewID(),
				TradeID:       fmt.Sprintf("trade-%d-%d-s", p, t),
				InstrumentID:  "SOYBEANS-2026Q2",
				ParticipantID: fmt.Sprintf("P-%d", p),
				Side:          engine.SideSell,
				Price:         price,
				Quantity:      10,
				Value:         price.MulUint64(10),
				Status:        engine.ClearingStatusNovated,
				CreatedAt:     time.Now(),
				NovatedAt:     time.Now(),
			})
		}
	}

	nettingSvc := engine.NewNettingService()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		results := nettingSvc.Net(obligations)
		// Verify all positions are flat (100% netting efficiency)
		for _, r := range results {
			if r.NetQuantity != 0 {
				b.Fatalf("expected flat position, got net qty %d for %s",
					r.NetQuantity, r.ParticipantID)
			}
			eff := r.NettingEfficiency()
			if eff != 100.0 {
				b.Fatalf("expected 100%% efficiency, got %.1f%% for %s",
					eff, r.ParticipantID)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkFullPipeline
// End-to-end benchmark: matching -> novation -> netting for N trades.
// Simulates a complete trading session clearing cycle.
// ---------------------------------------------------------------------------

func BenchmarkFullPipeline(b *testing.B) {
	tradeCounts := []int{100, 500, 2000}
	for _, n := range tradeCounts {
		b.Run(fmt.Sprintf("trades_%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var seq uint64
				idGen := &engine.SeqIDGen{}
				book := engine.NewOrderBook("WHEAT-2026Q3", idGen, &seq)
				novSvc := engine.NewNovationService(idGen)
				nettingSvc := engine.NewNettingService()
				price := engine.MustParseDecimal("425.50")

				allObligations := make([]engine.ClearingObligation, 0, n*2)

				// Phase 1: Generate trades through matching
				for j := 0; j < n; j++ {
					sell := &engine.Order{
						OrderID:       fmt.Sprintf("s-%d-%d", i, j),
						InstrumentID:  "WHEAT-2026Q3",
						AccountID:     fmt.Sprintf("seller-%d", j%20),
						ParticipantID: fmt.Sprintf("P-S-%d", j%20),
						Side:          engine.SideSell,
						OrderType:     engine.OrderTypeLimit,
						TimeInForce:   engine.TIFDay,
						Price:         price,
						Quantity:      10,
					}
					book.SubmitOrder(sell)

					buy := &engine.Order{
						OrderID:       fmt.Sprintf("b-%d-%d", i, j),
						InstrumentID:  "WHEAT-2026Q3",
						AccountID:     fmt.Sprintf("buyer-%d", j%20),
						ParticipantID: fmt.Sprintf("P-B-%d", j%20),
						Side:          engine.SideBuy,
						OrderType:     engine.OrderTypeLimit,
						TimeInForce:   engine.TIFDay,
						Price:         price,
						Quantity:      10,
					}
					result := book.SubmitOrder(buy)

					// Phase 2: Novate each trade
					for _, trade := range result.Trades {
						novResult, err := novSvc.Novate(trade)
						if err != nil {
							b.Fatalf("novation failed: %v", err)
						}
						allObligations = append(allObligations,
							novResult.BuyerObligation,
							novResult.SellerObligation,
						)
					}
				}

				// Phase 3: Net all obligations
				nettingResults := nettingSvc.Net(allObligations)
				if len(nettingResults) == 0 {
					b.Fatal("netting produced zero results")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: generate test obligations
// ---------------------------------------------------------------------------

func generateObligations(n int) []engine.ClearingObligation {
	idGen := &engine.SeqIDGen{}
	obligations := make([]engine.ClearingObligation, n)
	instruments := []string{"WHEAT-2026Q3", "CORN-2026Q4", "SOYBEANS-2026Q2", "RICE-2026Q3", "COTTON-2026Q4"}
	now := time.Now()

	for i := 0; i < n; i++ {
		side := engine.SideBuy
		if i%3 == 0 {
			side = engine.SideSell
		}
		price := engine.DecimalFromRaw(int64(3000000 + (i%100)*1000))
		obligations[i] = engine.ClearingObligation{
			ObligationID:  idGen.NewID(),
			TradeID:       fmt.Sprintf("trade-%d", i),
			InstrumentID:  instruments[i%len(instruments)],
			ParticipantID: fmt.Sprintf("P-%d", i%20),
			Side:          side,
			Price:         price,
			Quantity:      uint64(10 + i%50),
			Value:         price.MulUint64(uint64(10 + i%50)),
			Status:        engine.ClearingStatusNovated,
			CreatedAt:     now,
			NovatedAt:     now,
		}
	}
	return obligations
}
