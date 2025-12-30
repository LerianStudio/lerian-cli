package version

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestGetInfo(t *testing.T) {
	// Set test values
	Version = "v1.0.0"
	Commit = "abc123"
	Date = "2025-11-22T00:00:00Z"
	BuiltBy = "test@test.local"

	info := GetInfo()

	if info.Version != "v1.0.0" {
		t.Errorf("Expected version v1.0.0, got %s", info.Version)
	}

	if info.Commit != "abc123" {
		t.Errorf("Expected commit abc123, got %s", info.Commit)
	}

	if info.Date != "2025-11-22T00:00:00Z" {
		t.Errorf("Expected date 2025-11-22T00:00:00Z, got %s", info.Date)
	}

	if info.BuiltBy != "test@test.local" {
		t.Errorf("Expected builtBy test@test.local, got %s", info.BuiltBy)
	}

	if info.GoVersion != runtime.Version() {
		t.Errorf("Expected Go version %s, got %s", runtime.Version(), info.GoVersion)
	}

	expectedPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if info.Platform != expectedPlatform {
		t.Errorf("Expected platform %s, got %s", expectedPlatform, info.Platform)
	}
}

func TestInfoString(t *testing.T) {
	// Set test values
	Version = "v1.0.0"
	Commit = "abc123"
	Date = "2025-11-22T00:00:00Z"
	BuiltBy = "test@test.local"

	info := GetInfo()
	str := info.String()

	// Check that all components are present
	expectedComponents := []string{
		"lerian version v1.0.0",
		"commit: abc123",
		"built at: 2025-11-22T00:00:00Z",
		"built by: test@test.local",
		"go version: " + runtime.Version(),
		"platform: " + runtime.GOOS + "/" + runtime.GOARCH,
	}

	for _, component := range expectedComponents {
		if !strings.Contains(str, component) {
			t.Errorf("Expected string to contain '%s', but it didn't.\nGot: %s", component, str)
		}
	}
}

func TestInfoShort(t *testing.T) {
	// Set test values
	Version = "v1.0.0"

	info := GetInfo()
	short := info.Short()

	expected := "lerian version v1.0.0"
	if short != expected {
		t.Errorf("Expected short format '%s', got '%s'", expected, short)
	}
}

func TestInfoJSON(t *testing.T) {
	// Set test values
	Version = "v1.0.0"
	Commit = "abc123"
	Date = "2025-11-22T00:00:00Z"
	BuiltBy = "test@test.local"

	info := GetInfo()
	jsonStr, err := info.JSON()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Parse JSON to verify it's valid
	var parsed Info
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify fields
	if parsed.Version != "v1.0.0" {
		t.Errorf("Expected version v1.0.0, got %s", parsed.Version)
	}

	if parsed.Commit != "abc123" {
		t.Errorf("Expected commit abc123, got %s", parsed.Commit)
	}

	if parsed.Date != "2025-11-22T00:00:00Z" {
		t.Errorf("Expected date 2025-11-22T00:00:00Z, got %s", parsed.Date)
	}

	if parsed.BuiltBy != "test@test.local" {
		t.Errorf("Expected builtBy test@test.local, got %s", parsed.BuiltBy)
	}

	// Verify JSON is pretty-printed (has indentation)
	if !strings.Contains(jsonStr, "  ") {
		t.Error("Expected JSON to be indented")
	}
}

func TestGetters(t *testing.T) {
	// Set test values
	Version = "v1.0.0"
	Commit = "abc123"
	Date = "2025-11-22T00:00:00Z"
	BuiltBy = "test@test.local"

	if GetVersion() != "v1.0.0" {
		t.Errorf("GetVersion() expected v1.0.0, got %s", GetVersion())
	}

	if GetCommit() != "abc123" {
		t.Errorf("GetCommit() expected abc123, got %s", GetCommit())
	}

	if GetDate() != "2025-11-22T00:00:00Z" {
		t.Errorf("GetDate() expected 2025-11-22T00:00:00Z, got %s", GetDate())
	}

	if GetBuiltBy() != "test@test.local" {
		t.Errorf("GetBuiltBy() expected test@test.local, got %s", GetBuiltBy())
	}
}

