# Lerian CLI - Implementation Plan

## Status: Foundation Created ✅

**Completed:**
- ✅ Directory structure created
- ✅ Go module initialized (`github.com/lerian-studio/lerian-cli`)
- ✅ main.go created

## Phase 1: Minimal Working Version (Priority)

### Files to Create (in order):

#### 1. Root Command (`cmd/root.go`)
```go
package cmd

import (
	"fmt"
	"os"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "lerian",
	Short: "Lerian CLI - Manage your Lerian services",
	Long:  `Command-line interface for managing Lerian services including Midaz, Finflow, and Finbase.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringP("profile", "p", "default", "Configuration profile to use")
	rootCmd.PersistentFlags().StringP("output", "o", "table", "Output format (table|json|yaml)")
}
```

#### 2. Config Management (`internal/config/config.go`)
```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"gopkg.in/yaml.v3"
)

type Profile struct {
	APIURL   string `yaml:"api-url"`
	APIKey   string `yaml:"api-key"`
	TenantID string `yaml:"tenant-id"`
}

type Config struct {
	CurrentProfile string             `yaml:"current-profile"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

func GetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lerian", "config.yaml")
}

func LoadConfig() (*Config, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				CurrentProfile: "default",
				Profiles:       make(map[string]Profile),
			}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path := GetConfigPath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func GetProfile(profileName string) (*Profile, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	if profileName == "" {
		profileName = cfg.CurrentProfile
	}

	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile '%s' not found. Run 'lerian auth login' first", profileName)
	}

	return &profile, nil
}
```

#### 3. HTTP Client (`internal/client/client.go`)
```go
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL    string
	APIKey     string
	TenantID   string
	HTTPClient *http.Client
}

func New(baseURL, apiKey, tenantID string) *Client {
	return &Client{
		BaseURL:  baseURL,
		APIKey:   apiKey,
		TenantID: tenantID,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) DoRequest(method, path string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.APIKey)
	if c.TenantID != "" {
		req.Header.Set("X-Tenant-ID", c.TenantID)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Ledger API methods
type Ledger struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Region          string `json:"region"`
	Status          string `json:"status"`
	HelmReleaseName string `json:"helm_release_name"`
	HelmChart       string `json:"helm_chart"`
	HelmNamespace   string `json:"helm_namespace"`
	CreatedAt       string `json:"created_at"`
}

func (c *Client) ListLedgers() ([]Ledger, error) {
	var response struct {
		Data []Ledger `json:"data"`
	}

	err := c.DoRequest("GET", "/api/deployments", nil, &response)
	if err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) GetLedger(id string) (*Ledger, error) {
	var response struct {
		Data Ledger `json:"data"`
	}

	err := c.DoRequest("GET", fmt.Sprintf("/api/deployments/%s", id), nil, &response)
	if err != nil {
		return nil, err
	}

	return &response.Data, nil
}
```

#### 4. Output Formatting (`internal/output/table.go`)
```go
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

func PrintTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	// Print headers
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Print separator
	separators := make([]string, len(headers))
	for i := range headers {
		separators[i] = strings.Repeat("-", len(headers[i]))
	}
	fmt.Fprintln(w, strings.Join(separators, "\t"))

	// Print rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	w.Flush()
}

func PrintJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
```

#### 5. Auth Login Command (`cmd/auth/login.go`)
```go
package auth

import (
	"fmt"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/spf13/cobra"
)

var LoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Lerian API",
	Long:  `Store API credentials for authenticating with the Lerian Control Plane.`,
	RunE:  runLogin,
}

var (
	apiURL   string
	apiKey   string
	tenantID string
	profile  string
)

func init() {
	LoginCmd.Flags().StringVar(&apiURL, "api-url", "http://localhost:8080", "Control Plane API URL")
	LoginCmd.Flags().StringVar(&apiKey, "api-key", "", "API Key")
	LoginCmd.Flags().StringVar(&tenantID, "tenant-id", "", "Tenant ID")
	LoginCmd.Flags().StringVar(&profile, "profile", "default", "Profile name")

	LoginCmd.MarkFlagRequired("api-key")
	LoginCmd.MarkFlagRequired("tenant-id")
}

func runLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Update profile
	cfg.Profiles[profile] = config.Profile{
		APIURL:   apiURL,
		APIKey:   apiKey,
		TenantID: tenantID,
	}
	cfg.CurrentProfile = profile

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Successfully authenticated as profile '%s'\n", profile)
	fmt.Printf("  API URL: %s\n", apiURL)
	fmt.Printf("  Tenant ID: %s\n", tenantID)
	fmt.Printf("\nConfig saved to: %s\n", config.GetConfigPath())

	return nil
}
```

#### 6. Auth Command Root (`cmd/auth/auth.go`)
```go
package auth

import "github.com/spf13/cobra"

var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication commands",
	Long:  `Manage authentication with the Lerian Control Plane.`,
}

func init() {
	AuthCmd.AddCommand(LoginCmd)
}
```

#### 7. Midaz Root Command (`cmd/midaz/midaz.go`)
```go
package midaz

import "github.com/spf13/cobra"

var MidazCmd = &cobra.Command{
	Use:   "midaz",
	Short: "Midaz ledger management",
	Long:  `Manage Midaz ledgers, accounts, and transactions.`,
}

func init() {
	// Subcommands will be added here
}
```

#### 8. Ledger List Command (`cmd/midaz/ledger/list.go`)
```go
package ledger

import (
	"fmt"
	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/output"
	"github.com/spf13/cobra"
)

var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all ledgers",
	Long:  `List all Midaz ledgers in your account.`,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	// Get profile
	profileName, _ := cmd.Flags().GetString("profile")
	profile, err := config.GetProfile(profileName)
	if err != nil {
		return err
	}

	// Create client
	c := client.New(profile.APIURL, profile.APIKey, profile.TenantID)

	// List ledgers
	ledgers, err := c.ListLedgers()
	if err != nil {
		return fmt.Errorf("failed to list ledgers: %w", err)
	}

	if len(ledgers) == 0 {
		fmt.Println("No ledgers found")
		return nil
	}

	// Get output format
	outputFormat, _ := cmd.Flags().GetString("output")

	if outputFormat == "json" {
		return output.PrintJSON(ledgers)
	}

	// Print as table
	headers := []string{"ID", "NAME", "REGION", "STATUS", "CHART", "CREATED"}
	rows := make([][]string, len(ledgers))

	for i, l := range ledgers {
		rows[i] = []string{
			l.ID,
			l.Name,
			l.Region,
			l.Status,
			l.HelmChart,
			l.CreatedAt,
		}
	}

	output.PrintTable(headers, rows)

	return nil
}
```

#### 9. Ledger Describe Command (`cmd/midaz/ledger/describe.go`)
```go
package ledger

import (
	"fmt"
	"github.com/lerian-studio/lerian-cli/internal/client"
	"github.com/lerian-studio/lerian-cli/internal/config"
	"github.com/lerian-studio/lerian-cli/internal/output"
	"github.com/spf13/cobra"
)

var DescribeCmd = &cobra.Command{
	Use:   "describe <ledger-id>",
	Short: "Describe a ledger",
	Long:  `Show detailed information about a specific Midaz ledger.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDescribe,
}

