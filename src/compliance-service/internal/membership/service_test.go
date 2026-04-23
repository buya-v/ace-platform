package membership

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// counter-based ID generator for deterministic testing.
func testIDGen() IDGenerator {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("id-%03d", n)
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
}

func newTestService() *Service {
	svc := NewService(NewMemoryStore(), testIDGen())
	svc.withNow(fixedTime)
	return svc
}

func mustCreateActiveMember(t *testing.T, svc *Service) *Member {
	t.Helper()
	m, err := svc.CreateMember("user-1", "Test Corp", EntityCorporate, TierSpeculator)
	if err != nil {
		t.Fatalf("CreateMember: %v", err)
	}
	m, err = svc.Activate(m.ID, "admin-1")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	return m
}

// --- CreateMember tests ---

func TestCreateMember_Success(t *testing.T) {
	svc := newTestService()

	m, err := svc.CreateMember("user-1", "Farmer Co-op", EntityCooperative, TierFarmer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.ID == "" {
		t.Error("expected non-empty ID")
	}
	if m.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", m.UserID, "user-1")
	}
	if m.LegalName != "Farmer Co-op" {
		t.Errorf("LegalName = %q, want %q", m.LegalName, "Farmer Co-op")
	}
	if m.EntityType != EntityCooperative {
		t.Errorf("EntityType = %q, want %q", m.EntityType, EntityCooperative)
	}
	if m.Tier != TierFarmer {
		t.Errorf("Tier = %q, want %q", m.Tier, TierFarmer)
	}
	if m.Status != StatusPending {
		t.Errorf("Status = %q, want %q", m.Status, StatusPending)
	}
	if m.OnboardedAt != nil {
		t.Error("OnboardedAt should be nil for new member")
	}
}

func TestCreateMember_MissingUserID(t *testing.T) {
	svc := newTestService()
	_, err := svc.CreateMember("", "Name", EntityIndividual, TierSpeculator)
	if !errors.Is(err, ErrMissingUserID) {
		t.Errorf("expected ErrMissingUserID, got %v", err)
	}
}

func TestCreateMember_MissingLegalName(t *testing.T) {
	svc := newTestService()
	_, err := svc.CreateMember("user-1", "", EntityIndividual, TierSpeculator)
	if !errors.Is(err, ErrMissingLegalName) {
		t.Errorf("expected ErrMissingLegalName, got %v", err)
	}
}

func TestCreateMember_InvalidTier(t *testing.T) {
	svc := newTestService()
	_, err := svc.CreateMember("user-1", "Name", EntityIndividual, Tier("vip"))
	if !errors.Is(err, ErrInvalidTier) {
		t.Errorf("expected ErrInvalidTier, got %v", err)
	}
}

func TestCreateMember_RecordsHistory(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Name", EntityIndividual, TierHedger)

	history, err := svc.GetMemberHistory(m.ID)
	if err != nil {
		t.Fatalf("GetMemberHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Action != ActionCreated {
		t.Errorf("Action = %q, want %q", history[0].Action, ActionCreated)
	}
	if history[0].NewValue != string(TierHedger) {
		t.Errorf("NewValue = %q, want %q", history[0].NewValue, TierHedger)
	}
}

func TestCreateMember_AllTiers(t *testing.T) {
	tiers := []Tier{TierFarmer, TierHedger, TierSpeculator, TierMarketMaker, TierClearingMember}
	svc := newTestService()

	for _, tier := range tiers {
		t.Run(string(tier), func(t *testing.T) {
			m, err := svc.CreateMember("u-"+string(tier), "Name", EntityIndividual, tier)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Tier != tier {
				t.Errorf("Tier = %q, want %q", m.Tier, tier)
			}
		})
	}
}

// --- Activate tests ---

func TestActivate_Success(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Name", EntityIndividual, TierSpeculator)

	activated, err := svc.Activate(m.ID, "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activated.Status != StatusActive {
		t.Errorf("Status = %q, want %q", activated.Status, StatusActive)
	}
	if activated.OnboardedAt == nil {
		t.Error("OnboardedAt should be set after activation")
	}
}

func TestActivate_NotPending(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)

	_, err := svc.Activate(m.ID, "admin-1")
	if !errors.Is(err, ErrNotPending) {
		t.Errorf("expected ErrNotPending, got %v", err)
	}
}

func TestActivate_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.Activate("nonexistent", "admin-1")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

