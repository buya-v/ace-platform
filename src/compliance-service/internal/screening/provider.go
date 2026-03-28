package screening

import (
	"github.com/ace-platform/compliance-service/internal/types"
)

// Provider is the pluggable watchlist screening provider interface.
// Production implementations would call external APIs (Dow Jones, Refinitiv, etc.).
type Provider interface {
	Screen(name string, nationality string) ([]ProviderMatch, error)
	Name() string
}

// ProviderMatch represents a raw match from a screening provider.
type ProviderMatch struct {
	MatchedName     string
	MatchedEntityID string
	ListSource      string
	MatchType       types.MatchType
	Score           float64
}

// DefaultProvider is a no-op provider for development/testing that always returns clear.
type DefaultProvider struct{}

func NewDefaultProvider() *DefaultProvider {
	return &DefaultProvider{}
}

func (p *DefaultProvider) Screen(name string, nationality string) ([]ProviderMatch, error) {
	return nil, nil
}

func (p *DefaultProvider) Name() string {
	return "default-dev"
}

// StaticProvider returns configured matches (for testing).
type StaticProvider struct {
	Matches []ProviderMatch
	Err     error
}

func NewStaticProvider(matches []ProviderMatch, err error) *StaticProvider {
	return &StaticProvider{Matches: matches, Err: err}
}

func (p *StaticProvider) Screen(name string, nationality string) ([]ProviderMatch, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	return p.Matches, nil
}

func (p *StaticProvider) Name() string {
	return "static-test"
}
