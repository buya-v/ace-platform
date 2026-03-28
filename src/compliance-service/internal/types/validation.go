package types

import "fmt"

// RequiredDocuments returns the document types required for a participant type.
func RequiredDocuments(pt ParticipantType) []DocumentType {
	switch pt {
	case ParticipantIndividual:
		return []DocumentType{DocNationalID, DocProofOfAddress}
	case ParticipantCorporate:
		return []DocumentType{
			DocCompanyRegistration, DocBeneficialOwnership,
			DocFinancialStatements, DocBoardResolution, DocTaxRegistration,
		}
	case ParticipantCooperative:
		return []DocumentType{
			DocCompanyRegistration, DocBoardResolution,
			DocTaxRegistration, DocCooperativeMembership,
		}
	case ParticipantBroker:
		return []DocumentType{
			DocCompanyRegistration, DocBeneficialOwnership,
			DocFinancialStatements, DocBrokerLicense,
			DocBoardResolution, DocTaxRegistration,
		}
	case ParticipantInstitutional:
		return []DocumentType{
			DocCompanyRegistration, DocBeneficialOwnership,
			DocFinancialStatements, DocBoardResolution, DocTaxRegistration,
		}
	default:
		return nil
	}
}

// ValidateStatusTransition checks if the KYC status transition is allowed.
func ValidateStatusTransition(from, to KYCStatus) error {
	allowed := map[KYCStatus][]KYCStatus{
		StatusApplicationSubmitted:   {StatusDocumentsPending},
		StatusDocumentsPending:       {StatusDocumentsUploaded},
		StatusDocumentsUploaded:      {StatusVerificationInProgress},
		StatusVerificationInProgress: {StatusScreeningInProgress, StatusDocumentsPending},
		StatusScreeningInProgress:    {StatusRiskScoring, StatusManualReview},
		StatusRiskScoring:            {StatusApproved, StatusManualReview},
		StatusManualReview:           {StatusApproved, StatusRejected},
		StatusApproved:               {StatusSuspended, StatusExpired},
		StatusSuspended:              {StatusApproved},
	}

	targets, ok := allowed[from]
	if !ok {
		return fmt.Errorf("no transitions allowed from status %s", from)
	}
	for _, t := range targets {
		if t == to {
			return nil
		}
	}
	return fmt.Errorf("transition from %s to %s is not allowed", from, to)
}
