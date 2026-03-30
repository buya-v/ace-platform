package onboarding

import (
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/compliance-service/internal/types"
)

// Store defines the persistence interface for onboarding data.
type Store interface {
	SaveApplication(app *types.KYCApplication) error
	GetApplication(applicationID string) (*types.KYCApplication, error)
	ListApplications(statusFilter types.KYCStatus, typeFilter types.ParticipantType, limit int) ([]*types.KYCApplication, error)
	GetApplicationByParticipant(participantID string) (*types.KYCApplication, error)

	SaveDocument(doc *types.Document) error
	GetDocument(documentID string) (*types.Document, error)
	ListDocuments(applicationID string) ([]*types.Document, error)
}

// InMemoryStore is an in-memory store for development and testing.
type InMemoryStore struct {
	mu           sync.RWMutex
	applications map[string]*types.KYCApplication
	documents    map[string]*types.Document
	appByPart    map[string]string // participantID -> applicationID
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		applications: make(map[string]*types.KYCApplication),
		documents:    make(map[string]*types.Document),
		appByPart:    make(map[string]string),
	}
}

func (s *InMemoryStore) SaveApplication(app *types.KYCApplication) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	app.UpdatedAt = time.Now().UTC()
	s.applications[app.ApplicationID] = app
	s.appByPart[app.ParticipantID] = app.ApplicationID
	return nil
}

func (s *InMemoryStore) GetApplication(applicationID string) (*types.KYCApplication, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	app, ok := s.applications[applicationID]
	if !ok {
		return nil, fmt.Errorf("application %s not found", applicationID)
	}
	return app, nil
}

func (s *InMemoryStore) GetApplicationByParticipant(participantID string) (*types.KYCApplication, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	appID, ok := s.appByPart[participantID]
	if !ok {
		return nil, fmt.Errorf("no application found for participant %s", participantID)
	}
	app, ok := s.applications[appID]
	if !ok {
		return nil, fmt.Errorf("application %s not found", appID)
	}
	return app, nil
}

func (s *InMemoryStore) ListApplications(statusFilter types.KYCStatus, typeFilter types.ParticipantType, limit int) ([]*types.KYCApplication, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var result []*types.KYCApplication
	for _, app := range s.applications {
		if statusFilter != "" && app.Status != statusFilter {
			continue
		}
		if typeFilter != "" && app.ParticipantType != typeFilter {
			continue
		}
		result = append(result, app)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (s *InMemoryStore) SaveDocument(doc *types.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[doc.DocumentID] = doc
	return nil
}

func (s *InMemoryStore) GetDocument(documentID string) (*types.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.documents[documentID]
	if !ok {
		return nil, fmt.Errorf("document %s not found", documentID)
	}
	return doc, nil
}

func (s *InMemoryStore) ListDocuments(applicationID string) ([]*types.Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*types.Document
	for _, doc := range s.documents {
		if doc.ApplicationID == applicationID {
			result = append(result, doc)
		}
	}
	return result, nil
}
