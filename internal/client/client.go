package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents an HTTP client for the Lerian API
type Client struct {
	BaseURL    string
	APIKey     string
	TenantID   string
	HTTPClient *http.Client
}

// NewClient creates a new Lerian API client
func NewClient(baseURL, apiKey, tenantID string) *Client {
	return &Client{
		BaseURL:  baseURL,
		APIKey:   apiKey,
		TenantID: tenantID,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Ledger represents a Midaz ledger deployment
type Ledger struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Region            string    `json:"region"`
	Status            string    `json:"status"`
	HelmReleaseName   string    `json:"helm_release_name,omitempty"`
	HelmChart         string    `json:"helm_chart,omitempty"`
	HelmNamespace     string    `json:"helm_namespace,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// APIResponse represents a generic API response
type APIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *APIError       `json:"error,omitempty"`
}

// APIError represents an API error
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DoRequest performs an HTTP request to the Lerian API
func (c *Client) DoRequest(method, path string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiResp APIResponse
		if err := json.Unmarshal(responseBody, &apiResp); err == nil && apiResp.Error != nil {
			return fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
		}
		return fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, string(responseBody))
	}

	if result != nil {
		var apiResp APIResponse
		if err := json.Unmarshal(responseBody, &apiResp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if !apiResp.Success {
			if apiResp.Error != nil {
				return fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
			}
			return fmt.Errorf("API request failed")
		}

		if err := json.Unmarshal(apiResp.Data, result); err != nil {
			return fmt.Errorf("failed to parse response data: %w", err)
		}
	}

	return nil
}

// ListLedgers retrieves all ledgers for the tenant
func (c *Client) ListLedgers() ([]Ledger, error) {
	var ledgers []Ledger
	path := fmt.Sprintf("/api/tenants/%s/deployments", c.TenantID)
	if err := c.DoRequest("GET", path, nil, &ledgers); err != nil {
		return nil, err
	}
	return ledgers, nil
}

// GetLedger retrieves a specific ledger by ID
func (c *Client) GetLedger(id string) (*Ledger, error) {
	var ledger Ledger
	path := fmt.Sprintf("/api/deployments/%s", id)
	if err := c.DoRequest("GET", path, nil, &ledger); err != nil {
		return nil, err
	}
	return &ledger, nil
}
