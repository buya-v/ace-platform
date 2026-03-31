package collateral

import (
	"testing"

	"github.com/garudax-platform/margin-engine/internal/types"
)

// staticSource returns a fixed collateral value for any participant.
type staticSource struct {
	value types.Decimal
}

func (s *staticSource) GetCollateral(participantID string) types.Decimal {
	return s.value
}

// panicSource always panics.
type panicSource struct{}

func (s *panicSource) GetCollateral(participantID string) types.Decimal {
	panic("boom")
}

// participantAwareSource returns different values per participant.
type participantAwareSource struct {
	values map[string]types.Decimal
}

func (s *participantAwareSource) GetCollateral(participantID string) types.Decimal {
	if v, ok := s.values[participantID]; ok {
		return v
	}
	return types.DecimalZero()
}

func TestCompositeCollateralSource_Empty(t *testing.T) {
	cs := NewCompositeCollateralSource()
	result := cs.GetCollateral("P1")
	if !result.IsZero() {
		t.Errorf("expected zero for no sources, got %s", result.String())
	}
}

func TestCompositeCollateralSource_SingleSource(t *testing.T) {
	cs := NewCompositeCollateralSource()
	cs.Add("clearing", &staticSource{value: types.DecimalFromInt(5000)})

	result := cs.GetCollateral("P1")
	expected := types.DecimalFromInt(5000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCompositeCollateralSource_MultipleSources(t *testing.T) {
	cs := NewCompositeCollateralSource()
	cs.Add("clearing", &staticSource{value: types.DecimalFromInt(5000)})
	cs.Add("warehouse", &staticSource{value: types.DecimalFromInt(3000)})

	result := cs.GetCollateral("P1")
	// 5000 + 3000 = 8000
	expected := types.DecimalFromInt(8000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCompositeCollateralSource_ThreeSources(t *testing.T) {
	cs := NewCompositeCollateralSource()
	cs.Add("clearing", &staticSource{value: types.DecimalFromInt(10000)})
	cs.Add("warehouse", &staticSource{value: types.DecimalFromInt(5000)})
	cs.Add("cash", &staticSource{value: types.DecimalFromInt(2000)})

	result := cs.GetCollateral("P1")
	expected := types.DecimalFromInt(17000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCompositeCollateralSource_PanicRecovery(t *testing.T) {
	cs := NewCompositeCollateralSource()
	cs.Add("clearing", &staticSource{value: types.DecimalFromInt(5000)})
	cs.Add("broken", &panicSource{})
	cs.Add("warehouse", &staticSource{value: types.DecimalFromInt(3000)})

	// Should not panic; broken source contributes zero
	result := cs.GetCollateral("P1")
	expected := types.DecimalFromInt(8000)
	if !result.Equal(expected) {
		t.Errorf("expected %s (broken source = 0), got %s", expected.String(), result.String())
	}
}

func TestCompositeCollateralSource_AllSourcesPanic(t *testing.T) {
	cs := NewCompositeCollateralSource()
	cs.Add("broken1", &panicSource{})
	cs.Add("broken2", &panicSource{})

	result := cs.GetCollateral("P1")
	if !result.IsZero() {
		t.Errorf("expected zero when all sources panic, got %s", result.String())
	}
}

func TestCompositeCollateralSource_ZeroContribution(t *testing.T) {
	cs := NewCompositeCollateralSource()
	cs.Add("clearing", &staticSource{value: types.DecimalFromInt(5000)})
	cs.Add("empty", &staticSource{value: types.DecimalZero()})

	result := cs.GetCollateral("P1")
	expected := types.DecimalFromInt(5000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCompositeCollateralSource_PerParticipant(t *testing.T) {
	cs := NewCompositeCollateralSource()
	cs.Add("positions", &participantAwareSource{
		values: map[string]types.Decimal{
			"P1": types.DecimalFromInt(1000),
			"P2": types.DecimalFromInt(2000),
		},
	})
	cs.Add("warehouse", &participantAwareSource{
		values: map[string]types.Decimal{
			"P1": types.DecimalFromInt(500),
			"P3": types.DecimalFromInt(800),
		},
	})

	// P1: 1000 + 500 = 1500
	r1 := cs.GetCollateral("P1")
	if !r1.Equal(types.DecimalFromInt(1500)) {
		t.Errorf("P1: expected 1500, got %s", r1.String())
	}

	// P2: 2000 + 0 = 2000
	r2 := cs.GetCollateral("P2")
	if !r2.Equal(types.DecimalFromInt(2000)) {
		t.Errorf("P2: expected 2000, got %s", r2.String())
	}

	// P3: 0 + 800 = 800
	r3 := cs.GetCollateral("P3")
	if !r3.Equal(types.DecimalFromInt(800)) {
		t.Errorf("P3: expected 800, got %s", r3.String())
	}

	// P4: 0 + 0 = 0
	r4 := cs.GetCollateral("P4")
	if !r4.IsZero() {
		t.Errorf("P4: expected 0, got %s", r4.String())
	}
}

func TestCompositeCollateralSource_SourceCount(t *testing.T) {
	cs := NewCompositeCollateralSource()
	if cs.SourceCount() != 0 {
		t.Errorf("expected 0, got %d", cs.SourceCount())
	}

	cs.Add("a", &staticSource{value: types.DecimalZero()})
	if cs.SourceCount() != 1 {
		t.Errorf("expected 1, got %d", cs.SourceCount())
	}

	cs.Add("b", &staticSource{value: types.DecimalZero()})
	if cs.SourceCount() != 2 {
		t.Errorf("expected 2, got %d", cs.SourceCount())
	}
}
