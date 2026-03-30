package store

import (
	"testing"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

func makeObl(id, tradeID, participant, instrument string, side types.Side) types.ClearingObligation {
	return types.ClearingObligation{
		ObligationID:  id,
		TradeID:       tradeID,
		InstrumentID:  instrument,
		ParticipantID: participant,
		Side:          side,
		Price:         types.DecimalFromInt(500),
		Quantity:      10,
		Value:         types.DecimalFromInt(5000),
		Status:        types.ClearingStatusNovated,
		CreatedAt:     time.Now(),
		NovatedAt:     time.Now(),
	}
}

func TestAppendAndLen(t *testing.T) {
	s := NewInMemoryObligationStore()
	s.Append(makeObl("o1", "t1", "P1", "WHT", types.SideBuy))
	s.Append(makeObl("o2", "t1", "P2", "WHT", types.SideSell))

	if s.Len() != 2 {
		t.Errorf("len = %d, want 2", s.Len())
	}
}

func TestByTrade(t *testing.T) {
	s := NewInMemoryObligationStore()
	s.Append(makeObl("o1", "t1", "P1", "WHT", types.SideBuy))
	s.Append(makeObl("o2", "t1", "P2", "WHT", types.SideSell))
	s.Append(makeObl("o3", "t2", "P1", "WHT", types.SideBuy))

	obls := s.ByTrade("t1")
	if len(obls) != 2 {
		t.Errorf("got %d, want 2", len(obls))
	}
}

func TestByParticipant(t *testing.T) {
	s := NewInMemoryObligationStore()
	s.Append(makeObl("o1", "t1", "P1", "WHT", types.SideBuy))
	s.Append(makeObl("o2", "t1", "P2", "WHT", types.SideSell))
	s.Append(makeObl("o3", "t2", "P1", "CORN", types.SideBuy))

	obls := s.ByParticipant("P1")
	if len(obls) != 2 {
		t.Errorf("got %d, want 2", len(obls))
	}
}

func TestByInstrument(t *testing.T) {
	s := NewInMemoryObligationStore()
	s.Append(makeObl("o1", "t1", "P1", "WHT", types.SideBuy))
	s.Append(makeObl("o2", "t1", "P2", "WHT", types.SideSell))
	s.Append(makeObl("o3", "t2", "P1", "CORN", types.SideBuy))

	obls := s.ByInstrument("WHT")
	if len(obls) != 2 {
		t.Errorf("got %d, want 2", len(obls))
	}
}

func TestByStatus(t *testing.T) {
	s := NewInMemoryObligationStore()
	s.Append(makeObl("o1", "t1", "P1", "WHT", types.SideBuy))

	settled := makeObl("o2", "t2", "P2", "WHT", types.SideSell)
	settled.Status = types.ClearingStatusSettled
	s.Append(settled)

	novated := s.ByStatus(types.ClearingStatusNovated)
	if len(novated) != 1 {
		t.Errorf("novated = %d, want 1", len(novated))
	}
}

func TestAll(t *testing.T) {
	s := NewInMemoryObligationStore()
	s.Append(makeObl("o1", "t1", "P1", "WHT", types.SideBuy))
	s.Append(makeObl("o2", "t1", "P2", "WHT", types.SideSell))

	all := s.All()
	if len(all) != 2 {
		t.Errorf("all = %d, want 2", len(all))
	}
}