func TestDefaultValues(t *testing.T) {
	// Reset to default values
	Version = "dev"
	Commit = "none"
	Date = "unknown"
	BuiltBy = "manual"

	info := GetInfo()

	if info.Version != "dev" {
		t.Errorf("Default version should be 'dev', got %s", info.Version)
	}

	if info.Commit != "none" {
		t.Errorf("Default commit should be 'none', got %s", info.Commit)
	}

	if info.Date != "unknown" {
		t.Errorf("Default date should be 'unknown', got %s", info.Date)
	}

	if info.BuiltBy != "manual" {
		t.Errorf("Default builtBy should be 'manual', got %s", info.BuiltBy)
	}
}

func TestJSONRoundtrip(t *testing.T) {
	// Set test values
	Version = "v1.0.0"
	Commit = "abc123"
	Date = "2025-11-22T00:00:00Z"
	BuiltBy = "test@test.local"

	original := GetInfo()

	// Convert to JSON
	jsonStr, err := original.JSON()
	if err != nil {
		t.Fatalf("Failed to convert to JSON: %v", err)
	}

	// Parse back
	var parsed Info
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Compare
	if parsed.Version != original.Version {
		t.Errorf("Version mismatch after roundtrip: %s != %s", parsed.Version, original.Version)
	}

	if parsed.Commit != original.Commit {
		t.Errorf("Commit mismatch after roundtrip: %s != %s", parsed.Commit, original.Commit)
	}

	if parsed.Date != original.Date {
		t.Errorf("Date mismatch after roundtrip: %s != %s", parsed.Date, original.Date)
	}

	if parsed.BuiltBy != original.BuiltBy {
		t.Errorf("BuiltBy mismatch after roundtrip: %s != %s", parsed.BuiltBy, original.BuiltBy)
	}

	if parsed.GoVersion != original.GoVersion {
		t.Errorf("GoVersion mismatch after roundtrip: %s != %s", parsed.GoVersion, original.GoVersion)
	}

	if parsed.Platform != original.Platform {
		t.Errorf("Platform mismatch after roundtrip: %s != %s", parsed.Platform, original.Platform)
	}
}

func TestInfoStringFormat(t *testing.T) {
	// Set test values
	Version = "v1.0.0"
	Commit = "abc123"
	Date = "2025-11-22T00:00:00Z"
	BuiltBy = "test@test.local"

	info := GetInfo()
	str := info.String()

	// Check format has newlines (multi-line)
	if !strings.Contains(str, "\n") {
		t.Error("Expected String() to return multi-line format")
	}

	// Check starts with version
	if !strings.HasPrefix(str, "lerian version") {
		t.Errorf("Expected String() to start with 'lerian version', got: %s", str)
	}
}

func TestIsDevelopment(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"dev", true},
		{"", true},
		{"v1.0.0", false},
		{"v1.0.0-beta.1", false},
	}

	for _, tc := range tests {
		Version = tc.version
		if got := IsDevelopment(); got != tc.expected {
			t.Errorf("IsDevelopment() with version %q = %v, want %v", tc.version, got, tc.expected)
		}
	}
}

func TestIsRelease(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"dev", false},
		{"", false},
		{"v1.0.0", true},
		{"v1.0.0-beta.1", true},
	}

	for _, tc := range tests {
		Version = tc.version
		if got := IsRelease(); got != tc.expected {
			t.Errorf("IsRelease() with version %q = %v, want %v", tc.version, got, tc.expected)
		}
	}
}

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		v1, v2   string
		expected int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v2.0.0", -1},
		{"v2.0.0", "v1.0.0", 1},
		{"v1.0.0", "v1.1.0", -1},
	}

	for _, tc := range tests {
		if got := CompareVersion(tc.v1, tc.v2); got != tc.expected {
			t.Errorf("CompareVersion(%q, %q) = %d, want %d", tc.v1, tc.v2, got, tc.expected)
		}
	}
}

func BenchmarkGetInfo(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetInfo()
	}
}

func BenchmarkInfoString(b *testing.B) {
	info := GetInfo()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = info.String()
	}
}

func BenchmarkInfoJSON(b *testing.B) {
	info := GetInfo()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = info.JSON()
	}
}
