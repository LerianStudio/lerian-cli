package infra_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This CLI is run by people who do not work here, on machines that have never
// seen our accounts. An example that names one of our profiles tells somebody
// else's operator to create a profile called lerian-something, which is a
// suggestion about their organization that we are in no position to make.
//
// The product's own names are fine and unavoidable — the binary, the config
// directory, the templates repository, the state bucket the templates create.
// What this catches is our internal naming leaking into advice.
func TestNoInternalProfileNamesInAdvice(t *testing.T) {
	// Spellings that could only be an example of a profile or account of ours.
	banned := []string{
		"lerian-dev",
		"lerian-prd",
		"lerian-stg",
		"lerian-sandbox",
		"lerian-production",
		"lerian-staging",
		"lerian-devops",
		"lerian-network",
	}

	roots := []string{"..", "../../cmd"}
	for _, root := range roots {
		walkGoFiles(t, root, func(path, body string) {
			for _, name := range banned {
				if strings.Contains(body, name) {
					t.Errorf("%s names %q, which is a profile of ours offered as an example", path, name)
				}
			}
		})
	}
}

func walkGoFiles(t *testing.T, root string, check func(path, body string)) {
	t.Helper()

	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// A tree this cannot read is a tree it cannot check, and failing the
			// sweep on it would report a house-style problem that is not one.
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		check(path, string(body))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Our region is a fine example and a bad default. A prompt that offers it turns
// the most likely keypress into "create everything in Ohio", which is our answer
// and not necessarily anybody else's — and where the resources land is not
// something to inherit from a tool's habit.
func TestOurRegionIsNotOfferedAsTheDefault(t *testing.T) {
	offered := []string{
		`suggestion = "us-east-2"`,
		`ask.text("AWS region", "us-east-2"`,
	}

	walkGoFiles(t, "../infracli", func(path, body string) {
		for _, form := range offered {
			if strings.Contains(body, form) {
				t.Errorf("%s offers our own region as the default: %s", path, form)
			}
		}
	})
}
