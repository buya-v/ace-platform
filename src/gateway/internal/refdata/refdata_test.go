package refdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/middleware"
)

// mockStore implements Store for testing.
type mockStore struct {
	commodities []Commodity
	instruments []Instrument
	detail      *InstrumentDetail
	err         error
}

func (m *mockStore) ListCommodities(_ context.Context) ([]Commodity, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.commodities, nil
}

func (m *mockStore) ListInstruments(_ context.Context, status string) ([]Instrument, error) {
	if m.err != nil {
		return nil, m.err
	}
	if status != "" {
		var filtered []Instrument
		for _, inst := range m.instruments {
			if inst.Status == status {
				filtered = append(filtered, inst)
			}
		}
		return filtered, nil
	}
	return m.instruments, nil
}

func (m *mockStore) GetInstrument(_ context.Context, id string) (*InstrumentDetail, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.detail != nil && m.detail.ID == id {
		return m.detail, nil
	}
	return nil, nil
}

func (m *mockStore) CreateInstrument(_ context.Context, input InstrumentInput) (*Instrument, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &Instrument{ID: input.ID, CommodityID: input.CommodityID, Name: input.Name,
		ContractSize: input.ContractSize, TickSize: input.TickSize,
		Currency: input.Currency, SettlementType: input.SettlementType,
		Status: "active"}, nil
}

func (m *mockStore) UpdateInstrument(_ context.Context, id string, _ map[string]interface{}) (*Instrument, error) {
	if m.err != nil {
		return nil, m.err
	}
	for i := range m.instruments {
		if m.instruments[i].ID == id {
			return &m.instruments[i], nil
		}
	}
	return nil, nil
}

func (m *mockStore) CreateCommodity(_ context.Context, input CommodityInput) (*Commodity, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &Commodity{ID: input.ID, Name: input.Name, Category: input.Category, Unit: input.Unit}, nil
}

// --- Commodity fixtures ---

