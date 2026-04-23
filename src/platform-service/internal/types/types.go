// Package types defines all domain types for the platform-service.
package types

// TenantStatus constants represent the lifecycle states of a tenant.
const (
	TenantStatusActive          = "ACTIVE"
	TenantStatusSuspended       = "SUSPENDED"
	TenantStatusOnboarding      = "ONBOARDING"
	TenantStatusDecommissioned  = "DECOMMISSIONED"
)

// Tenant holds the registry entry for a GarudaX tenant (venue).
// Corresponds to the platform.tenants table in V29__platform_schemas.sql.
type Tenant struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Status             string                 `json:"status"`
	Flagship           bool                   `json:"flagship"`
	GovernanceTier     string                 `json:"governance_tier"`
	OnboardingMetadata map[string]interface{} `json:"onboarding_metadata"`
	CreatedAt          string                 `json:"created_at"`
	UpdatedAt          string                 `json:"updated_at"`
}

// ProvisionResult describes the outcome of a tenant provisioning operation.
type ProvisionResult struct {
	TenantID       string   `json:"tenant_id"`
	SchemasCreated []string `json:"schemas_created"`
	TopicPrefixes  []string `json:"topic_prefixes"`
	ConfigEntries  []string `json:"config_entries"`
	Status         string   `json:"status"`
}

// ErrorDetail carries a machine-readable code and human-readable message.
type ErrorDetail struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

// ErrorResponse is the standard error envelope returned by all endpoints.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}
