package client

import "time"

// LedgerVersion represents an available ledger version
type LedgerVersion struct {
	ID           string    `json:"id"`
	AppVersion   string    `json:"app_version"`
	ChartVersion string    `json:"chart_version"`
	IsLatest     bool      `json:"is_latest"`
	SupportTier  string    `json:"support_tier"`
	ReleaseDate  time.Time `json:"release_date"`
	Changelog    string    `json:"changelog"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ListVersions retrieves all available ledger versions
func (c *Client) ListVersions() ([]*LedgerVersion, error) {
	var versions []*LedgerVersion
	if err := c.DoRequest("GET", "/api/ledger-versions", nil, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}