func TestActivate_RecordsHistory(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Name", EntityIndividual, TierSpeculator)
	svc.Activate(m.ID, "admin-1")

	history, _ := svc.GetMemberHistory(m.ID)
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if history[1].Action != ActionActivated {
		t.Errorf("Action = %q, want %q", history[1].Action, ActionActivated)
	}
	if history[1].OldValue != string(StatusPending) {
		t.Errorf("OldValue = %q, want %q", history[1].OldValue, StatusPending)
	}
	if history[1].NewValue != string(StatusActive) {
		t.Errorf("NewValue = %q, want %q", history[1].NewValue, StatusActive)
	}
	if history[1].ActorID != "admin-1" {
		t.Errorf("ActorID = %q, want %q", history[1].ActorID, "admin-1")
	}
}

// --- ChangeTier tests ---

func TestChangeTier_Success(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)

	updated, err := svc.ChangeTier(m.ID, TierMarketMaker, "promoted", "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Tier != TierMarketMaker {
		t.Errorf("Tier = %q, want %q", updated.Tier, TierMarketMaker)
	}
}

func TestChangeTier_InvalidTier(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)

	_, err := svc.ChangeTier(m.ID, Tier("gold"), "reason", "admin-1")
	if !errors.Is(err, ErrInvalidTier) {
		t.Errorf("expected ErrInvalidTier, got %v", err)
	}
}

func TestChangeTier_SameTier(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)

	_, err := svc.ChangeTier(m.ID, TierSpeculator, "no change", "admin-1")
	if !errors.Is(err, ErrSameTier) {
		t.Errorf("expected ErrSameTier, got %v", err)
	}
}

func TestChangeTier_NotActive(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Name", EntityIndividual, TierSpeculator)

	_, err := svc.ChangeTier(m.ID, TierHedger, "reason", "admin-1")
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("expected ErrNotActive, got %v", err)
	}
}

func TestChangeTier_Terminated(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)
	svc.Terminate(m.ID, "done", "admin-1")

	_, err := svc.ChangeTier(m.ID, TierHedger, "reason", "admin-1")
	if !errors.Is(err, ErrAlreadyTerminated) {
		t.Errorf("expected ErrAlreadyTerminated, got %v", err)
	}
}

func TestChangeTier_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.ChangeTier("nonexistent", TierHedger, "reason", "admin-1")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

func TestChangeTier_RecordsHistory(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)

	svc.ChangeTier(m.ID, TierMarketMaker, "promoted", "admin-1")
	history, _ := svc.GetMemberHistory(m.ID)

	// CREATED, ACTIVATED, TIER_CHANGED
	if len(history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(history))
	}
	last := history[2]
	if last.Action != ActionTierChanged {
		t.Errorf("Action = %q, want %q", last.Action, ActionTierChanged)
	}
	if last.OldValue != string(TierSpeculator) {
		t.Errorf("OldValue = %q, want %q", last.OldValue, TierSpeculator)
	}
	if last.NewValue != string(TierMarketMaker) {
		t.Errorf("NewValue = %q, want %q", last.NewValue, TierMarketMaker)
	}
	if last.Reason != "promoted" {
		t.Errorf("Reason = %q, want %q", last.Reason, "promoted")
	}
}

// --- Suspend tests ---

func TestSuspend_Success(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)

	suspended, err := svc.Suspend(m.ID, "compliance violation", "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suspended.Status != StatusSuspended {
		t.Errorf("Status = %q, want %q", suspended.Status, StatusSuspended)
	}
}

func TestSuspend_NotActive(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Name", EntityIndividual, TierSpeculator)

	_, err := svc.Suspend(m.ID, "reason", "admin-1")
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("expected ErrNotActive, got %v", err)
	}
}

func TestSuspend_Terminated(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)
	svc.Terminate(m.ID, "done", "admin-1")

	_, err := svc.Suspend(m.ID, "reason", "admin-1")
	if !errors.Is(err, ErrAlreadyTerminated) {
		t.Errorf("expected ErrAlreadyTerminated, got %v", err)
	}
}

func TestSuspend_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.Suspend("nonexistent", "reason", "admin-1")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

func TestSuspend_RecordsHistory(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)
	svc.Suspend(m.ID, "bad behavior", "admin-1")

	history, _ := svc.GetMemberHistory(m.ID)
	last := history[len(history)-1]
	if last.Action != ActionSuspended {
		t.Errorf("Action = %q, want %q", last.Action, ActionSuspended)
	}
	if last.Reason != "bad behavior" {
		t.Errorf("Reason = %q, want %q", last.Reason, "bad behavior")
	}
}

