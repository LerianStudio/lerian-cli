package version

import (
	"encoding/json"
	"fmt"
	"runtime"
)

// Build information. Populated at build-time via ldflags.
var (
	// Version is the semantic version (e.g., "v0.1.0").
	Version = "dev"

	// Commit is the git commit hash.
	Commit = "none"

	// Date is the build date.
	Date = "unknown"

	// BuiltBy is the builder identifier.
	BuiltBy = "manual"
)

// Info contains version information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	BuiltBy   string `json:"builtBy"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// GetInfo returns the version information.
func GetInfo() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		BuiltBy:   BuiltBy,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a human-readable version string.
func (i Info) String() string {
	return fmt.Sprintf(
		"lerian version %s\ncommit: %s\nbuilt at: %s\nbuilt by: %s\ngo version: %s\nplatform: %s",
		i.Version,
		i.Commit,
		i.Date,
		i.BuiltBy,
		i.GoVersion,
		i.Platform,
	)
}

// Short returns a short version string.
func (i Info) Short() string {
	return fmt.Sprintf("lerian version %s", i.Version)
}

// JSON returns the version information as JSON.
func (i Info) JSON() (string, error) {
	data, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal version info: %w", err)
	}
	return string(data), nil
}

// GetVersion returns just the version string.
func GetVersion() string {
	return Version
}

// GetCommit returns just the commit hash.
func GetCommit() string {
	return Commit
}

// GetDate returns just the build date.
func GetDate() string {
	return Date
}

// GetBuiltBy returns just the builder identifier.
func GetBuiltBy() string {
	return BuiltBy
}
