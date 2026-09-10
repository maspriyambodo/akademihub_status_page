package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInitComponents(t *testing.T) {
	comps := initComponents()
	if len(comps) == 0 {
		t.Fatal("expected at least 1 component")
	}

	foundExam := false
	for _, c := range comps {
		if c.ID == "exam-engine" {
			foundExam = true
		}
		if len(c.DailyHistory) != 90 {
			t.Fatalf("component %s expected 90 day history, got %d", c.ID, len(c.DailyHistory))
		}
	}

	if !foundExam {
		t.Fatal("expected exam-engine component to be present")
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status 'ok', got '%s'", resp["status"])
	}
}

func TestAPIStatusEndpoint(t *testing.T) {
	statusStore.Components = initComponents()
	statusStore.OverallStatus = "operational"
	statusStore.LastUpdated = time.Now()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()

	handleAPIStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var res SystemStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if res.OverallStatus != "operational" {
		t.Fatalf("expected operational, got %s", res.OverallStatus)
	}
}

func TestSummaryJSONEndpoint(t *testing.T) {
	statusStore.Components = initComponents()
	statusStore.OverallStatus = "operational"
	statusStore.LastUpdated = time.Now()

	req := httptest.NewRequest(http.MethodGet, "/api/summary.json", nil)
	rec := httptest.NewRecorder()

	handleSummaryJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var parsed struct {
		Page struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"page"`
		Components []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"components"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid summary.json output: %v", err)
	}
	if parsed.Page.URL != "https://status.akademihub.id" {
		t.Fatalf("expected https://status.akademihub.id, got %s", parsed.Page.URL)
	}
	if len(parsed.Components) == 0 {
		t.Fatal("expected components in summary.json")
	}
}

func TestIncidentsEndpoint(t *testing.T) {
	statusStore.Incidents = []Incident{}
	adminKey = "test-secret"

	// Test unauthorized POST
	badReq := httptest.NewRequest(http.MethodPost, "/api/incidents", bytes.NewBufferString(`{"title":"Test"}`))
	badRec := httptest.NewRecorder()
	handleIncidents(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", badRec.Code)
	}

	// Test authorized POST
	body := `{"title":"Uji Coba Insiden","status":"investigating","impact":"minor"}`
	goodReq := httptest.NewRequest(http.MethodPost, "/api/incidents", bytes.NewBufferString(body))
	goodReq.Header.Set("Authorization", "Bearer test-secret")
	goodRec := httptest.NewRecorder()
	handleIncidents(goodRec, goodReq)
	if goodRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d", goodRec.Code)
	}

	// Test GET incidents
	getReq := httptest.NewRequest(http.MethodGet, "/api/incidents", nil)
	getRec := httptest.NewRecorder()
	handleIncidents(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", getRec.Code)
	}
	var list []Incident
	if err := json.Unmarshal(getRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode incidents: %v", err)
	}
	if len(list) != 1 || list[0].Title != "Uji Coba Insiden" {
		t.Fatalf("unexpected incidents list: %+v", list)
	}
}