// --- Reinstate tests ---

func TestReinstate_Success(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)
	svc.Suspend(m.ID, "reason", "admin-1")

	reinstated, err := svc.Reinstate(m.ID, "admin-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reinstated.Status != StatusActive {
		t.Errorf("Status = %q, want %q", reinstated.Status, StatusActive)
	}
}

func TestReinstate_NotSuspended(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)

	_, err := svc.Reinstate(m.ID, "admin-1")
	if !errors.Is(err, ErrNotSuspended) {
		t.Errorf("expected ErrNotSuspended, got %v", err)
	}
}

func TestReinstate_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.Reinstate("nonexistent", "admin-1")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

func TestReinstate_RecordsHistory(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)
	svc.Suspend(m.ID, "reason", "admin-1")
	svc.Reinstate(m.ID, "admin-2")

	history, _ := svc.GetMemberHistory(m.ID)
	last := history[len(history)-1]
	if last.Action != ActionReinstated {
		t.Errorf("Action = %q, want %q", last.Action, ActionReinstated)
	}
	if last.ActorID != "admin-2" {
		t.Errorf("ActorID = %q, want %q", last.ActorID, "admin-2")
	}
}

// --- Terminate tests ---

func TestTerminate_FromActive(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)

	terminated, err := svc.Terminate(m.ID, "breach of contract", "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if terminated.Status != StatusTerminated {
		t.Errorf("Status = %q, want %q", terminated.Status, StatusTerminated)
	}
}

func TestTerminate_FromPending(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Name", EntityIndividual, TierSpeculator)

	terminated, err := svc.Terminate(m.ID, "rejected", "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if terminated.Status != StatusTerminated {
		t.Errorf("Status = %q, want %q", terminated.Status, StatusTerminated)
	}
}

func TestTerminate_AlreadyTerminated(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)
	svc.Terminate(m.ID, "done", "admin-1")

	_, err := svc.Terminate(m.ID, "again", "admin-1")
	if !errors.Is(err, ErrAlreadyTerminated) {
		t.Errorf("expected ErrAlreadyTerminated, got %v", err)
	}
}

func TestTerminate_FromSuspended(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)
	svc.Suspend(m.ID, "reason", "admin-1")

	_, err := svc.Terminate(m.ID, "done", "admin-1")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestTerminate_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.Terminate("nonexistent", "reason", "admin-1")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

func TestTerminate_RecordsHistory(t *testing.T) {
	svc := newTestService()
	m := mustCreateActiveMember(t, svc)
	svc.Terminate(m.ID, "breach", "admin-1")

	history, _ := svc.GetMemberHistory(m.ID)
	last := history[len(history)-1]
	if last.Action != ActionTerminated {
		t.Errorf("Action = %q, want %q", last.Action, ActionTerminated)
	}
	if last.Reason != "breach" {
		t.Errorf("Reason = %q, want %q", last.Reason, "breach")
	}
}

// --- GetMember tests ---

func TestGetMember_Success(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Name", EntityIndividual, TierSpeculator)

	got, err := svc.GetMember(m.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != m.ID {
		t.Errorf("ID = %q, want %q", got.ID, m.ID)
	}
}

func TestGetMember_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.GetMember("nonexistent")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

// --- ListMembers tests ---

func TestListMembers_NoFilter(t *testing.T) {
	svc := newTestService()
	svc.CreateMember("user-1", "A", EntityIndividual, TierFarmer)
	svc.CreateMember("user-2", "B", EntityCorporate, TierHedger)

	members, err := svc.ListMembers(ListFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

func TestListMembers_FilterByStatus(t *testing.T) {
	svc := newTestService()
	m1, _ := svc.CreateMember("user-1", "A", EntityIndividual, TierFarmer)
	svc.CreateMember("user-2", "B", EntityCorporate, TierHedger)
	svc.Activate(m1.ID, "admin-1")

	status := StatusActive
	members, err := svc.ListMembers(ListFilter{Status: &status})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 active member, got %d", len(members))
	}
	if members[0].UserID != "user-1" {
		t.Errorf("expected user-1, got %s", members[0].UserID)
	}
}

func TestListMembers_FilterByTier(t *testing.T) {
	svc := newTestService()
	svc.CreateMember("user-1", "A", EntityIndividual, TierFarmer)
	svc.CreateMember("user-2", "B", EntityCorporate, TierHedger)
	svc.CreateMember("user-3", "C", EntityIndividual, TierFarmer)

	tier := TierFarmer
	members, err := svc.ListMembers(ListFilter{Tier: &tier})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 farmer members, got %d", len(members))
	}
}

func TestListMembers_FilterByEntityType(t *testing.T) {
	svc := newTestService()
	svc.CreateMember("user-1", "A", EntityIndividual, TierFarmer)
	svc.CreateMember("user-2", "B", EntityCorporate, TierHedger)

	et := EntityCorporate
	members, err := svc.ListMembers(ListFilter{EntityType: &et})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 corporate member, got %d", len(members))
	}
}

