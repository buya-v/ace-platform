package screening

import (
	"testing"

	"github.com/garudax-platform/compliance-service/internal/types"
)

func TestComputeRiskScore(t *testing.T) {
	tests := []struct {
		name    string
		factors types.RiskFactorBreakdown
		wantMin uint32
		wantMax uint32
		tier    types.RiskTier
	}{
		{
			name: "low risk individual in low-risk country with clear screening",
			factors: types.RiskFactorBreakdown{
				ParticipantTypeScore:    20,
				CountryRiskScore:        15,
				ScreeningResultScore:    0,
				TransactionProfileScore: 10,
				SourceOfFundsScore:      20,
				DocumentQualityScore:    10,
			},
			wantMin: 0,
			wantMax: 25,
			tier:    types.RiskLow,
		},
		{
			name: "high risk institutional in high-risk country with matches",
			factors: types.RiskFactorBreakdown{
				ParticipantTypeScore:    60,
				CountryRiskScore:        90,
				ScreeningResultScore:    80,
				TransactionProfileScore: 50,
				SourceOfFundsScore:      60,
				DocumentQualityScore:    70,
			},
			wantMin: 51,
			wantMax: 100,
			tier:    types.RiskHigh,
		},
		{
			name: "medium risk broker",
			factors: types.RiskFactorBreakdown{
				ParticipantTypeScore:    50,
				CountryRiskScore:        60,
				ScreeningResultScore:    30,
				TransactionProfileScore: 30,
				SourceOfFundsScore:      40,
				DocumentQualityScore:    20,
			},
			wantMin: 26,
			wantMax: 50,
			tier:    types.RiskMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, tier := ComputeRiskScore(tt.factors)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("score %d not in range [%d, %d]", score, tt.wantMin, tt.wantMax)
			}
			if tier != tt.tier {
				t.Errorf("expected tier %s, got %s (score=%d)", tt.tier, tier, score)
			}
		})
	}
}

func TestScoreToTier(t *testing.T) {
	tests := []struct {
		score uint32
		tier  types.RiskTier
	}{
		{0, types.RiskLow},
		{25, types.RiskLow},
		{26, types.RiskMedium},
		{50, types.RiskMedium},
		{51, types.RiskHigh},
		{75, types.RiskHigh},
		{76, types.RiskProhibited},
		{100, types.RiskProhibited},
	}

	for _, tt := range tests {
		if got := ScoreToTier(tt.score); got != tt.tier {
			t.Errorf("ScoreToTier(%d) = %s, want %s", tt.score, got, tt.tier)
		}
	}
}

func TestParticipantTypeRisk(t *testing.T) {
	// Individual should be lowest risk
	indiv := ParticipantTypeRisk(types.ParticipantIndividual)
	inst := ParticipantTypeRisk(types.ParticipantInstitutional)
	if indiv >= inst {
		t.Errorf("individual risk (%d) should be less than institutional (%d)", indiv, inst)
	}

	// Unknown type should get a default
	unknown := ParticipantTypeRisk(types.ParticipantType("UNKNOWN"))
	if unknown == 0 {
		t.Error("unknown type should have non-zero risk")
	}
}

func TestCountryRisk(t *testing.T) {
	tests := []struct {
		country string
		risk    uint32
	}{
		{"KP", 90},  // North Korea - high risk
		{"IR", 90},  // Iran - high risk
		{"MM", 60},  // Myanmar - medium risk
		{"MN", 15},  // Mongolia - low risk
		{"US", 15},  // US - low risk
	}

	for _, tt := range tests {
		if got := CountryRisk(tt.country); got != tt.risk {
			t.Errorf("CountryRisk(%s) = %d, want %d", tt.country, got, tt.risk)
		}
	}
}

func TestScreeningResultRisk(t *testing.T) {
	// Clear should be 0
	if got := ScreeningResultRisk(types.ScreeningClear, 0); got != 0 {
		t.Errorf("clear screening risk = %d, want 0", got)
	}

	// Match found should be high
	risk := ScreeningResultRisk(types.ScreeningMatchFound, 2)
	if risk < 60 {
		t.Errorf("match found risk = %d, should be >= 60", risk)
	}

	// Many matches should cap at 100
	risk = ScreeningResultRisk(types.ScreeningMatchFound, 10)
	if risk > 100 {
		t.Errorf("risk should cap at 100, got %d", risk)
	}

	// Error should be moderate
	risk = ScreeningResultRisk(types.ScreeningError, 0)
	if risk != 50 {
		t.Errorf("error risk = %d, want 50", risk)
	}
}
