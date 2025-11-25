package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetConfigPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	want := filepath.Join(home, ".lerian", "config.yaml")
	got, err := GetConfigPath()

	if err != nil {
		t.Fatalf("GetConfigPath() error = %v", err)
	}

	if got != want {
		t.Errorf("GetConfigPath() = %v, want %v", got, want)
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	// Setup: Use temporary directory
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test: Load should return empty config when file doesn't exist
	config, err := Load()

	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if config == nil {
		t.Fatal("Load() returned nil config")
	}

	if config.CurrentProfile != "default" {
		t.Errorf("CurrentProfile = %v, want default", config.CurrentProfile)
	}

	if config.Profiles == nil {
		t.Error("Profiles map is nil, want empty map")
	}
}

func TestLoad_ValidConfigFile(t *testing.T) {
	// Setup: Create temporary config file
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".lerian")
	configPath := filepath.Join(configDir, "config.yaml")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `current-profile: test
profiles:
  test:
    api-url: https://api.example.com
    api-key: test-key-123
    tenant-id: tenant-456
  production:
    api-url: https://api.production.com
    api-key: prod-key-789
    tenant-id: tenant-789
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test: Load should parse the config correctly
	config, err := Load()

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify current profile
	if config.CurrentProfile != "test" {
		t.Errorf("CurrentProfile = %v, want test", config.CurrentProfile)
	}

	// Verify profiles
	if len(config.Profiles) != 2 {
		t.Errorf("got %d profiles, want 2", len(config.Profiles))
	}

	// Verify test profile
	testProfile, ok := config.Profiles["test"]
	if !ok {
		t.Fatal("test profile not found")
	}

	if testProfile.APIURL != "https://api.example.com" {
		t.Errorf("APIURL = %v, want https://api.example.com", testProfile.APIURL)
	}

	if testProfile.APIKey != "test-key-123" {
		t.Errorf("APIKey = %v, want test-key-123", testProfile.APIKey)
	}

	if testProfile.TenantID != "tenant-456" {
		t.Errorf("TenantID = %v, want tenant-456", testProfile.TenantID)
	}

	// Verify production profile exists
	_, ok = config.Profiles["production"]
	if !ok {
		t.Error("production profile not found")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	// Setup: Create temporary config file with invalid YAML
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".lerian")
	configPath := filepath.Join(configDir, "config.yaml")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	invalidYAML := `current-profile: test
profiles:
  test:
    api-url: https://api.example.com
    api-key: test-key
    invalid yaml here without proper indentation
  tenant-id: broken