func runDescribe(cmd *cobra.Command, args []string) error {
	ledgerID := args[0]

	// Get profile
	profileName, _ := cmd.Flags().GetString("profile")
	profile, err := config.GetProfile(profileName)
	if err != nil {
		return err
	}

	// Create client
	c := client.New(profile.APIURL, profile.APIKey, profile.TenantID)

	// Get ledger
	ledger, err := c.GetLedger(ledgerID)
	if err != nil {
		return fmt.Errorf("failed to get ledger: %w", err)
	}

	// Get output format
	outputFormat, _ := cmd.Flags().GetString("output")

	if outputFormat == "json" {
		return output.PrintJSON(ledger)
	}

	// Print as formatted text
	fmt.Printf("ID:                %s\n", ledger.ID)
	fmt.Printf("Name:              %s\n", ledger.Name)
	fmt.Printf("Region:            %s\n", ledger.Region)
	fmt.Printf("Status:            %s\n", ledger.Status)
	fmt.Printf("Helm Release:      %s\n", ledger.HelmReleaseName)
	fmt.Printf("Helm Chart:        %s\n", ledger.HelmChart)
	fmt.Printf("Helm Namespace:    %s\n", ledger.HelmNamespace)
	fmt.Printf("Created:           %s\n", ledger.CreatedAt)

	return nil
}
```

#### 10. Ledger Command Root (`cmd/midaz/ledger/ledger.go`)
```go
package ledger

