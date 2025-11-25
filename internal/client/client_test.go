package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewClient(t *testing.T) {
	baseURL := "https://api.example.com"
	apiKey := "test-key"
	tenantID := "test-tenant"

	client := NewClient(baseURL, apiKey, tenantID)

	if client.BaseURL != baseURL {
		t.Errorf("NewClient() BaseURL = %v, want %v", client.BaseURL, baseURL)
	}
	if client.APIKey != apiKey {
		t.Errorf("NewClient() APIKey = %v, want %v", client.APIKey, apiKey)
	}
	if client.TenantID != tenantID {
		t.Errorf("NewClient() TenantID = %v, want %v", client.TenantID, tenantID)
	}
	if client.HTTPClient == nil {
		t.Error("NewClient() HTTPClient is nil")
	}
	if client.HTTPClient.Timeout != 30*time.Second {
		t.Errorf("NewClient() HTTPClient.Timeout = %v, want %v", client.HTTPClient.Timeout, 30*time.Second)
	}
}

func TestClient_DoRequest_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %v, want application/json", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("X-API-Key = %v, want test-key", r.Header.Get("X-API-Key"))
		}

		// Send success response
		response := APIResponse{
			Success: true,
			Data:    json.RawMessage(`{"id":"123","name":"test"}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	var result map[string]interface{}
	err := client.DoRequest("GET", "/test", nil, &result)

	if err != nil {
		t.Errorf("DoRequest() error = %v", err)
	}
	if result["id"] != "123" {
		t.Errorf("DoRequest() result id = %v, want 123", result["id"])
	}
}

func TestClient_DoRequest_WithBody(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request body
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}
		if body["name"] != "test" {
			t.Errorf("Request body name = %v, want test", body["name"])
		}

		// Send response
		response := APIResponse{
			Success: true,
			Data:    json.RawMessage(`{"created":true}`),
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	requestBody := map[string]interface{}{"name": "test"}
	var result map[string]interface{}
	err := client.DoRequest("POST", "/test", requestBody, &result)

	if err != nil {
		t.Errorf("DoRequest() error = %v", err)
	}
}

func TestClient_DoRequest_APIError(t *testing.T) {
	// Create test server that returns API error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		response := APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "INVALID_REQUEST",
				Message: "Invalid request parameters",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	var result map[string]interface{}
	err := client.DoRequest("GET", "/test", nil, &result)

	if err == nil {
		t.Error("DoRequest() expected error, got nil")
	}
	if err != nil && err.Error() != "API error: INVALID_REQUEST - Invalid request parameters" {
		t.Errorf("DoRequest() error = %v, want API error", err)
	}
}

func TestClient_DoRequest_HTTPError(t *testing.T) {
	// Create test server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	var result map[string]interface{}
	err := client.DoRequest("GET", "/test", nil, &result)

	if err == nil {
		t.Error("DoRequest() expected error, got nil")
	}
}

func TestClient_DoRequest_InvalidJSON(t *testing.T) {
	// Create test server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	var result map[string]interface{}
	err := client.DoRequest("GET", "/test", nil, &result)

	if err == nil {
		t.Error("DoRequest() expected error for invalid JSON, got nil")
	}
}

func TestClient_DoRequest_NoResult(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	err := client.DoRequest("DELETE", "/test", nil, nil)

	if err != nil {
		t.Errorf("DoRequest() error = %v, want nil for successful request without result", err)
	}
}

func TestClient_DoRequest_FailedSuccess(t *testing.T) {
	// Create test server that returns success=false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "FAILED",
				Message: "Operation failed",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	var result map[string]interface{}
	err := client.DoRequest("GET", "/test", nil, &result)

	if err == nil {
		t.Error("DoRequest() expected error for success=false, got nil")
	}
}

func TestClient_DoRequest_FailedSuccessNoError(t *testing.T) {
	// Create test server that returns success=false without error details
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := APIResponse{
			Success: false,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	var result map[string]interface{}
	err := client.DoRequest("GET", "/test", nil, &result)

	if err == nil {
		t.Error("DoRequest() expected error, got nil")
	}
	if err.Error() != "API request failed" {
		t.Errorf("DoRequest() error = %v, want 'API request failed'", err)
	}
}

func TestClient_ListLedgers_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tenants/test-tenant/deployments" {
			t.Errorf("Request path = %v, want /api/tenants/test-tenant/deployments", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Request method = %v, want GET", r.Method)
		}

		ledgers := []Ledger{
			{ID: "123", Name: "ledger1", Region: "us-east-1", Status: "active"},
			{ID: "456", Name: "ledger2", Region: "eu-west-1", Status: "pending"},
		}
		response := APIResponse{
			Success: true,
			Data:    mustMarshal(ledgers),
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	ledgers, err := client.ListLedgers()

	if err != nil {
		t.Errorf("ListLedgers() error = %v", err)
	}
	if len(ledgers) != 2 {
		t.Errorf("ListLedgers() returned %d ledgers, want 2", len(ledgers))
	}
	if ledgers[0].ID != "123" {
		t.Errorf("ListLedgers() first ledger ID = %v, want 123", ledgers[0].ID)
	}
}

func TestClient_ListLedgers_Error(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		response := APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "UNAUTHORIZED",
				Message: "Invalid API key",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	ledgers, err := client.ListLedgers()

	if err == nil {
		t.Error("ListLedgers() expected error, got nil")
	}
	if ledgers != nil {
		t.Errorf("ListLedgers() ledgers = %v, want nil on error", ledgers)
	}
}

func TestClient_GetLedger_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/deployments/123" {
			t.Errorf("Request path = %v, want /api/deployments/123", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Request method = %v, want GET", r.Method)
		}

		ledger := Ledger{
			ID:     "123",
			Name:   "test-ledger",
			Region: "us-east-1",
			Status: "active",
		}
		response := APIResponse{
			Success: true,
			Data:    mustMarshal(ledger),
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	ledger, err := client.GetLedger("123")

	if err != nil {
		t.Errorf("GetLedger() error = %v", err)
	}
	if ledger == nil {
		t.Fatal("GetLedger() returned nil ledger")
	}
	if ledger.ID != "123" {
		t.Errorf("GetLedger() ID = %v, want 123", ledger.ID)
	}
	if ledger.Name != "test-ledger" {
		t.Errorf("GetLedger() Name = %v, want test-ledger", ledger.Name)
	}
}

func TestClient_GetLedger_NotFound(t *testing.T) {
	// Create test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		response := APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "NOT_FOUND",
				Message: "Ledger not found",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	ledger, err := client.GetLedger("nonexistent")

	if err == nil {
		t.Error("GetLedger() expected error, got nil")
	}
	if ledger != nil {
		t.Errorf("GetLedger() ledger = %v, want nil on error", ledger)
	}
}

// Helper function to marshal data
func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}

// Tests for deployment.go

func TestClient_CreateDeployment_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tenants/test-tenant/deployments" {
			t.Errorf("Request path = %v, want /api/tenants/test-tenant/deployments", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Request method = %v, want POST", r.Method)
		}

		// Parse request body
		var req CreateDeploymentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		// Create response
		deployment := Deployment{
			ID:          mustParseUUID("123e4567-e89b-12d3-a456-426614174000"),
			TenantID:    mustParseUUID("223e4567-e89b-12d3-a456-426614174000"),
			Name:        req.Name,
			Type:        req.Type,
			Region:      req.Region,
			Status:      DeploymentStatusProvisioning,
			Environment: req.Environment,
			MultiAZ:     req.MultiAZ,
		}
		response := APIResponse{
			Success: true,
			Data:    mustMarshal(deployment),
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	testSize := SizeTest
	req := &CreateDeploymentRequest{
		Name:        "test-ledger",
		Type:        DeploymentTypeSaaS,
		Region:      "us-east-1",
		Size:        &testSize,
		Environment: EnvironmentDev,
		MultiAZ:     false,
		Sandbox:     false,
	}

	deployment, err := client.CreateDeployment(req)

	if err != nil {
		t.Errorf("CreateDeployment() error = %v", err)
	}
	if deployment == nil {
		t.Fatal("CreateDeployment() returned nil deployment")
	}
	if deployment.Name != "test-ledger" {
		t.Errorf("CreateDeployment() Name = %v, want test-ledger", deployment.Name)
	}
	if deployment.Status != DeploymentStatusProvisioning {
		t.Errorf("CreateDeployment() Status = %v, want %v", deployment.Status, DeploymentStatusProvisioning)
	}
}

func TestClient_CreateDeployment_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		response := APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "INVALID_REQUEST",
				Message: "Invalid deployment configuration",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	req := &CreateDeploymentRequest{
		Name:        "test-ledger",
		Type:        DeploymentTypeSaaS,
		Region:      "invalid-region",
		Environment: EnvironmentDev,
	}

	deployment, err := client.CreateDeployment(req)

	if err == nil {
		t.Error("CreateDeployment() expected error, got nil")
	}
	if deployment != nil {
		t.Errorf("CreateDeployment() deployment = %v, want nil on error", deployment)
	}
}

func TestClient_GetDeploymentByID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/deployments/123e4567-e89b-12d3-a456-426614174000" {
			t.Errorf("Request path = %v, want /api/deployments/123e4567-e89b-12d3-a456-426614174000", r.URL.Path)
		}

		deployment := Deployment{
			ID:          mustParseUUID("123e4567-e89b-12d3-a456-426614174000"),
			TenantID:    mustParseUUID("223e4567-e89b-12d3-a456-426614174000"),
			Name:        "test-ledger",
			Type:        DeploymentTypeSaaS,
			Region:      "us-east-1",
			Status:      DeploymentStatusAvailable,
			Environment: EnvironmentDev,
		}
		response := APIResponse{
			Success: true,
			Data:    mustMarshal(deployment),
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	deployment, err := client.GetDeploymentByID("123e4567-e89b-12d3-a456-426614174000")

	if err != nil {
		t.Errorf("GetDeploymentByID() error = %v", err)
	}
	if deployment == nil {
		t.Fatal("GetDeploymentByID() returned nil deployment")
	}
	if deployment.Name != "test-ledger" {
		t.Errorf("GetDeploymentByID() Name = %v, want test-ledger", deployment.Name)
	}
}

func TestClient_DeleteDeployment_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/deployments/123e4567-e89b-12d3-a456-426614174000" {
			t.Errorf("Request path = %v", r.URL.Path)
		}
		if r.Method != "DELETE" {
			t.Errorf("Request method = %v, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	err := client.DeleteDeployment("123e4567-e89b-12d3-a456-426614174000")

	if err != nil {
		t.Errorf("DeleteDeployment() error = %v", err)
	}
}

func TestClient_DeleteDeployment_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		response := APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "NOT_FOUND",
				Message: "Deployment not found",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	err := client.DeleteDeployment("nonexistent")

	if err == nil {
		t.Error("DeleteDeployment() expected error, got nil")
	}
}

// Tests for version.go

func TestClient_ListVersions_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ledger-versions" {
			t.Errorf("Request path = %v, want /api/ledger-versions", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Request method = %v, want GET", r.Method)
		}

		versions := []*LedgerVersion{
			{
				ID:           "1",
				AppVersion:   "v1.0.0",
				ChartVersion: "1.0.0",
				IsLatest:     true,
				SupportTier:  "stable",
			},
			{
				ID:           "2",
				AppVersion:   "v0.9.0",
				ChartVersion: "0.9.0",
				IsLatest:     false,
				SupportTier:  "stable",
			},
		}
		response := APIResponse{
			Success: true,
			Data:    mustMarshal(versions),
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	versions, err := client.ListVersions()

	if err != nil {
		t.Errorf("ListVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("ListVersions() returned %d versions, want 2", len(versions))
	}
	if versions[0].AppVersion != "v1.0.0" {
		t.Errorf("ListVersions() first version AppVersion = %v, want v1.0.0", versions[0].AppVersion)
	}
	if !versions[0].IsLatest {
		t.Error("ListVersions() first version should be latest")
	}
}

func TestClient_ListVersions_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		response := APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to fetch versions",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-tenant")

	versions, err := client.ListVersions()

	if err == nil {
		t.Error("ListVersions() expected error, got nil")
	}
	if versions != nil {
		t.Errorf("ListVersions() versions = %v, want nil on error", versions)
	}
}

// Helper to parse UUID
func mustParseUUID(s string) (u uuid.UUID) {
	u, _ = uuid.Parse(s)
	return
}