func sampleCommodities() []Commodity {
	return []Commodity{
		{
			ID:        "WHT-HRW",
			Name:      "Hard Red Winter Wheat",
			Category:  "grain",
			Unit:      "bushel",
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        "CSH-RAW",
			Name:      "Raw Cashmere",
			Category:  "fiber",
			Unit:      "kg",
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

func sampleInstruments() []Instrument {
	active := "ACTIVE"
	_ = active
	ftd := "2026-01-15"
	ltd := "2026-06-30"
	return []Instrument{
		{
			ID:             "WHT-HRW-2026M07-UB",
			CommodityID:    "WHT-HRW",
			Name:           "HRW Wheat Jul 2026",
			DeliveryMonth:  7,
			DeliveryYear:   2026,
			ContractSize:   "5000.0000",
			TickSize:       "0.00250000",
			Currency:       "MNT",
			FirstTradeDate: &ftd,
			LastTradeDate:  &ltd,
			SettlementType: "PHYSICAL",
			Status:         "ACTIVE",
			CreatedAt:      "2026-01-01T00:00:00Z",
			UpdatedAt:      "2026-01-01T00:00:00Z",
		},
		{
			ID:             "CSH-RAW-2026M09-UB",
			CommodityID:    "CSH-RAW",
			Name:           "Raw Cashmere Sep 2026",
			DeliveryMonth:  9,
			DeliveryYear:   2026,
			ContractSize:   "100.0000",
			TickSize:       "0.01000000",
			Currency:       "MNT",
			FirstTradeDate: &ftd,
			LastTradeDate:  &ltd,
			SettlementType: "PHYSICAL",
			Status:         "SUSPENDED",
			CreatedAt:      "2026-01-01T00:00:00Z",
			UpdatedAt:      "2026-01-01T00:00:00Z",
		},
	}
}

func sampleDetail() *InstrumentDetail {
	insts := sampleInstruments()
	comms := sampleCommodities()
	return &InstrumentDetail{
		Instrument: insts[0],
		Commodity:  &comms[0],
	}
}

// --- Handler tests ---

func TestListCommodities_Success(t *testing.T) {
	h := NewHandlers(&mockStore{commodities: sampleCommodities()})
	req := httptest.NewRequest("GET", "/api/v1/commodities", nil)
	rec := httptest.NewRecorder()

	h.ListCommodities(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Error("response missing 'data' field")
	}

	var commodities []Commodity
	if err := json.Unmarshal(resp["data"], &commodities); err != nil {
		t.Fatalf("invalid commodities data: %v", err)
	}
	if len(commodities) != 2 {
		t.Errorf("len(commodities) = %d, want 2", len(commodities))
	}
	if commodities[0].ID != "WHT-HRW" {
		t.Errorf("commodities[0].ID = %q, want %q", commodities[0].ID, "WHT-HRW")
	}
}

func TestListCommodities_Empty(t *testing.T) {
	h := NewHandlers(&mockStore{})
	req := httptest.NewRequest("GET", "/api/v1/commodities", nil)
	rec := httptest.NewRecorder()

	h.ListCommodities(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var commodities []Commodity
	json.Unmarshal(resp["data"], &commodities)
	if len(commodities) != 0 {
		t.Errorf("expected empty list, got %d items", len(commodities))
	}
}

func TestListCommodities_StoreError(t *testing.T) {
	h := NewHandlers(&mockStore{err: errors.New("db connection failed")})
	req := httptest.NewRequest("GET", "/api/v1/commodities", nil)
	rec := httptest.NewRecorder()

	h.ListCommodities(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestListInstruments_Success(t *testing.T) {
	h := NewHandlers(&mockStore{instruments: sampleInstruments()})
	req := httptest.NewRequest("GET", "/api/v1/instruments", nil)
	rec := httptest.NewRecorder()

	h.ListInstruments(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var instruments []Instrument
	json.Unmarshal(resp["data"], &instruments)
	if len(instruments) != 2 {
		t.Errorf("len(instruments) = %d, want 2", len(instruments))
	}
}

func TestListInstruments_FilterByStatus(t *testing.T) {
	h := NewHandlers(&mockStore{instruments: sampleInstruments()})
	req := httptest.NewRequest("GET", "/api/v1/instruments?status=ACTIVE", nil)
	rec := httptest.NewRecorder()

	h.ListInstruments(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var instruments []Instrument
	json.Unmarshal(resp["data"], &instruments)
	if len(instruments) != 1 {
		t.Errorf("len(instruments) = %d, want 1 (only ACTIVE)", len(instruments))
	}
	if instruments[0].ID != "WHT-HRW-2026M07-UB" {
		t.Errorf("instruments[0].ID = %q, want %q", instruments[0].ID, "WHT-HRW-2026M07-UB")
	}
}

func TestListInstruments_Empty(t *testing.T) {
	h := NewHandlers(&mockStore{})
	req := httptest.NewRequest("GET", "/api/v1/instruments", nil)
	rec := httptest.NewRecorder()

	h.ListInstruments(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var instruments []Instrument
	json.Unmarshal(resp["data"], &instruments)
	if len(instruments) != 0 {
		t.Errorf("expected empty list, got %d items", len(instruments))
	}
}

func TestListInstruments_StoreError(t *testing.T) {
	h := NewHandlers(&mockStore{err: errors.New("db failure")})
	req := httptest.NewRequest("GET", "/api/v1/instruments", nil)
	rec := httptest.NewRecorder()

	h.ListInstruments(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestGetInstrument_Success(t *testing.T) {
	h := NewHandlers(&mockStore{detail: sampleDetail()})
	// The router sets path params as query params (see router.go)
	req := httptest.NewRequest("GET", "/api/v1/instruments/WHT-HRW-2026M07-UB?id=WHT-HRW-2026M07-UB", nil)
	rec := httptest.NewRecorder()

	h.GetInstrument(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var detail InstrumentDetail
	json.Unmarshal(resp["data"], &detail)
	if detail.ID != "WHT-HRW-2026M07-UB" {
		t.Errorf("detail.ID = %q, want %q", detail.ID, "WHT-HRW-2026M07-UB")
	}
	if detail.Commodity == nil {
		t.Error("expected commodity to be populated")
	} else if detail.Commodity.ID != "WHT-HRW" {
		t.Errorf("commodity.ID = %q, want %q", detail.Commodity.ID, "WHT-HRW")
	}
}

func TestGetInstrument_NotFound(t *testing.T) {
	h := NewHandlers(&mockStore{detail: sampleDetail()})
	req := httptest.NewRequest("GET", "/api/v1/instruments/NONEXISTENT?id=NONEXISTENT", nil)
	rec := httptest.NewRecorder()

	h.GetInstrument(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetInstrument_MissingID(t *testing.T) {
	h := NewHandlers(&mockStore{})
	req := httptest.NewRequest("GET", "/api/v1/instruments/", nil)
	rec := httptest.NewRecorder()

	h.GetInstrument(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetInstrument_StoreError(t *testing.T) {
	h := NewHandlers(&mockStore{err: errors.New("db failure")})
	req := httptest.NewRequest("GET", "/api/v1/instruments/WHT-HRW-2026M07-UB?id=WHT-HRW-2026M07-UB", nil)
	rec := httptest.NewRecorder()

	h.GetInstrument(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestListCommodities_ResponseContentType(t *testing.T) {
	h := NewHandlers(&mockStore{commodities: sampleCommodities()})
	req := httptest.NewRequest("GET", "/api/v1/commodities", nil)
	rec := httptest.NewRecorder()

	h.ListCommodities(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestListInstruments_ResponseFormat(t *testing.T) {
	h := NewHandlers(&mockStore{instruments: sampleInstruments()})
	req := httptest.NewRequest("GET", "/api/v1/instruments?status=ACTIVE", nil)
	rec := httptest.NewRecorder()

	h.ListInstruments(rec, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatal("data is not an array")
	}
	if len(data) != 1 {
		t.Errorf("expected 1 active instrument, got %d", len(data))
	}
	inst := data[0].(map[string]interface{})
	if inst["settlement_type"] != "PHYSICAL" {
		t.Errorf("settlement_type = %v, want PHYSICAL", inst["settlement_type"])
	}
	if inst["currency"] != "MNT" {
		t.Errorf("currency = %v, want MNT", inst["currency"])
	}
}

func TestGetInstrument_CommodityFields(t *testing.T) {
	h := NewHandlers(&mockStore{detail: sampleDetail()})
	req := httptest.NewRequest("GET", "/api/v1/instruments/WHT-HRW-2026M07-UB?id=WHT-HRW-2026M07-UB", nil)
	rec := httptest.NewRecorder()

	h.GetInstrument(rec, req)

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var detail map[string]interface{}
	json.Unmarshal(resp["data"], &detail)

	commodity, ok := detail["commodity"].(map[string]interface{})
	if !ok {
		t.Fatal("commodity field missing or not an object")
	}
	if commodity["category"] != "grain" {
		t.Errorf("commodity.category = %v, want grain", commodity["category"])
	}
	if commodity["unit"] != "bushel" {
		t.Errorf("commodity.unit = %v, want bushel", commodity["unit"])
	}
}

func TestListInstruments_FilterSuspended(t *testing.T) {
	h := NewHandlers(&mockStore{instruments: sampleInstruments()})
	req := httptest.NewRequest("GET", "/api/v1/instruments?status=SUSPENDED", nil)
	rec := httptest.NewRecorder()

	h.ListInstruments(rec, req)

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var instruments []Instrument
	json.Unmarshal(resp["data"], &instruments)
	if len(instruments) != 1 {
		t.Errorf("len(instruments) = %d, want 1 (only SUSPENDED)", len(instruments))
	}
	if len(instruments) > 0 && instruments[0].Status != "SUSPENDED" {
		t.Errorf("instruments[0].Status = %q, want SUSPENDED", instruments[0].Status)
	}
}

func TestListInstruments_NoMatchingStatus(t *testing.T) {
	h := NewHandlers(&mockStore{instruments: sampleInstruments()})
	req := httptest.NewRequest("GET", "/api/v1/instruments?status=EXPIRED", nil)
	rec := httptest.NewRecorder()

	h.ListInstruments(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	var instruments []Instrument
	json.Unmarshal(resp["data"], &instruments)
	if len(instruments) != 0 {
		t.Errorf("expected empty list for EXPIRED filter, got %d", len(instruments))
	}
}

func TestWriteError_Format(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusNotFound, "NOT_FOUND", "test not found")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var resp map[string]map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"]["code"] != "NOT_FOUND" {
		t.Errorf("error.code = %q, want NOT_FOUND", resp["error"]["code"])
	}
	if resp["error"]["message"] != "test not found" {
		t.Errorf("error.message = %q, want %q", resp["error"]["message"], "test not found")
	}
}

// --- Admin handler tests ---

func adminClaims() *auth.Claims {
	return &auth.Claims{
		Sub:           "admin-user",
		ParticipantID: "P-ADMIN",
		Roles:         []string{"exchange_admin"},
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
	}
}

func withAdminContext(r *http.Request) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ClaimsContextKey, adminClaims())
	return r.WithContext(ctx)
}

func TestCreateInstrument_Success(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"commodity_id":"WHT-HRW","name":"Wheat Jul 2026","delivery_month":7,"delivery_year":2026,"contract_size":"50","tick_size":"0.25","currency":"MNT","settlement_type":"physical"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/instruments", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.CreateInstrument(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if _, ok := resp["data"]; !ok {
		t.Error("response missing 'data' field")
	}
}

func TestCreateInstrument_MissingFields(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"commodity_id":"WHT-HRW"}` // missing name, currency etc.
	req := httptest.NewRequest("POST", "/api/v1/admin/instruments", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.CreateInstrument(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateInstrument_NoAuth(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"commodity_id":"WHT-HRW","name":"Wheat Jul","contract_size":"50","tick_size":"0.25","currency":"MNT"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/instruments", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	h.CreateInstrument(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCreateInstrument_StoreError(t *testing.T) {
	h := NewHandlers(&mockStore{err: errors.New("db down")})
	body := `{"commodity_id":"WHT-HRW","name":"Wheat Jul","contract_size":"50","tick_size":"0.25","currency":"MNT","settlement_type":"physical"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/instruments", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.CreateInstrument(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCreateInstrument_AutoID(t *testing.T) {
	h := NewHandlers(&mockStore{})
	// No id field → should auto-generate
	body := `{"commodity_id":"WHT-HRW","name":"Wheat Jul","delivery_month":7,"delivery_year":2026,"contract_size":"50","tick_size":"0.25","currency":"MNT","settlement_type":"physical"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/instruments", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.CreateInstrument(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Data.ID == "" {
		t.Error("expected auto-generated id, got empty string")
	}
}

func TestUpdateInstrument_Success(t *testing.T) {
	instruments := sampleInstruments()
	h := NewHandlers(&mockStore{instruments: instruments})
	body := `{"status":"suspended"}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/instruments/WHT-HRW-2026M07-UB?id=WHT-HRW-2026M07-UB", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.UpdateInstrument(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestUpdateInstrument_NoAuth(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"status":"suspended"}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/instruments/INST-1?id=INST-1", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	h.UpdateInstrument(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestUpdateInstrument_MissingID(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"status":"suspended"}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/instruments/", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.UpdateInstrument(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateInstrument_EmptyBody(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/instruments/INST-1?id=INST-1", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.UpdateInstrument(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateCommodity_Success(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"name":"Rice","category":"grain","unit":"MT"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/commodities", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.CreateCommodity(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if _, ok := resp["data"]; !ok {
		t.Error("response missing 'data' field")
	}
}

func TestCreateCommodity_MissingFields(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"name":"Rice"}` // missing category, unit
	req := httptest.NewRequest("POST", "/api/v1/admin/commodities", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.CreateCommodity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateCommodity_NoAuth(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"name":"Rice","category":"grain","unit":"MT"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/commodities", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	h.CreateCommodity(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCreateCommodity_AutoID(t *testing.T) {
	h := NewHandlers(&mockStore{})
	body := `{"name":"Cotton","category":"fiber","unit":"bale"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/commodities", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.CreateCommodity(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Data.ID == "" {
		t.Error("expected auto-generated id, got empty")
	}
}

func TestCreateCommodity_StoreError(t *testing.T) {
	h := NewHandlers(&mockStore{err: errors.New("db down")})
	body := `{"name":"Cotton","category":"fiber","unit":"bale"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/commodities", bytes.NewBufferString(body))
	req = withAdminContext(req)
	rec := httptest.NewRecorder()

	h.CreateCommodity(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
