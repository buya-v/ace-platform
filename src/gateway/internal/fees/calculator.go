package fees

import (
	"strings"

	"github.com/garudax-platform/decimal"
)

// FeeResult represents the result of a fee calculation.
//
// All money fields are the platform's shared fixed-point decimal type
// (Decimal(18,4)) rather than float64: fees are money and must be computed
// without float drift, silent overflow, or truncation bias (see R006 audit).
type FeeResult struct {
	FeeType        string          `json:"fee_type"`
	RateBPS        decimal.Decimal `json:"rate_bps"`
	RateAmount     decimal.Decimal `json:"rate_amount"`
	PerContractAmt decimal.Decimal `json:"per_contract_amount"`
	TotalFee       decimal.Decimal `json:"total_fee"`
	MinApplied     bool            `json:"min_applied,omitempty"`
	MaxApplied     bool            `json:"max_applied,omitempty"`
}

// CalculateFee computes the fee for a trade given the active rules.
// tradeValue is the notional value (price * quantity * contract_size).
// contracts is the number of contracts traded.
// participantTier is the participant's fee tier (farmer, hedger, speculator, market_maker).
// instrumentID is used for instrument pattern matching.
// feeType is the type of fee to calculate (trading, clearing, data, membership).
// rules should be the active rules from the current schedule.
func CalculateFee(tradeValue decimal.Decimal, contracts int, participantTier, instrumentID, feeType string, rules []FeeRule) FeeResult {
	result := FeeResult{
		FeeType: feeType,
	}

	rule := findMatchingRule(rules, feeType, participantTier, instrumentID)
	if rule == nil {
		return result
	}

	result.RateBPS = rule.RateBPS

	// Rate-based fee: tradeValue * rateBPS / 10000. The multiply is performed
	// before the divide so a fractional basis-point rate (e.g. 2.5 bps) is not
	// lost to the fixed-point scale; the division then rounds half-to-even.
	rateFee := tradeValue.MulDecimal(rule.RateBPS).DivInt64(10000)
	result.RateAmount = rateFee

	// Per-contract fee: perContractFee * contracts.
	perContractTotal := rule.PerContractFee.MulInt64(int64(contracts))
	result.PerContractAmt = perContractTotal

	// Total fee before min/max.
	totalFee := rateFee.Add(perContractTotal)

	// Apply minimum fee.
	if rule.MinFee.IsPos() && totalFee.LessThan(rule.MinFee) {
		totalFee = rule.MinFee
		result.MinApplied = true
	}

	// Apply maximum fee cap.
	if rule.MaxFee != nil && totalFee.GreaterThan(*rule.MaxFee) {
		totalFee = *rule.MaxFee
		result.MaxApplied = true
	}

	result.TotalFee = totalFee
	return result
}

// CalculateAllFees calculates all applicable fees for a trade.
// Returns one FeeResult per matching fee type.
func CalculateAllFees(tradeValue decimal.Decimal, contracts int, participantTier, instrumentID string, rules []FeeRule) []FeeResult {
	feeTypes := collectFeeTypes(rules)
	var results []FeeResult
	for _, ft := range feeTypes {
		r := CalculateFee(tradeValue, contracts, participantTier, instrumentID, ft, rules)
		if r.TotalFee.IsPos() || r.RateBPS.IsPos() {
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
