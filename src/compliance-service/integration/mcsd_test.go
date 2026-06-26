package integration

import (
	"context"
	"testing"

	"github.com/garudax-platform/decimal"
)

const testTenant = "mse-equities"

func newAccount(t *testing.T, a *StubAdapter, owner string) *CustodyAccount {
	t.Helper()
	acct, err := a.CreateCustodyAccount(context.Background(), CreateAccountRequest{
		TenantID: testTenant, OwnerID: owner, Name: owner + " custody", Currency: "MNT",
	})
	if err != nil {
		t.Fatalf("CreateCustodyAccount: %v", err)
	}
	return acct
}

func TestCreateAccountValidation(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	if _, err := a.CreateCustodyAccount(ctx, CreateAccountRequest{OwnerID: "o", Name: "n"}); err != ErrMissingTenant {
		t.Fatalf("expected ErrMissingTenant, got %v", err)
	}
	if _, err := a.CreateCustodyAccount(ctx, CreateAccountRequest{TenantID: testTenant}); err != ErrMissingFields {
		t.Fatalf("expected ErrMissingFields, got %v", err)
	}
	acct := newAccount(t, a, "broker-1")
	if acct.AccountID == "" || acct.TenantID != testTenant {
		t.Fatalf("bad account: %+v", acct)
	}
}

func TestGetBalanceZeroAndCredit(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	acct := newAccount(t, a, "broker-1")

	bal, err := a.GetBalance(ctx, acct.AccountID, "MSE:APU")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal.Quantity != 0 {
		t.Fatalf("expected zero balance, got %d", bal.Quantity)
	}
	if _, err := a.GetBalance(ctx, "nope", "MSE:APU"); err != ErrAccountNotFound {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}

	if err := a.Credit(acct.AccountID, "MSE:APU", 1000); err != nil {
		t.Fatalf("Credit: %v", err)
	}
	bal, _ = a.GetBalance(ctx, acct.AccountID, "MSE:APU")
	if bal.Quantity != 1000 {
		t.Fatalf("expected 1000, got %d", bal.Quantity)
	}
	if err := a.Credit(acct.AccountID, "MSE:APU", -5); err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity, got %v", err)
	}
}

func TestInstructDvPSettlesImmediately(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	from := newAccount(t, a, "seller")
	to := newAccount(t, a, "buyer")
	if err := a.Credit(from.AccountID, "MSE:APU", 1000); err != nil {
		t.Fatal(err)
	}

	resp, err := a.InstructDvP(ctx, DvPInstruction{
		TenantID: testTenant, FromAccountID: from.AccountID, ToAccountID: to.AccountID,
		InstrumentID: "MSE:APU", Quantity: 400, SettlementAmount: decimal.DecimalFromInt(8000), Currency: "MNT", SettlementDate: "2026-06-23",
	})
	if err != nil {
		t.Fatalf("InstructDvP: %v", err)
	}
	if resp.State != StateSettled {
		t.Fatalf("state = %s, want SETTLED", resp.State)
	}
	if resp.MessageID != MsgSettlementInstruction {
		t.Fatalf("message id = %s", resp.MessageID)
	}

	fromBal, _ := a.GetBalance(ctx, from.AccountID, "MSE:APU")
	toBal, _ := a.GetBalance(ctx, to.AccountID, "MSE:APU")
	if fromBal.Quantity != 600 || toBal.Quantity != 400 {
		t.Fatalf("balances after settle: from=%d to=%d", fromBal.Quantity, toBal.Quantity)
	}

	st, err := a.GetTransferStatus(ctx, resp.TransferID)
	if err != nil {
		t.Fatalf("GetTransferStatus: %v", err)
	}
	if st.State != StateSettled || st.SettledAt.IsZero() {
		t.Fatalf("status wrong: %+v", st)
	}
}