func TestListMembers_Empty(t *testing.T) {
	svc := newTestService()
	members, err := svc.ListMembers(ListFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

// --- GetMemberHistory tests ---

func TestGetMemberHistory_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.GetMemberHistory("nonexistent")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

// --- GetTierConfig tests ---

func TestGetTierConfig_Success(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Name", EntityIndividual, TierMarketMaker)

	cfg, err := svc.GetTierConfig(m.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FeeRateBps != 3 {
		t.Errorf("FeeRateBps = %d, want 3", cfg.FeeRateBps)
	}
	if cfg.PositionLimit != 500 {
		t.Errorf("PositionLimit = %d, want 500", cfg.PositionLimit)
	}
	if cfg.OrderSizeLimit != 5000 {
		t.Errorf("OrderSizeLimit = %d, want 5000", cfg.OrderSizeLimit)
	}
}

func TestGetTierConfig_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.GetTierConfig("nonexistent")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

func TestGetTierConfig_AllTiers(t *testing.T) {
	svc := newTestService()
	tiers := []Tier{TierFarmer, TierHedger, TierSpeculator, TierMarketMaker, TierClearingMember}

	for _, tier := range tiers {
		t.Run(string(tier), func(t *testing.T) {
			m, _ := svc.CreateMember("u-"+string(tier), "Name", EntityIndividual, tier)
			cfg, err := svc.GetTierConfig(m.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Tier != tier {
				t.Errorf("Tier = %q, want %q", cfg.Tier, tier)
			}
			if cfg.FeeRateBps <= 0 {
				t.Error("FeeRateBps should be > 0")
			}
			if cfg.PositionLimit <= 0 {
				t.Error("PositionLimit should be > 0")
			}
			if cfg.OrderSizeLimit <= 0 {
				t.Error("OrderSizeLimit should be > 0")
			}
		})
	}
}

// --- Full lifecycle tests ---

func TestFullLifecycle_HappyPath(t *testing.T) {
	svc := newTestService()

	// Create
	m, err := svc.CreateMember("user-1", "Acme Corp", EntityCorporate, TierSpeculator)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.Status != StatusPending {
		t.Fatalf("expected PENDING, got %s", m.Status)
	}

	// Activate (KYC approved)
	m, err = svc.Activate(m.ID, "kyc-system")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if m.Status != StatusActive {
		t.Fatalf("expected ACTIVE, got %s", m.Status)
	}

	// Change tier
	m, err = svc.ChangeTier(m.ID, TierMarketMaker, "volume qualification", "admin-1")
	if err != nil {
		t.Fatalf("ChangeTier: %v", err)
	}
	if m.Tier != TierMarketMaker {
		t.Fatalf("expected market_maker tier, got %s", m.Tier)
	}

	// Suspend
	m, err = svc.Suspend(m.ID, "investigation", "compliance-1")
	if err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if m.Status != StatusSuspended {
		t.Fatalf("expected SUSPENDED, got %s", m.Status)
	}

	// Reinstate
	m, err = svc.Reinstate(m.ID, "compliance-1")
	if err != nil {
		t.Fatalf("Reinstate: %v", err)
	}
	if m.Status != StatusActive {
		t.Fatalf("expected ACTIVE, got %s", m.Status)
	}

	// Terminate
	m, err = svc.Terminate(m.ID, "voluntary exit", "admin-1")
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if m.Status != StatusTerminated {
		t.Fatalf("expected TERMINATED, got %s", m.Status)
	}

	// Verify full history
	history, err := svc.GetMemberHistory(m.ID)
	if err != nil {
		t.Fatalf("GetMemberHistory: %v", err)
	}
	expectedActions := []HistoryAction{
		ActionCreated,
		ActionActivated,
		ActionTierChanged,
		ActionSuspended,
		ActionReinstated,
		ActionTerminated,
	}
	if len(history) != len(expectedActions) {
		t.Fatalf("expected %d history entries, got %d", len(expectedActions), len(history))
	}
	for i, expected := range expectedActions {
		if history[i].Action != expected {
			t.Errorf("history[%d].Action = %q, want %q", i, history[i].Action, expected)
		}
	}
}

func TestFullLifecycle_PendingToTerminated(t *testing.T) {
	svc := newTestService()
	m, _ := svc.CreateMember("user-1", "Rejected Corp", EntityCorporate, TierSpeculator)

	m, err := svc.Terminate(m.ID, "KYC rejected", "kyc-system")
	if err != nil {
		t.Fatalf("Terminate from PENDING: %v", err)
	}
	if m.Status != StatusTerminated {
		t.Errorf("Status = %q, want %q", m.Status, StatusTerminated)
	}
}

// --- Member.Validate tests ---

func TestMemberValidate(t *testing.T) {
	tests := []struct {
		name    string
		member  Member
		wantErr error
	}{
		{
			name:    "valid",
			member:  Member{UserID: "u1", LegalName: "Name", Tier: TierFarmer},
			wantErr: nil,
		},
		{
			name:    "missing user_id",
			member:  Member{LegalName: "Name", Tier: TierFarmer},
			wantErr: ErrMissingUserID,
		},
		{
			name:    "missing legal_name",
			member:  Member{UserID: "u1", Tier: TierFarmer},
			wantErr: ErrMissingLegalName,
		},
		{
			name:    "invalid tier",
			member:  Member{UserID: "u1", LegalName: "Name", Tier: Tier("invalid")},
			wantErr: ErrInvalidTier,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.member.Validate()
			if tt.wantErr == nil && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

// --- IsValidTransition tests ---

func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		from, to Status
		valid    bool
	}{
		{StatusPending, StatusActive, true},
		{StatusActive, StatusSuspended, true},
		{StatusSuspended, StatusActive, true},
		{StatusActive, StatusTerminated, true},
		{StatusPending, StatusTerminated, true},
		// Invalid transitions
		{StatusPending, StatusSuspended, false},
		{StatusSuspended, StatusTerminated, false},
		{StatusTerminated, StatusActive, false},
		{StatusTerminated, StatusPending, false},
		{StatusActive, StatusPending, false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s->%s", tt.from, tt.to)
		t.Run(name, func(t *testing.T) {
			got := IsValidTransition(tt.from, tt.to)
			if got != tt.valid {
				t.Errorf("IsValidTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.valid)
			}
		})
	}
}

// --- DefaultTierConfigs tests ---

func TestDefaultTierConfigs(t *testing.T) {
	configs := DefaultTierConfigs()
	if len(configs) != 5 {
		t.Errorf("expected 5 tier configs, got %d", len(configs))
	}

	// Farmers should have lowest fees (they're producers)
	if configs[TierFarmer].FeeRateBps >= configs[TierSpeculator].FeeRateBps {
		t.Error("farmer fee should be lower than speculator fee")
	}

	// Market makers should have lower fees than speculators (they provide liquidity)
	if configs[TierMarketMaker].FeeRateBps >= configs[TierSpeculator].FeeRateBps {
		t.Error("market maker fee should be lower than speculator fee")
	}

	// Clearing members should have highest position limits
	if configs[TierClearingMember].PositionLimit <= configs[TierSpeculator].PositionLimit {
		t.Error("clearing member position limit should be higher than speculator")
	}
}

// --- MemoryStore edge case tests ---

func TestMemoryStore_SaveMember_DoesNotMutateOriginal(t *testing.T) {
	store := NewMemoryStore()
	m := &Member{ID: "m1", UserID: "u1", LegalName: "Original", Tier: TierFarmer, Status: StatusPending}
	store.SaveMember(m)

	m.LegalName = "Modified"

	got, _ := store.GetMember("m1")
	if got.LegalName != "Original" {
		t.Errorf("store should have a copy, got %q", got.LegalName)
	}
}

func TestMemoryStore_GetMember_DoesNotMutateStored(t *testing.T) {
	store := NewMemoryStore()
	m := &Member{ID: "m1", UserID: "u1", LegalName: "Original", Tier: TierFarmer, Status: StatusPending}
	store.SaveMember(m)

	got, _ := store.GetMember("m1")
	got.LegalName = "Modified"

	got2, _ := store.GetMember("m1")
	if got2.LegalName != "Original" {
		t.Errorf("store should return copies, got %q", got2.LegalName)
	}
}

func TestMemoryStore_GetHistory_Empty(t *testing.T) {
	store := NewMemoryStore()
	history, err := store.GetHistory("m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}
}
