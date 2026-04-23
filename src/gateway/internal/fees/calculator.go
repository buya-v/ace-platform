package fees

import (
	"math"
	"strings"
)

// FeeResult represents the result of a fee calculation.
type FeeResult struct {
	FeeType        string  `json:"fee_type"`
	RateBPS        float64 `json:"rate_bps"`
	RateAmount     float64 `json:"rate_amount"`
	PerContractAmt float64 `json:"per_contract_amount"`
	TotalFee       float64 `json:"total_fee"`
	MinApplied     bool    `json:"min_applied,omitempty"`
	MaxApplied     bool    `json:"max_applied,omitempty"`
}

// CalculateFee computes the fee for a trade given the active rules.
// tradeValue is the notional value (price * quantity * contract_size).
// contracts is the number of contracts traded.
// participantTier is the participant's fee tier (farmer, hedger, speculator, market_maker).
// instrumentID is used for instrument pattern matching.
// feeType is the type of fee to calculate (trading, clearing, data, membership).
// rules should be the active rules from the current schedule.
func CalculateFee(tradeValue float64, contracts int, participantTier, instrumentID, feeType string, rules []FeeRule) FeeResult {
	result := FeeResult{
		FeeType: feeType,
	}

	rule := findMatchingRule(rules, feeType, participantTier, instrumentID)
	if rule == nil {
		return result
	}

	result.RateBPS = rule.RateBPS

	// Calculate rate-based fee: tradeValue * (rateBPS / 10000)
	rateFee := tradeValue * (rule.RateBPS / 10000.0)
	result.RateAmount = roundTo4(rateFee)

	// Calculate per-contract fee
	perContractTotal := rule.PerContractFee * float64(contracts)
	result.PerContractAmt = roundTo4(perContractTotal)

	// Total fee before min/max
	totalFee := rateFee + perContractTotal

	// Apply minimum fee
	if totalFee < rule.MinFee && rule.MinFee > 0 {
		totalFee = rule.MinFee
		result.MinApplied = true
	}

	// Apply maximum fee cap
	if rule.MaxFee != nil && totalFee > *rule.MaxFee {
		totalFee = *rule.MaxFee
		result.MaxApplied = true
	}

	result.TotalFee = roundTo4(totalFee)
	return result
}

// CalculateAllFees calculates all applicable fees for a trade.
// Returns one FeeResult per matching fee type.
func CalculateAllFees(tradeValue float64, contracts int, participantTier, instrumentID string, rules []FeeRule) []FeeResult {
	feeTypes := collectFeeTypes(rules)
	var results []FeeResult
	for _, ft := range feeTypes {
		r := CalculateFee(tradeValue, contracts, participantTier, instrumentID, ft, rules)
		if r.TotalFee > 0 || r.RateBPS > 0 {
			results = append(results, r)
		}
	}
	return results
}

// findMatchingRule finds the best matching rule for the given parameters.
// Matching priority:
//  1. Exact tier + exact instrument match
//  2. Exact tier + wildcard instrument
//  3. Wildcard tier + exact instrument match
//  4. Wildcard tier + wildcard instrument
func findMatchingRule(rules []FeeRule, feeType, participantTier, instrumentID string) *FeeRule {
	var exactTierExactInst *FeeRule
	var exactTierWildInst *FeeRule
	var wildTierExactInst *FeeRule
	var wildTierWildInst *FeeRule

	for i := range rules {
		r := &rules[i]
		if r.FeeType != feeType {
			continue
		}

		tierMatch := r.ParticipantTier == participantTier
		tierWild := r.ParticipantTier == "*"
		instMatch := matchInstrumentPattern(r.InstrumentPattern, instrumentID)
		instWild := r.InstrumentPattern == "*"

		if tierMatch && instMatch && !instWild {
			exactTierExactInst = r
		} else if tierMatch && instWild {
			exactTierWildInst = r
		} else if tierWild && instMatch && !instWild {
			wildTierExactInst = r
		} else if tierWild && instWild {
			wildTierWildInst = r
		}
	}

	if exactTierExactInst != nil {
		return exactTierExactInst
	}
	if exactTierWildInst != nil {
		return exactTierWildInst
	}
	if wildTierExactInst != nil {
		return wildTierExactInst
	}
	return wildTierWildInst
}

// matchInstrumentPattern checks if an instrument ID matches a pattern.
// Patterns support prefix wildcard: "WHT-*" matches "WHT-HRW-2026M07-UB".
// "*" matches everything.
func matchInstrumentPattern(pattern, instrumentID string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(instrumentID, prefix)
	}
	return pattern == instrumentID
}

// collectFeeTypes returns the distinct fee types present in the rules.
func collectFeeTypes(rules []FeeRule) []string {
	seen := make(map[string]bool)
	var types []string
	for _, r := range rules {
		if !seen[r.FeeType] {
			seen[r.FeeType] = true
			types = append(types, r.FeeType)
		}
	}
	return types
}

// roundTo4 rounds a float to 4 decimal places.
func roundTo4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