`

	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test: Load should return error for invalid YAML
	_, err := Load()

	if err == nil {
		t.Error("Load() error = nil, want error for invalid YAML")
	}
}

func TestSave(t *testing.T) {
	// Setup: Use temporary directory
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create test config
	config := &Config{
		CurrentProfile: "test",
		Profiles: map[string]Profile{
			"test": {
				APIURL:   "https://api.test.com",
				APIKey:   "test-key",
				TenantID: "test-tenant",
			},
		},
	}

	// Test: Save should create config file
	err := config.Save()

	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, ".lerian", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Verify file content by loading it back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.CurrentProfile != config.CurrentProfile {
		t.Errorf("CurrentProfile = %v, want %v", loaded.CurrentProfile, config.CurrentProfile)
	}

	if len(loaded.Profiles) != len(config.Profiles) {
		t.Errorf("got %d profiles, want %d", len(loaded.Profiles), len(config.Profiles))
	}
}

func TestGetProfile(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		profileName    string
		wantProfile    *Profile
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "valid current profile with empty string",
			config: &Config{
				CurrentProfile: "test",
				Profiles: map[string]Profile{
					"test": {
						APIURL:   "https://api.test.com",
						APIKey:   "key",
						TenantID: "tenant",
					},
				},
			},
			profileName: "",
			wantProfile: &Profile{
				APIURL:   "https://api.test.com",
				APIKey:   "key",
				TenantID: "tenant",
			},
			wantErr: false,
		},
		{
			name: "valid named profile",
			config: &Config{
				CurrentProfile: "default",
				Profiles: map[string]Profile{
					"test": {
						APIURL:   "https://api.example.com",
						APIKey:   "test-key",
						TenantID: "test-tenant",
					},
				},
			},
			profileName: "test",
			wantProfile: &Profile{
				APIURL:   "https://api.example.com",
				APIKey:   "test-key",
				TenantID: "test-tenant",
			},
			wantErr: false,
		},
		{
			name: "profile not found",
			config: &Config{
				CurrentProfile: "missing",
				Profiles:       map[string]Profile{},
			},
			profileName:    "",
			wantProfile:    nil,
			wantErr:        true,
			wantErrMessage: "profile 'missing' not found",
		},
		{
			name: "named profile not found",
			config: &Config{
				CurrentProfile: "default",
				Profiles: map[string]Profile{
					"default": {APIURL: "url"},
				},
			},
			profileName:    "nonexistent",
			wantProfile:    nil,
			wantErr:        true,
			wantErrMessage: "profile 'nonexistent' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := tt.config.GetProfile(tt.profileName)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.wantErrMessage != "" && err.Error() != tt.wantErrMessage {
					t.Errorf("error message = %v, want %v", err.Error(), tt.wantErrMessage)
				}
				return
			}

			if profile == nil && tt.wantProfile != nil {
				t.Error("GetProfile() returned nil, want non-nil profile")
				return
			}

			if profile != nil && tt.wantProfile != nil {
				if profile.APIURL != tt.wantProfile.APIURL {
					t.Errorf("APIURL = %v, want %v", profile.APIURL, tt.wantProfile.APIURL)
				}
				if profile.APIKey != tt.wantProfile.APIKey {
					t.Errorf("APIKey = %v, want %v", profile.APIKey, tt.wantProfile.APIKey)
				}
				if profile.TenantID != tt.wantProfile.TenantID {
					t.Errorf("TenantID = %v, want %v", profile.TenantID, tt.wantProfile.TenantID)
				}
			}
		})
	}
}

func TestSetProfile(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		profileName string
		profile     Profile
		wantLen     int
	}{
		{
			name: "add new profile to existing profiles",
			config: &Config{
				CurrentProfile: "default",
				Profiles: map[string]Profile{
					"default": {
						APIURL:   "https://api.default.com",
						APIKey:   "default-key",
						TenantID: "default-tenant",
					},
				},
			},
			profileName: "test",
			profile: Profile{
				APIURL:   "https://api.test.com",
				APIKey:   "test-key",
				TenantID: "test-tenant",
			},
			wantLen: 2,
		},
		{
			name: "add profile to nil profiles map",
			config: &Config{
				CurrentProfile: "default",
				Profiles:       nil,
			},
			profileName: "test",
			profile: Profile{
				APIURL:   "https://api.test.com",
				APIKey:   "test-key",
				TenantID: "test-tenant",
			},
			wantLen: 1,
		},
		{
			name: "update existing profile",
			config: &Config{
				CurrentProfile: "test",
				Profiles: map[string]Profile{
					"test": {
						APIURL:   "https://api.old.com",
						APIKey:   "old-key",
						TenantID: "old-tenant",
					},
				},
			},
			profileName: "test",
			profile: Profile{
				APIURL:   "https://api.new.com",
				APIKey:   "new-key",
				TenantID: "new-tenant",
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetProfile(tt.profileName, tt.profile)

			if len(tt.config.Profiles) != tt.wantLen {
				t.Errorf("profiles count = %d, want %d", len(tt.config.Profiles), tt.wantLen)
			}

			profile, ok := tt.config.Profiles[tt.profileName]
			if !ok {
				t.Fatalf("profile %s not found after SetProfile", tt.profileName)
			}

			if profile.APIURL != tt.profile.APIURL {
				t.Errorf("APIURL = %v, want %v", profile.APIURL, tt.profile.APIURL)
			}
			if profile.APIKey != tt.profile.APIKey {
				t.Errorf("APIKey = %v, want %v", profile.APIKey, tt.profile.APIKey)
			}
			if profile.TenantID != tt.profile.TenantID {
				t.Errorf("TenantID = %v, want %v", profile.TenantID, tt.profile.TenantID)
			}
		})
	}
}

func TestDeleteProfile(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		profileName    string
		wantErr        bool
		wantErrMessage string
		wantLen        int
	}{
		{
			name: "delete non-current profile successfully",
			config: &Config{
				CurrentProfile: "default",
				Profiles: map[string]Profile{
					"default": {APIURL: "https://api.default.com"},
					"test":    {APIURL: "https://api.test.com"},
				},
			},
			profileName: "test",
			wantErr:     false,
			wantLen:     1,
		},
		{
			name: "cannot delete current profile",
			config: &Config{
				CurrentProfile: "test",
				Profiles: map[string]Profile{
					"test": {APIURL: "https://api.test.com"},
				},
			},
			profileName:    "test",
			wantErr:        true,
			wantErrMessage: "cannot delete current profile",
			wantLen:        1,
		},
		{
			name: "cannot delete non-existent profile",
			config: &Config{
				CurrentProfile: "default",
				Profiles: map[string]Profile{
					"default": {APIURL: "https://api.default.com"},
				},
			},
			profileName:    "nonexistent",
			wantErr:        true,
			wantErrMessage: "profile 'nonexistent' not found",
			wantLen:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.DeleteProfile(tt.profileName)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if err.Error() != tt.wantErrMessage {
					t.Errorf("error message = %v, want %v", err.Error(), tt.wantErrMessage)
				}
			}

			if len(tt.config.Profiles) != tt.wantLen {
				t.Errorf("profiles count = %d, want %d", len(tt.config.Profiles), tt.wantLen)
			}
		})
	}
}

func TestLoad_FileExistsButUnreadable(t *testing.T) {
	// Setup: Create config file with no read permissions
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".lerian")
	configPath := filepath.Join(configDir, "config.yaml")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create file with valid content
	configContent := `current-profile: test
