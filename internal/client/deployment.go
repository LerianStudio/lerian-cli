package client

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DeploymentType constants
const (
	DeploymentTypeSaaS         = "saas"
	DeploymentTypeSingleTenant = "single-tenant"
	// FIXME(nitpick): Add DeploymentTypeHybrid = "hybrid" to match Control Plane API
	// Reported by: business-logic-reviewer on 2025-11-17
	// Severity: Low
)

// DeploymentStatus constants
const (
	DeploymentStatusProvisioning = "provisioning"
	DeploymentStatusAvailable    = "available"
	DeploymentStatusFailed       = "failed"
	// FIXME(nitpick): Add DeploymentStatusDeleting and DeploymentStatusDeleted constants
	// Reported by: business-logic-reviewer on 2025-11-17
	// Severity: Low
)

// Environment constants
const (
	EnvironmentDev     = "dev"
	EnvironmentStaging = "staging"
	EnvironmentProd    = "prod"
)

// Size constants
const (
	SizeTest       = "test"
	SizeStaging    = "staging"
	SizeProduction = "production"
)

// CreateDeploymentRequest represents the request to create a deployment
// TODO(review): Add validation method to check deployment type, environment, and size
// Reported by: business-logic-reviewer on 2025-11-17
// Severity: Medium
type CreateDeploymentRequest struct {
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Region        string                 `json:"region"`
	Size          *string                `json:"size,omitempty"`
	TPS           *int                   `json:"tps,omitempty"`
	Environment   string                 `json:"environment"`
	MultiAZ       bool                   `json:"multi_az"`
	Sandbox       bool                   `json:"sandbox"`
	AppVersion    *string                `json:"app_version,omitempty"`
	ChartVersion  *string                `json:"chart_version,omitempty"`
	AgentID       *uuid.UUID             `json:"agent_id,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

// Deployment represents a ledger deployment
type Deployment struct {
	ID              uuid.UUID `json:"id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	Region          string    `json:"region"`
	Size            *string   `json:"size,omitempty"`
	TPS             *int      `json:"tps,omitempty"`
	Status          string    `json:"status"`
	Endpoint        *string   `json:"endpoint,omitempty"`
	Environment     string    `json:"environment"`
	MultiAZ         bool      `json:"multi_az"`
	HelmReleaseName *string   `json:"helm_release_name,omitempty"`
	HelmChart       *string   `json:"helm_chart,omitempty"`
	HelmNamespace   *string   `json:"helm_namespace,omitempty"`
	AppVersion      *string   `json:"app_version,omitempty"`
	ChartVersion    *string   `json:"chart_version,omitempty"`
	ErrorMessage    *string   `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateDeployment creates a new ledger deployment
func (c *Client) CreateDeployment(req *CreateDeploymentRequest) (*Deployment, error) {
	var deployment Deployment
	path := fmt.Sprintf("/api/tenants/%s/deployments", c.TenantID)
	if err := c.DoRequest("POST", path, req, &deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

// GetDeploymentByID retrieves a deployment by ID
// FIXME(nitpick): Add UUID format validation before making API call
// Reported by: security-reviewer on 2025-11-17
// Severity: Low
func (c *Client) GetDeploymentByID(id string) (*Deployment, error) {
	var deployment Deployment
	path := fmt.Sprintf("/api/deployments/%s", id)
	if err := c.DoRequest("GET", path, nil, &deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

// DeleteDeployment deletes a deployment by ID
func (c *Client) DeleteDeployment(id string) error {
	path := fmt.Sprintf("/api/deployments/%s", id)
	return c.DoRequest("DELETE", path, nil, nil)
}
