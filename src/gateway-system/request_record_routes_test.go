package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestRequestRecordRoutes(t *testing.T) {
	fakes := newGatewayFakes()
	system := newTestGateway(t, fakes)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	fakes.requestRecords.config = types.RequestRecordConfig{Enabled: true, Limit: 50, UpdatedAt: now}
	fakes.requestRecords.records = []types.RequestRecord{{
		ID:              "request-record-1",
		CreatedAt:       now,
		Method:          "POST",
		URL:             "https://api.example.com/v1/chat/completions",
		Headers:         map[string]string{"Authorization": "[REDACTED]", "Content-Type": "application/json"},
		Body:            `{"model":"deepseek-v4.1-flash"}`,
		ResponseStatus:  200,
		ResponseHeaders: map[string][]string{"Content-Type": {"text/event-stream"}},
		ResponseBody:    "data: {\"ok\":true}",
		DurationMs:      120,
	}}

	rec := httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/request-records/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("load config status = %d body=%s", rec.Code, rec.Body.String())
	}
	var configPayload struct {
		Data types.RequestRecordConfig `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &configPayload); err != nil {
		t.Fatalf("decode config response error = %v", err)
	}
	if !configPayload.Data.Enabled || configPayload.Data.Limit != 50 {
		t.Fatalf("config = %#v", configPayload.Data)
	}

	rec = httptest.NewRecorder()
	body := strings.NewReader(`{"enabled":false,"limit":20}`)
	system.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/request-records/config", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("save config status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fakes.requestRecords.config.Enabled || fakes.requestRecords.config.Limit != 20 {
		t.Fatalf("saved config = %#v", fakes.requestRecords.config)
	}

	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/request-records", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var listPayload struct {
		Data []types.RequestRecordSummary `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list response error = %v", err)
	}
	if len(listPayload.Data) != 1 || listPayload.Data[0].ID != "request-record-1" || listPayload.Data[0].Status != 200 {
		t.Fatalf("list = %#v", listPayload.Data)
	}

	rec = httptest.NewRecorder()
	system.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/request-records/request-record-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("load record status = %d body=%s", rec.Code, rec.Body.String())
	}
	var recordPayload struct {
		Data types.RequestRecord `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &recordPayload); err != nil {
		t.Fatalf("decode record response error = %v", err)
	}
	if recordPayload.Data.ID != "request-record-1" || recordPayload.Data.Headers["Authorization"] != "[REDACTED]" || recordPayload.Data.ResponseBody != "data: {\"ok\":true}" {
		t.Fatalf("record = %#v", recordPayload.Data)
	}
}