func TestInstructValidation(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	from := newAccount(t, a, "seller")
	to := newAccount(t, a, "buyer")

	cases := []struct {
		name string
		req  DvPInstruction
		want error
	}{
		{"missing tenant", DvPInstruction{FromAccountID: from.AccountID, ToAccountID: to.AccountID, InstrumentID: "i", Quantity: 1, SettlementAmount: decimal.DecimalFromInt(1)}, ErrMissingTenant},
		{"bad amount", DvPInstruction{TenantID: testTenant, FromAccountID: from.AccountID, ToAccountID: to.AccountID, InstrumentID: "i", Quantity: 1, SettlementAmount: decimal.Zero()}, ErrInvalidAmount},
		{"missing fields", DvPInstruction{TenantID: testTenant, FromAccountID: "", ToAccountID: to.AccountID, InstrumentID: "i", Quantity: 1, SettlementAmount: decimal.DecimalFromInt(1)}, ErrMissingFields},
		{"bad qty", DvPInstruction{TenantID: testTenant, FromAccountID: from.AccountID, ToAccountID: to.AccountID, InstrumentID: "i", Quantity: 0, SettlementAmount: decimal.DecimalFromInt(1)}, ErrInvalidQuantity},
	}
	for _, c := range cases {
		if _, err := a.InstructDvP(ctx, c.req); err != c.want {
			t.Fatalf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestInsufficientHoldings(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	from := newAccount(t, a, "seller")
	to := newAccount(t, a, "buyer")
	a.Credit(from.AccountID, "MSE:APU", 100)

	_, err := a.InstructFoP(ctx, FoPInstruction{
		TenantID: testTenant, FromAccountID: from.AccountID, ToAccountID: to.AccountID,
		InstrumentID: "MSE:APU", Quantity: 500,
	})
	if err != ErrInsufficientHoldings {
		t.Fatalf("expected ErrInsufficientHoldings, got %v", err)
	}
	// Balance must be unchanged after a failed instruction.
	bal, _ := a.GetBalance(ctx, from.AccountID, "MSE:APU")
	if bal.Quantity != 100 {
		t.Fatalf("balance changed after failed transfer: %d", bal.Quantity)
	}
}

func TestTenantIsolationRejectsCrossTenantTransfer(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	from := newAccount(t, a, "seller")
	other, err := a.CreateCustodyAccount(ctx, CreateAccountRequest{
		TenantID: "ace-commodities", OwnerID: "x", Name: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	a.Credit(from.AccountID, "MSE:APU", 100)
	_, err = a.InstructFoP(ctx, FoPInstruction{
		TenantID: testTenant, FromAccountID: from.AccountID, ToAccountID: other.AccountID,
		InstrumentID: "MSE:APU", Quantity: 10,
	})
	if err != ErrTenantMismatch {
		t.Fatalf("expected ErrTenantMismatch, got %v", err)
	}
}

func TestManualAffirmSettleHandshake(t *testing.T) {
	a := NewStubAdapter()
	a.AutoSettle = false
	ctx := context.Background()
	from := newAccount(t, a, "seller")
	to := newAccount(t, a, "buyer")
	a.Credit(from.AccountID, "MSE:APU", 1000)

	resp, err := a.InstructFoP(ctx, FoPInstruction{
		TenantID: testTenant, FromAccountID: from.AccountID, ToAccountID: to.AccountID,
		InstrumentID: "MSE:APU", Quantity: 300,
	})
	if err != nil {
		t.Fatalf("InstructFoP: %v", err)
	}
	if resp.State != StatePending {
		t.Fatalf("state = %s, want PENDING", resp.State)
	}
	// Pending reservation is reflected but settled qty unchanged.
	fromBal, _ := a.GetBalance(ctx, from.AccountID, "MSE:APU")
	if fromBal.Quantity != 1000 || fromBal.Pending != -300 {
		t.Fatalf("pending reservation wrong: qty=%d pending=%d", fromBal.Quantity, fromBal.Pending)
	}

	if _, err := a.Affirm(resp.TransferID); err != nil {
		t.Fatalf("Affirm: %v", err)
	}
	st, err := a.Settle(resp.TransferID)
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if st.State != StateSettled {
		t.Fatalf("state = %s, want SETTLED", st.State)
	}
	fromBal, _ = a.GetBalance(ctx, from.AccountID, "MSE:APU")
	toBal, _ := a.GetBalance(ctx, to.AccountID, "MSE:APU")
	if fromBal.Quantity != 700 || fromBal.Pending != 0 {
		t.Fatalf("from after settle: qty=%d pending=%d", fromBal.Quantity, fromBal.Pending)
	}
	if toBal.Quantity != 300 || toBal.Pending != 0 {
		t.Fatalf("to after settle: qty=%d pending=%d", toBal.Quantity, toBal.Pending)
	}
}

func TestFailReleasesReservation(t *testing.T) {
	a := NewStubAdapter()
	a.AutoSettle = false
	ctx := context.Background()
	from := newAccount(t, a, "seller")
	to := newAccount(t, a, "buyer")
	a.Credit(from.AccountID, "MSE:APU", 1000)

	resp, _ := a.InstructFoP(ctx, FoPInstruction{
		TenantID: testTenant, FromAccountID: from.AccountID, ToAccountID: to.AccountID,
		InstrumentID: "MSE:APU", Quantity: 300,
	})
	st, err := a.Fail(resp.TransferID, "counterparty default")
	if err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if st.State != StateFailed || st.Reason != "counterparty default" {
		t.Fatalf("fail status wrong: %+v", st)
	}
	fromBal, _ := a.GetBalance(ctx, from.AccountID, "MSE:APU")
	if fromBal.Pending != 0 || fromBal.Quantity != 1000 {
		t.Fatalf("reservation not released: qty=%d pending=%d", fromBal.Quantity, fromBal.Pending)
	}
	// Re-failing a terminal transfer is invalid.
	if _, err := a.Fail(resp.TransferID, "x"); err != ErrInvalidState {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
}

func TestTransferStatusNotFound(t *testing.T) {
	a := NewStubAdapter()
	if _, err := a.GetTransferStatus(context.Background(), "nope"); err != ErrTransferNotFound {
		t.Fatalf("expected ErrTransferNotFound, got %v", err)
	}
}

func TestCorporateActionEntitlements(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	h1 := newAccount(t, a, "holder-1")
	h2 := newAccount(t, a, "holder-2")
	empty := newAccount(t, a, "holder-3")
	_ = empty
	a.Credit(h1.AccountID, "MSE:APU", 1000)
	a.Credit(h2.AccountID, "MSE:APU", 500)

	// Dividend of 2.50 per share.
	if err := a.NotifyCorporateAction(ctx, CorporateAction{
		ActionID: "ca-div-1", TenantID: testTenant, InstrumentID: "MSE:APU",
		Type: "DIVIDEND", RecordDate: "2026-06-19", RatioOrAmount: 2.50,
	}); err != nil {
		t.Fatalf("NotifyCorporateAction: %v", err)
	}
	ents, err := a.GetEntitlements(ctx, "ca-div-1")
	if err != nil {
		t.Fatalf("GetEntitlements: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("expected 2 entitlements (holders with positions), got %d", len(ents))
	}
	byOwner := map[string]Entitlement{}
	for _, e := range ents {
		byOwner[e.OwnerID] = e
	}
	if !byOwner["holder-1"].CashEntitlement.Equal(decimal.DecimalFromInt(2500)) {
		t.Fatalf("holder-1 cash = %v, want 2500", byOwner["holder-1"].CashEntitlement)
	}
	if !byOwner["holder-2"].CashEntitlement.Equal(decimal.DecimalFromInt(1250)) {
		t.Fatalf("holder-2 cash = %v, want 1250", byOwner["holder-2"].CashEntitlement)
	}
}

func TestCorporateActionShareEntitlementAndValidation(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	h := newAccount(t, a, "holder-1")
	a.Credit(h.AccountID, "MSE:APU", 1000)

	// 1:1 bonus issue (STOCK_SPLIT ratio 1.0 → +1000 shares).
	if err := a.NotifyCorporateAction(ctx, CorporateAction{
		ActionID: "ca-split-1", TenantID: testTenant, InstrumentID: "MSE:APU",
		Type: "STOCK_SPLIT", RatioOrAmount: 1.0,
	}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	ents, _ := a.GetEntitlements(ctx, "ca-split-1")
	if len(ents) != 1 || ents[0].ShareEntitlement != 1000 {
		t.Fatalf("share entitlement wrong: %+v", ents)
	}

	// Validation paths.
	if err := a.NotifyCorporateAction(ctx, CorporateAction{InstrumentID: "i", Type: "DIVIDEND", ActionID: "x"}); err != ErrMissingTenant {
		t.Fatalf("expected ErrMissingTenant, got %v", err)
	}
	if err := a.NotifyCorporateAction(ctx, CorporateAction{TenantID: testTenant, ActionID: "x"}); err != ErrMissingFields {
		t.Fatalf("expected ErrMissingFields, got %v", err)
	}
	if _, err := a.GetEntitlements(ctx, "does-not-exist"); err == nil {
		t.Fatal("expected error for unknown action")
	}
}

// TestConcurrentInstructions exercises the mutex under parallel transfers.
func TestConcurrentInstructions(t *testing.T) {
	a := NewStubAdapter()
	ctx := context.Background()
	from := newAccount(t, a, "seller")
	to := newAccount(t, a, "buyer")
	a.Credit(from.AccountID, "MSE:APU", 1000)

	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := a.InstructFoP(ctx, FoPInstruction{
				TenantID: testTenant, FromAccountID: from.AccountID, ToAccountID: to.AccountID,
				InstrumentID: "MSE:APU", Quantity: 100,
			})
			done <- err
		}()
	}
	settled := 0
	for i := 0; i < 10; i++ {
		if err := <-done; err == nil {
			settled++
		}
	}
	// Exactly 10 transfers of 100 against 1000 held should all settle.
	if settled != 10 {
		t.Fatalf("settled = %d, want 10", settled)
	}
	fromBal, _ := a.GetBalance(ctx, from.AccountID, "MSE:APU")
	toBal, _ := a.GetBalance(ctx, to.AccountID, "MSE:APU")
	if fromBal.Quantity != 0 || toBal.Quantity != 1000 {
		t.Fatalf("final balances: from=%d to=%d", fromBal.Quantity, toBal.Quantity)
	}
}
