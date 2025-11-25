//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var (
	binaryPath string
)

// TestMain builds the binary before running tests
func TestMain(m *testing.M) {
	// Build the binary
	tmpDir := os.TempDir()
	binaryPath = filepath.Join(tmpDir, "lerian-test")

	// Build command - explicitly specify main.go
	mainFile := filepath.Join("cmd", "lerian", "main.go")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, mainFile)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	// Set working directory to project root
	if wd, err := os.Getwd(); err == nil {
		// We're in cmd/lerian, go up two levels to project root
		projectRoot := filepath.Join(wd, "..", "..")
		buildCmd.Dir = projectRoot
	}

	if err := buildCmd.Run(); err != nil {
		panic("Failed to build test binary: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Cleanup
	os.Remove(binaryPath)

	os.Exit(code)
}

func TestCLI_Version_Flag(t *testing.T) {
	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)

	// Check output starts with "lerian version"
	if !strings.HasPrefix(outputStr, "lerian version") {
		t.Errorf("Expected output to start with 'lerian version', got: %s", outputStr)
	}

	// Should be a single line
	lines := strings.Split(strings.TrimSpace(outputStr), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected single line output, got %d lines", len(lines))
	}
}

func TestCLI_VersionCommand(t *testing.T) {
	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)

	// Check for expected components
	expectedStrings := []string{
		"lerian version",
		"commit:",
		"built at:",
		"built by:",
		"go version:",
		"platform:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected output to contain '%s', but it didn't.\nOutput: %s", expected, outputStr)
		}
	}

	// Should be multi-line
	lines := strings.Split(strings.TrimSpace(outputStr), "\n")
	if len(lines) < 6 {
		t.Errorf("Expected at least 6 lines of output, got %d", len(lines))
	}
}

func TestCLI_VersionCommand_JSON(t *testing.T) {
	cmd := exec.Command(binaryPath, "version", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, output)
	}

	// Parse JSON
	var versionInfo map[string]interface{}
	if err := json.Unmarshal(output, &versionInfo); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, output)
	}

	// Check required fields
	requiredFields := []string{"version", "commit", "date", "builtBy", "goVersion", "platform"}
	for _, field := range requiredFields {
		if _, exists := versionInfo[field]; !exists {
			t.Errorf("Expected JSON to contain field '%s'", field)
		}
	}

	// Check field types
	if _, ok := versionInfo["version"].(string); !ok {
		t.Errorf("Expected 'version' to be a string")
	}

	if _, ok := versionInfo["commit"].(string); !ok {
		t.Errorf("Expected 'commit' to be a string")
	}

	// Check platform format
	platform, ok := versionInfo["platform"].(string)
	if !ok {
		t.Fatal("Expected 'platform' to be a string")
	}

	if !strings.Contains(platform, "/") {
		t.Errorf("Expected platform to contain '/', got: %s", platform)
	}
}

func TestCLI_Help(t *testing.T) {
	cmd := exec.Command(binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)

	// Check for expected help sections
	expectedStrings := []string{
		"Lerian CLI",
		"Usage:",
		"Available Commands:",
		"Flags:",
		"--help",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected help output to contain '%s'", expected)
		}
	}
}

func TestCLI_VersionHelp(t *testing.T) {
	cmd := exec.Command(binaryPath, "version", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)

	// Check for expected help content
	expectedStrings := []string{
		"Print",
		"version",
		"Usage:",
		"--json",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Expected version help to contain '%s'", expected)
		}
	}
}

func TestCLI_InvalidCommand(t *testing.T) {
	cmd := exec.Command(binaryPath, "nonexistent-command")
	output, err := cmd.CombinedOutput()

	// Should fail
	if err == nil {
		t.Error("Expected command to fail for invalid command")
	}

	outputStr := string(output)

	// Should contain error message
	if !strings.Contains(outputStr, "unknown command") && !strings.Contains(outputStr, "Error") {
		t.Errorf("Expected error message for unknown command, got: %s", outputStr)
	}
}

func TestCLI_NoArgs(t *testing.T) {
	cmd := exec.Command(binaryPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)

	// Should show help when run without arguments
	if !strings.Contains(outputStr, "Usage:") {
		t.Errorf("Expected help output when run without arguments, got: %s", outputStr)
	}
}

func TestCLI_VersionCommand_ExitCode(t *testing.T) {
	cmd := exec.Command(binaryPath, "version")
	err := cmd.Run()

	if err != nil {
		t.Errorf("Expected version command to exit with code 0, but got error: %v", err)
	}
}

func TestCLI_GlobalFlags(t *testing.T) {
	// Test that global flags don't cause errors (even if they do nothing without subcommands)
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "config flag",
			args: []string{"--config", "/tmp/test.yaml", "--help"},
		},
		{
			name: "profile flag",
			args: []string{"--profile", "test", "--help"},
		},
		{
			name: "output flag",
			args: []string{"--output", "json", "--help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tt.args...)
			_, err := cmd.CombinedOutput()

			if err != nil {
				t.Errorf("Command failed with args %v: %v", tt.args, err)
			}
		})
	}
}

func TestCLI_VersionJSON_ValidJSON(t *testing.T) {
	cmd := exec.Command(binaryPath, "version", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Should be valid JSON
	var result interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Errorf("Output is not valid JSON: %v\nOutput: %s", err, output)
	}

	// Should be pretty-printed (has indentation)
	outputStr := string(output)
	if !strings.Contains(outputStr, "  ") {
		t.Error("Expected JSON to be pretty-printed with indentation")
	}
}