profiles:
  test:
    api-url: https://api.test.com
    api-key: test-key
    tenant-id: test-tenant
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Make file unreadable
	if err := os.Chmod(configPath, 0000); err != nil {
		t.Fatalf("failed to chmod config file: %v", err)
	}

	// Restore permissions after test
	defer os.Chmod(configPath, 0644)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test: Load should return error for unreadable file
	_, err := Load()

	if err == nil {
		t.Error("Load() error = nil, want error for unreadable file")
	}
}

func TestSave_DirectoryCreationFailure(t *testing.T) {
	// This test is challenging to implement reliably across platforms
	// because we need to create a scenario where MkdirAll fails.
	// Skipping on systems where we can't simulate this properly.
	t.Skip("Skipping directory creation failure test - requires special setup")
}

func TestSave_GetConfigPathFailure(t *testing.T) {
	// This test would require mocking os.UserHomeDir() which is not
	// straightforward without dependency injection or build tags.
	// This error path is tested indirectly through other tests.
	t.Skip("Skipping GetConfigPath failure test - requires mocking")
}

// Benchmark tests
func BenchmarkLoad(b *testing.B) {
	// Setup
	tmpDir := b.TempDir()
	configDir := filepath.Join(tmpDir, ".lerian")
	configPath := filepath.Join(configDir, "config.yaml")

	os.MkdirAll(configDir, 0755)
	configContent := `current-profile: test
profiles:
  test:
    api-url: https://api.example.com
    api-key: test-key
    tenant-id: tenant-id
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Load()
	}
}
