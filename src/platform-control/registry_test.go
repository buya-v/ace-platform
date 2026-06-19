package main

import (
	"errors"
	"testing"
)

func newReq(id, name string) CreateTenantRequest {
	return CreateTenantRequest{ID: id, DisplayName: name}
}

func TestSeededRegistry(t *testing.T) {
	r := NewSeededRegistry()
	all := r.List("")
	if len(all) != 2 {
		t.Fatalf("expected 2 seeded tenants, got %d", len(all))
	}
	// Sorted by ID: ace-commodities first.
	if all[0].ID != "ace-commodities" || all[1].ID != "mse-equities" {
		t.Fatalf("unexpected seed order: %s, %s", all[0].ID, all[1].ID)
	}
	mse, err := r.Get("mse-equities")
	if err != nil {
		t.Fatalf("get mse: %v", err)
	}
	if !mse.Flagship || mse.GovernanceTier != TierFlagship || mse.Status != StatusOnboarding {
		t.Fatalf("mse seed fields wrong: %+v", mse)
	}
}

func TestGet_NotFound(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreate_Success_Defaults(t *testing.T) {
	r := NewTenantRegistry()
	got, err := r.Create(newReq("baganuur-coal", "Baganuur Coal Exchange"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.Status != StatusOnboarding {
		t.Errorf("new tenant must start ONBOARDING, got %s", got.Status)
	}
	if got.GovernanceTier != TierStandard {
		t.Errorf("default tier should be STANDARD, got %s", got.GovernanceTier)
	}
	if got.PrimaryCurrency != "MNT" || got.Timezone != "Asia/Ulaanbaatar" || got.DefaultSettlementCycle != "T+0" {
		t.Errorf("defaults not applied: %+v", got)
	}
	if got.AssetClasses == nil {
		t.Errorf("asset_classes should default to empty slice, not nil")
	}
	if got.ConfigVersion != 1 {
		t.Errorf("config version should start at 1, got %d", got.ConfigVersion)
	}
}

func TestCreate_DuplicateID(t *testing.T) {
	r := NewSeededRegistry()
	_, err := r.Create(newReq("ace-commodities", "Dup"))
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestCreate_ValidationErrors(t *testing.T) {
	r := NewTenantRegistry()
	cases := map[string]CreateTenantRequest{
		"missing id":       {DisplayName: "X"},
		"bad slug":         {ID: "Bad_ID!", DisplayName: "X"},
		"leading hyphen":   {ID: "-bad", DisplayName: "X"},
		"missing name":     {ID: "good-id"},
		"bad tier":         {ID: "good-id", DisplayName: "X", GovernanceTier: "PLATINUM"},
		"bad settle cycle": {ID: "good-id", DisplayName: "X", DefaultSettlementCycle: "T+9"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := r.Create(req)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
			var verr *ValidationError
			if !errors.As(err, &verr) || len(verr.Fields) == 0 {
				t.Fatalf("expected ValidationError with fields, got %v", err)
			}
		})
	}
}

func TestCreate_FlagshipConflict(t *testing.T) {
	r := NewSeededRegistry() // mse-equities is already flagship
	req := newReq("second-flagship", "Second")
	req.Flagship = true
	_, err := r.Create(req)
	if !errors.Is(err, ErrFlagshipConflict) {
		t.Fatalf("expected ErrFlagshipConflict, got %v", err)
	}
}

func TestCreate_FlagshipAllowedAfterDecommission(t *testing.T) {
	r := NewTenantRegistry()
	first := newReq("flag-one", "One")
	first.Flagship = true
	if _, err := r.Create(first); err != nil {
		t.Fatalf("create first flagship: %v", err)
	}
	// Decommission the first flagship, then a new flagship should be allowed.
	if _, err := r.TransitionStatus("flag-one", StatusDecommissioned, "admin", "retire"); err != nil {
		t.Fatalf("decommission: %v", err)
	}
	second := newReq("flag-two", "Two")
	second.Flagship = true
	if _, err := r.Create(second); err != nil {
		t.Fatalf("expected second flagship allowed after decommission, got %v", err)
	}
}

func TestUpdate_PartialAndConfigBump(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Create(newReq("uvs-grain", "Uvs Grain")); err != nil {
		t.Fatal(err)
	}
	newName := "Uvs Grain Exchange"
	newTier := TierSandbox
	classes := []string{"COMMODITY", "GRAIN"}
	got, err := r.Update("uvs-grain", UpdateTenantRequest{
		DisplayName:    &newName,
		GovernanceTier: &newTier,
		AssetClasses:   &classes,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.DisplayName != newName || got.GovernanceTier != newTier {
		t.Errorf("update not applied: %+v", got)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config version should bump to 2 on tier/asset change, got %d", got.ConfigVersion)
	}
}

func TestUpdate_NoConfigBumpForDescriptiveOnly(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Create(newReq("desc-only", "Desc")); err != nil {
		t.Fatal(err)
	}
	desc := "A purely descriptive change"
	got, err := r.Update("desc-only", UpdateTenantRequest{Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigVersion != 1 {
		t.Errorf("descriptive-only update should not bump config, got %d", got.ConfigVersion)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	r := NewTenantRegistry()
	name := "X"
	if _, err := r.Update("ghost", UpdateTenantRequest{DisplayName: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdate_RejectsEmptyDisplayNameAndBadTier(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Create(newReq("validate-me", "Name")); err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := r.Update("validate-me", UpdateTenantRequest{DisplayName: &empty}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error for empty name, got %v", err)
	}
	badTier := "GOLD"
	if _, err := r.Update("validate-me", UpdateTenantRequest{GovernanceTier: &badTier}); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error for bad tier, got %v", err)
	}
}

func TestUpdate_DecommissionedImmutable(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Create(newReq("gone", "Gone")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.TransitionStatus("gone", StatusDecommissioned, "admin", ""); err != nil {
		t.Fatal(err)
	}
	name := "New"
	if _, err := r.Update("gone", UpdateTenantRequest{DisplayName: &name}); !errors.Is(err, ErrTerminal) {
		t.Fatalf("expected ErrTerminal, got %v", err)
	}
}

func TestTransition_FullLifecycle(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Create(newReq("life", "Lifecycle")); err != nil {
		t.Fatal(err)
	}
	// ONBOARDING -> ACTIVE
	got, err := r.TransitionStatus("life", StatusActive, "admin", "go live")
	if err != nil || got.Status != StatusActive {
		t.Fatalf("activate failed: %v, status=%v", err, got.Status)
	}
	if got.ActivatedAt == "" {
		t.Error("activated_at not stamped")
	}
	// ACTIVE -> SUSPENDED
	got, err = r.TransitionStatus("life", StatusSuspended, "admin", "halt")
	if err != nil || got.Status != StatusSuspended {
		t.Fatalf("suspend failed: %v", err)
	}
	if got.SuspendedAt == "" {
		t.Error("suspended_at not stamped")
	}
	// SUSPENDED -> ACTIVE
	if _, err = r.TransitionStatus("life", StatusActive, "admin", "resume"); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	// ACTIVE -> DECOMMISSIONED
	got, err = r.TransitionStatus("life", StatusDecommissioned, "admin", "retire")
	if err != nil || got.Status != StatusDecommissioned {
		t.Fatalf("decommission failed: %v", err)
	}
	if got.DecommissionedAt == "" {
		t.Error("decommissioned_at not stamped")
	}
}

func TestTransition_IllegalPaths(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Create(newReq("strict", "Strict")); err != nil {
		t.Fatal(err)
	}
	// ONBOARDING -> SUSPENDED is illegal.
	if _, err := r.TransitionStatus("strict", StatusSuspended, "admin", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition onboarding->suspended, got %v", err)
	}
	// Decommission, then any transition is illegal (terminal).
	if _, err := r.TransitionStatus("strict", StatusDecommissioned, "admin", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.TransitionStatus("strict", StatusActive, "admin", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition from terminal, got %v", err)
	}
}

func TestTransition_SameStateRejected(t *testing.T) {
	r := NewSeededRegistry() // ace-commodities is ACTIVE
	if _, err := r.TransitionStatus("ace-commodities", StatusActive, "admin", ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for no-op, got %v", err)
	}
}

func TestTransition_InvalidTargetAndNotFound(t *testing.T) {
	r := NewSeededRegistry()
	if _, err := r.TransitionStatus("ace-commodities", "BOGUS", "admin", ""); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error for bogus status, got %v", err)
	}
	if _, err := r.TransitionStatus("ghost", StatusActive, "admin", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestAudit_TrailRecorded(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Create(newReq("audited", "Audited")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.TransitionStatus("audited", StatusActive, "alice", "launch"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.TransitionStatus("audited", StatusSuspended, "bob", "incident"); err != nil {
		t.Fatal(err)
	}
	entries, err := r.Audit("audited")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(entries) != 3 { // created, activated, suspended
		t.Fatalf("expected 3 audit entries, got %d", len(entries))
	}
	if entries[0].Action != "tenant.created" {
		t.Errorf("first action should be tenant.created, got %s", entries[0].Action)
	}
	if entries[2].Action != "tenant.suspended" || entries[2].Actor != "bob" || entries[2].Detail != "incident" {
		t.Errorf("suspend audit entry wrong: %+v", entries[2])
	}
	// Sequence is monotonic.
	if entries[0].Sequence >= entries[1].Sequence || entries[1].Sequence >= entries[2].Sequence {
		t.Error("audit sequence not monotonic")
	}
}

func TestAudit_NotFoundForUnknownTenant(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Audit("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestAudit_AllTenants(t *testing.T) {
	r := NewSeededRegistry()
	all, err := r.Audit("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 { // two seed entries
		t.Fatalf("expected 2 global audit entries, got %d", len(all))
	}
}

func TestActorDefaultsToPlatformAdmin(t *testing.T) {
	r := NewTenantRegistry()
	if _, err := r.Create(newReq("noactor", "No Actor")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.TransitionStatus("noactor", StatusActive, "", ""); err != nil {
		t.Fatal(err)
	}
	entries, _ := r.Audit("noactor")
	last := entries[len(entries)-1]
	if last.Actor != "platform-admin" {
		t.Errorf("empty actor should default to platform-admin, got %q", last.Actor)
	}
}
