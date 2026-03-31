package defaultmgmt

import (
	"testing"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

func TestNewDefaultFundManager(t *testing.T) {
	fm := NewDefaultFundManager()
	if fm == nil {
		t.Fatal("expected non-nil fund manager")
	}
	if !fm.GetTotalFund().IsZero() {
		t.Error("expected zero total fund for new manager")
	}
	if fm.GetParticipantCount() != 0 {
		t.Error("expected zero participants for new manager")
	}
}

func TestAddContribution(t *testing.T) {
	fm := NewDefaultFundManager()

	err := fm.AddContribution("P001", types.DecimalFromInt(1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := fm.GetContribution("P001")
	if !got.Equal(types.DecimalFromInt(1000)) {
		t.Errorf("expected 1000, got %s", got.String())
	}
}

func TestAddContribution_Negative(t *testing.T) {
	fm := NewDefaultFundManager()
	err := fm.AddContribution("P001", types.DecimalFromInt(-100))
	if err == nil {
		t.Fatal("expected error for negative contribution")
	}
}

func TestAddContribution_Replace(t *testing.T) {
	fm := NewDefaultFundManager()
	fm.AddContribution("P001", types.DecimalFromInt(1000))
	fm.AddContribution("P001", types.DecimalFromInt(2000))

	got := fm.GetContribution("P001")
	if !got.Equal(types.DecimalFromInt(2000)) {
		t.Errorf("expected replaced value 2000, got %s", got.String())
	}
	if fm.GetParticipantCount() != 1 {
		t.Errorf("expected 1 participant, got %d", fm.GetParticipantCount())
	}
}

func TestGetContribution_Missing(t *testing.T) {
	fm := NewDefaultFundManager()
	got := fm.GetContribution("NONEXISTENT")
	if !got.IsZero() {
		t.Errorf("expected zero for missing participant, got %s", got.String())
	}
}

func TestGetTotalFund(t *testing.T) {
	fm := NewDefaultFundManager()
	fm.AddContribution("P001", types.DecimalFromInt(1000))
	fm.AddContribution("P002", types.DecimalFromInt(2000))
	fm.AddContribution("P003", types.DecimalFromInt(3000))

	total := fm.GetTotalFund()
	if !total.Equal(types.DecimalFromInt(6000)) {
		t.Errorf("expected total 6000, got %s", total.String())
	}
}

func TestCCPSkinInTheGame(t *testing.T) {
	fm := NewDefaultFundManager()

	if !fm.GetCCPSkinInTheGame().IsZero() {
		t.Error("expected zero initial CCP skin-in-the-game")
	}

	err := fm.SetCCPSkinInTheGame(types.DecimalFromInt(500))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fm.GetCCPSkinInTheGame().Equal(types.DecimalFromInt(500)) {
		t.Errorf("expected 500, got %s", fm.GetCCPSkinInTheGame().String())
	}

	err = fm.SetCCPSkinInTheGame(types.DecimalFromInt(-1))
	if err == nil {
		t.Fatal("expected error for negative CCP capital")
	}
}

func TestCCPAdditionalCapital(t *testing.T) {
	fm := NewDefaultFundManager()

	if !fm.GetCCPAdditionalCapital().IsZero() {
		t.Error("expected zero initial CCP additional capital")
	}

	err := fm.SetCCPAdditionalCapital(types.DecimalFromInt(1000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fm.GetCCPAdditionalCapital().Equal(types.DecimalFromInt(1000)) {
		t.Errorf("expected 1000, got %s", fm.GetCCPAdditionalCapital().String())
	}

	err = fm.SetCCPAdditionalCapital(types.DecimalFromInt(-1))
	if err == nil {
		t.Fatal("expected error for negative CCP additional capital")
	}
}

func TestGetNonDefaultingContributions(t *testing.T) {
	fm := NewDefaultFundManager()
	fm.AddContribution("P001", types.DecimalFromInt(1000))
	fm.AddContribution("P002", types.DecimalFromInt(2000))
	fm.AddContribution("P003", types.DecimalFromInt(3000))

	result := fm.GetNonDefaultingContributions("P002")
	if len(result) != 2 {
		t.Fatalf("expected 2 non-defaulting participants, got %d", len(result))
	}
	if _, ok := result["P002"]; ok {
		t.Error("defaulting participant should not be in result")
	}
	if !result["P001"].Equal(types.DecimalFromInt(1000)) {
		t.Errorf("expected P001=1000, got %s", result["P001"].String())
	}
	if !result["P003"].Equal(types.DecimalFromInt(3000)) {
		t.Errorf("expected P003=3000, got %s", result["P003"].String())
	}
}
