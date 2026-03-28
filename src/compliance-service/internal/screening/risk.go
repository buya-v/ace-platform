package screening

import (
	"github.com/ace-platform/compliance-service/internal/types"
)

const riskModelVersion = "v1.0"

// Risk factor weights (must sum to 100).
const (
	weightParticipantType    = 15
	weightCountryRisk        = 20
	weightScreeningResult    = 25
	weightTransactionProfile = 15
	weightSourceOfFunds      = 15
	weightDocumentQuality    = 10
)

// ComputeRiskScore calculates a risk score based on the six weighted factors.
func ComputeRiskScore(factors types.RiskFactorBreakdown) (uint32, types.RiskTier) {
	weighted := uint32(0)
	weighted += factors.ParticipantTypeScore * weightParticipantType / 100
	weighted += factors.CountryRiskScore * weightCountryRisk / 100
	weighted += factors.ScreeningResultScore * weightScreeningResult / 100
	weighted += factors.TransactionProfileScore * weightTransactionProfile / 100
	weighted += factors.SourceOfFundsScore * weightSourceOfFunds / 100
	weighted += factors.DocumentQualityScore * weightDocumentQuality / 100

	tier := ScoreToTier(weighted)
	return weighted, tier
}

// ScoreToTier maps a numeric score (0-100) to a risk tier.
func ScoreToTier(score uint32) types.RiskTier {
	switch {
	case score <= 25:
		return types.RiskLow
	case score <= 50:
		return types.RiskMedium
	case score <= 75:
		return types.RiskHigh
	default:
		return types.RiskProhibited
	}
}

// ParticipantTypeRisk returns a base risk score for a participant type.
func ParticipantTypeRisk(pt types.ParticipantType) uint32 {
	switch pt {
	case types.ParticipantIndividual:
		return 20
	case types.ParticipantCooperative:
		return 25
	case types.ParticipantCorporate:
		return 40
	case types.ParticipantBroker:
		return 50
	case types.ParticipantInstitutional:
		return 60
	default:
		return 50
	}
}

// CountryRisk returns a risk score for a country.
// In production this would use a configurable country risk database.
func CountryRisk(countryCode string) uint32 {
	highRisk := map[string]bool{
		"KP": true, "IR": true, "SY": true, "AF": true, "YE": true,
	}
	mediumRisk := map[string]bool{
		"MM": true, "LY": true, "SO": true, "SD": true, "VE": true,
	}
	if highRisk[countryCode] {
		return 90
	}
	if mediumRisk[countryCode] {
		return 60
	}
	return 15
}

// ScreeningResultRisk converts screening outcome to a risk score.
func ScreeningResultRisk(outcome types.ScreeningOutcome, matchCount int) uint32 {
	switch outcome {
	case types.ScreeningClear:
		return 0
	case types.ScreeningMatchFound:
		base := uint32(60)
		extra := uint32(matchCount * 10)
		if base+extra > 100 {
			return 100
		}
		return base + extra
	case types.ScreeningError:
		return 50
	default:
		return 50
	}
}