import "github.com/spf13/cobra"

var LedgerCmd = &cobra.Command{
	Use:   "ledger",
	Short: "Ledger management",
	Long:  `Manage Midaz ledgers.`,
}

func init() {
	LedgerCmd.AddCommand(ListCmd)
	LedgerCmd.AddCommand(DescribeCmd)
}
```

#### 11. Update Root Command (`cmd/root.go` - add subcommands)
```go
// Add to init() function:
func init() {
	// Import subcommands
	rootCmd.AddCommand(auth.AuthCmd)
	rootCmd.AddCommand(midaz.MidazCmd)

	// Add midaz subcommands
	midaz.MidazCmd.AddCommand(ledger.LedgerCmd)

	// Global flags
	rootCmd.PersistentFlags().StringP("profile", "p", "default", "Configuration profile to use")
	rootCmd.PersistentFlags().StringP("output", "o", "table", "Output format (table|json|yaml)")
}
```

#### 12. Makefile
```makefile
.PHONY: build install clean test

# Build binary
build:
	go build -o bin/lerian .

# Install to local bin
install: build
	cp bin/lerian /usr/local/bin/lerian

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test ./...

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run locally
run:
	go run main.go

# Build for multiple platforms
build-all:
	GOOS=darwin GOARCH=amd64 go build -o bin/lerian-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o bin/lerian-darwin-arm64 .
	GOOS=linux GOARCH=amd64 go build -o bin/lerian-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build -o bin/lerian-windows-amd64.exe .
```

## Installation & Usage

### Install Dependencies
```bash
cd lerian-cli
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
go get gopkg.in/yaml.v3
go mod tidy
```

### Build
```bash
make build
# OR
go build -o bin/lerian .
```

### Usage Examples
```bash
# Login
./bin/lerian auth login \
  --api-url http://localhost:8080 \
  --api-key 203daaeed49fee7220c62adc19e24e0a24102784559160393fc7f1c4008bf29d \
  --tenant-id 3d818736-0574-44ca-8bed-193039bbdaf9

# List ledgers
./bin/lerian midaz ledger list

# List ledgers as JSON
./bin/lerian midaz ledger list --output json

# Describe specific ledger
./bin/lerian midaz ledger describe <ledger-id>
```

## Next Steps

After this minimal version is working:

1. Add `lerian midaz ledger create` command
2. Add `lerian midaz ledger delete` command
3. Add `lerian midaz account` commands
4. Add `lerian midaz transaction` commands
5. Add `lerian midaz ledger backup/restore` commands
6. Add progress indicators with spinner library
7. Add colored output with fatih/color library
8. Add interactive mode with AlecAivazis/survey library
9. Add shell completion (bash/zsh/fish)
10. Set up CI/CD for binary releases

## Dependencies

Required Go packages:
```
github.com/spf13/cobra v1.8.0
gopkg.in/yaml.v3 v3.0.1
```

Optional (for future):
```
github.com/fatih/color v1.16.0              # Colored output
github.com/briandowns/spinner v1.23.0        # Loading spinners
github.com/AlecAivazis/survey/v2 v2.3.7     # Interactive prompts
```
