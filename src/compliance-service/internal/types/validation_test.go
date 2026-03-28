package types

import "testing"

func TestValidParticipantType(t *testing.T) {
	tests := []struct {
		pt   ParticipantType
		want bool
	}{
		{ParticipantIndividual, true},
		{ParticipantCorporate, true},
		{ParticipantCooperative, true},
		{ParticipantBroker, true},
		{ParticipantInstitutional, true},
		{ParticipantType("UNKNOWN"), false},
		{ParticipantType(""), false},
	}

	for _, tt := range tests {
		if got := ValidParticipantType(tt.pt); got != tt.want {
			t.Errorf("ValidParticipantType(%q) = %v, want %v", tt.pt, got, tt.want)
		}
	}
}

func TestRequiredDocuments(t *testing.T) {
	tests := []struct {
		pt    ParticipantType
		count int
	}{
		{ParticipantIndividual, 2},
		{ParticipantCorporate, 5},
		{ParticipantCooperative, 4},
		{ParticipantBroker, 6},
		{ParticipantInstitutional, 5},
		{ParticipantType("UNKNOWN"), 0},
	}

	for _, tt := range tests {
		docs := RequiredDocuments(tt.pt)
		if len(docs) != tt.count {
			t.Errorf("RequiredDocuments(%q) returned %d docs, want %d", tt.pt, len(docs), tt.count)
		}
	}

	// Verify individual requires ID and proof of address
	docs := RequiredDocuments(ParticipantIndividual)
	found := map[DocumentType]bool{}
	for _, d := range docs {
		found[d] = true
	}
	if !found[DocNationalID] {
		t.Error("Individual should require NATIONAL_ID")
	}
	if !found[DocProofOfAddress] {
		t.Error("Individual should require PROOF_OF_ADDRESS")
	}
}

func TestValidateStatusTransition(t *testing.T) {
	validTransitions := []struct {
		from, to KYCStatus
	}{
		{StatusApplicationSubmitted, StatusDocumentsPending},
		{StatusDocumentsPending, StatusDocumentsUploaded},
		{StatusDocumentsUploaded, StatusVerificationInProgress},
		{StatusVerificationInProgress, StatusScreeningInProgress},
		{StatusVerificationInProgress, StatusDocumentsPending},
		{StatusScreeningInProgress, StatusRiskScoring},
		{StatusScreeningInProgress, StatusManualReview},
		{StatusRiskScoring, StatusApproved},
		{StatusRiskScoring, StatusManualReview},
		{StatusManualReview, StatusApproved},
		{StatusManualReview, StatusRejected},
		{StatusApproved, StatusSuspended},
		{StatusApproved, StatusExpired},
		{StatusSuspended, StatusApproved},
	}

	for _, tt := range validTransitions {
		if err := ValidateStatusTransition(tt.from, tt.to); err != nil {
			t.Errorf("transition %s -> %s should be valid, got error: %v", tt.from, tt.to, err)
		}
	}

	invalidTransitions := []struct {
		from, to KYCStatus
	}{
		{StatusApplicationSubmitted, StatusApproved},
		{StatusRejected, StatusApproved},
		{StatusApproved, StatusRejected},
		{StatusDocumentsPending, StatusApproved},
		{StatusExpired, StatusApproved},
	}

	for _, tt := range invalidTransitions {
		if err := ValidateStatusTransition(tt.from, tt.to); err == nil {
			t.Errorf("transition %s -> %s should be invalid, but got no error", tt.from, tt.to)
		}
	}
}
