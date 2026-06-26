package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/compliance-service/integration"
	"github.com/garudax-platform/compliance-service/internal/onboarding"
	"github.com/garudax-platform/compliance-service/internal/screening"
	"github.com/garudax-platform/compliance-service/reporting"
	"github.com/garudax-platform/decimal"
)

// newWiredServer returns an httptest.Server with FRC reporting and MCSD
// integration wired in, plus the underlying stub adapter for assertions.
func newWiredServer(t *testing.T) (*httptest.Server, *integration.StubAdapter) {
	t.Helper()
	onboardStore := onboarding.NewInMemoryStore()
	onboardSvc := onboarding.NewService(onboardStore)
	screenStore := screening.NewInMemoryStore()
	screeningSvc := screening.NewService(screenStore, nil, onboardStore)
	srv := NewServer(onboardSvc, screeningSvc, DefaultConfig())

	pub := &reporting.RecordingPublisher{}
	rep, err := reporting.NewReporter("mse-equities", pub)
	if err != nil {
		t.Fatalf("reporter: %v", err)
	}
	srv.SetFRCReporter(rep, pub)

	adapter := integration.NewStubAdapter()
	srv.SetCSDAdapter(adapter)

	mux := http.NewServeMux()
	srv.mountRoutes(mux)
	return httptest.NewServer(mux), adapter
}

func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func getURL(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func TestFRCReportGenerateAndList(t *testing.T) {
	ts, _ := newWiredServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/frc/reports", map[string]interface{}{
		"report_type":  reporting.ReportDailyTradingSummary,
		"format":       reporting.FormatJSON,
		"session_date": "2026-06-19",
		"instruments": []map[string]interface{}{
			{"instrument_id": "MSE:APU", "symbol": "APU", "trades": 5, "volume": 500, "value": 2500, "price_change": 1.2},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	delivery := created["delivery"].(map[string]interface{})
	if delivery["kafka_topic"] != "mse-equities.compliance.frc-report-generated" {
		t.Fatalf("kafka topic = %v", delivery["kafka_topic"])
	}

	// List should now show the report.
	lr := getURL(t, ts.URL+"/frc/reports")
	defer lr.Body.Close()
	var list map[string]interface{}
	json.NewDecoder(lr.Body).Decode(&list)
	if list["total"].(float64) != 1 {
		t.Fatalf("list total = %v, want 1", list["total"])
	}
}

func TestFRCReportInvalidType(t *testing.T) {
	ts, _ := newWiredServer(t)
	defer ts.Close()
	resp := postJSON(t, ts.URL+"/frc/reports", map[string]interface{}{"report_type": "BOGUS"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCSDFullFlow(t *testing.T) {
	ts, _ := newWiredServer(t)
	defer ts.Close()

	// Create two accounts.
	mk := func(owner string) string {
		resp := postJSON(t, ts.URL+"/csd/accounts", integration.CreateAccountRequest{
			TenantID: "mse-equities", OwnerID: owner, Name: owner, Currency: "MNT",
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create account %s: status %d", owner, resp.StatusCode)
		}
		var acct integration.CustodyAccount
		json.NewDecoder(resp.Body).Decode(&acct)
		return acct.AccountID
	}
	from := mk("seller")
	to := mk("buyer")

	// Seed holdings directly via the adapter is not exposed over HTTP; instead
	// use an FoP from a seeded account. Seed via a corporate-action-free path:
	// credit through the stub adapter retrieved from the flow is unavailable, so
	// we drive a DvP that fails for insufficient holdings, proving the wiring.
	resp := postJSON(t, ts.URL+"/csd/transfers/dvp", integration.DvPInstruction{
		TenantID: "mse-equities", FromAccountID: from, ToAccountID: to,
		InstrumentID: "MSE:APU", Quantity: 100, SettlementAmount: decimal.DecimalFromInt(1000),
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 insufficient holdings, got %d", resp.StatusCode)
	}

	// Balance query returns zero for a known account.
	br := getURL(t, ts.URL+"/csd/accounts/balance?account_id="+from+"&instrument_id=MSE:APU")
	defer br.Body.Close()
	if br.StatusCode != http.StatusOK {
		t.Fatalf("balance status = %d", br.StatusCode)
	}
	var bal integration.Balance
	json.NewDecoder(br.Body).Decode(&bal)
	if bal.Quantity != 0 {
		t.Fatalf("balance = %d, want 0", bal.Quantity)
	}
}

func TestCSDTransferSettlesWithSeededHoldings(t *testing.T) {
	ts, adapter := newWiredServer(t)
	defer ts.Close()

	from, _ := adapter.CreateCustodyAccount(context.Background(), integration.CreateAccountRequest{
		TenantID: "mse-equities", OwnerID: "seller", Name: "seller",
	})
	to, _ := adapter.CreateCustodyAccount(context.Background(), integration.CreateAccountRequest{
		TenantID: "mse-equities", OwnerID: "buyer", Name: "buyer",
	})
	if err := adapter.Credit(from.AccountID, "MSE:APU", 1000); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, ts.URL+"/csd/transfers/fop", integration.FoPInstruction{
		TenantID: "mse-equities", FromAccountID: from.AccountID, ToAccountID: to.AccountID,
		InstrumentID: "MSE:APU", Quantity: 250,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("fop status = %d", resp.StatusCode)
	}
	var tr integration.TransferResponse
	json.NewDecoder(resp.Body).Decode(&tr)
	if tr.State != integration.StateSettled {
		t.Fatalf("state = %s, want SETTLED", tr.State)
	}

	// Query the transfer status back.
	sr := getURL(t, ts.URL+"/csd/transfers?id="+tr.TransferID)
	defer sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		t.Fatalf("status query = %d", sr.StatusCode)
	}
}

func TestCSDCorporateActionEntitlements(t *testing.T) {
	ts, adapter := newWiredServer(t)
	defer ts.Close()
	h, _ := adapter.CreateCustodyAccount(context.Background(), integration.CreateAccountRequest{
		TenantID: "mse-equities", OwnerID: "holder", Name: "holder",
	})
	adapter.Credit(h.AccountID, "MSE:APU", 400)

	resp := postJSON(t, ts.URL+"/csd/corporate-actions", integration.CorporateAction{
		ActionID: "ca-1", TenantID: "mse-equities", InstrumentID: "MSE:APU",
		Type: "DIVIDEND", RecordDate: "2026-06-19", RatioOrAmount: 1.5,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("notify status = %d", resp.StatusCode)
	}
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["total"].(float64) != 1 {
		t.Fatalf("entitlements total = %v, want 1", body["total"])
	}

	gr := getURL(t, ts.URL+"/csd/corporate-actions?action_id=ca-1")
	defer gr.Body.Close()
	if gr.StatusCode != http.StatusOK {
		t.Fatalf("get entitlements = %d", gr.StatusCode)
	}
}

func TestCSDMissingTenantRejected(t *testing.T) {
	ts, _ := newWiredServer(t)
	defer ts.Close()
	resp := postJSON(t, ts.URL+"/csd/accounts", integration.CreateAccountRequest{OwnerID: "x", Name: "x"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing tenant, got %d", resp.StatusCode)
	}
}

// TestRoutesDisabledWhenUnwired confirms the new routes are absent on a plain server.
func TestRoutesDisabledWhenUnwired(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	for _, path := range []string{"/frc/reports", "/csd/accounts"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404 (route should be unregistered)", path, resp.StatusCode)
		}
	}
}
