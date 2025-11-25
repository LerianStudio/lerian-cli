//nolint:errcheck // Test file - error checking not critical for test setup
package output

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestNewPrinter(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{"json format", "json", "json"},
		{"yaml format", "yaml", "yaml"},
		{"table format", "table", "table"},
		{"empty format", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPrinter(tt.format)
			if p.Format != tt.want {
				t.Errorf("NewPrinter() Format = %v, want %v", p.Format, tt.want)
			}
		})
	}
}

func TestPrinter_Print(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		data    interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name:    "json format",
			format:  "json",
			data:    map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "yaml format",
			format:  "yaml",
			data:    map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:   "table format with map slice",
			format: "table",
			data: []map[string]interface{}{
				{"id": "123", "name": "test"},
			},
			wantErr: false,
		},
		{
			name:    "unsupported format",
			format:  "xml",
			data:    map[string]string{"key": "value"},
			wantErr: true,
			errMsg:  "unsupported output format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			p := NewPrinter(tt.format)
			err := p.Print(tt.data)

			// Restore stdout
			w.Close()
			os.Stdout = old

			if (err != nil) != tt.wantErr {
				t.Errorf("Print() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Print() error = %v, want error containing %v", err, tt.errMsg)
			}

			// Read captured output
			io.ReadAll(r)
		})
	}
}

func TestPrinter_printJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
	}{
		{
			name:    "simple map",
			data:    map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "slice",
			data:    []string{"item1", "item2"},
			wantErr: false,
		},
		{
			name:    "nil",
			data:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			p := NewPrinter("json")
			err := p.printJSON(tt.data)

			// Restore stdout
			w.Close()
			os.Stdout = old

			if (err != nil) != tt.wantErr {
				t.Errorf("printJSON() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Read and validate output
			out, _ := io.ReadAll(r)
			if !tt.wantErr && len(out) > 0 {
				var result interface{}
				if err := json.Unmarshal(out, &result); err != nil && tt.data != nil {
					t.Errorf("printJSON() output is not valid JSON: %v", err)
				}
			}
		})
	}
}

func TestPrinter_printYAML(t *testing.T) {
	data := map[string]string{"key": "value"}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	p := NewPrinter("yaml")
	err := p.printYAML(data)

	// Restore stdout
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Errorf("printYAML() error = %v", err)
	}

	// Read output
	io.ReadAll(r)
}

func TestPrinter_printTable(t *testing.T) {
	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
	}{
		{
			name:    "empty slice",
			data:    []map[string]interface{}{},
			wantErr: false,
		},
		{
			name: "single row",
			data: []map[string]interface{}{
				{"id": "123", "name": "test"},
			},
			wantErr: false,
		},
		{
			name: "multiple rows",
			data: []map[string]interface{}{
				{"id": "123", "name": "test1"},
				{"id": "456", "name": "test2"},
			},
			wantErr: false,
		},
		{
			name:    "unknown type fallback to JSON",
			data:    "string data",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			p := NewPrinter("table")
			err := p.printTable(tt.data)

			// Restore stdout
			w.Close()
			os.Stdout = old

			if (err != nil) != tt.wantErr {
				t.Errorf("printTable() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Read output
			io.ReadAll(r)
		})
	}
}

func TestPrinter_printMapSliceTable(t *testing.T) {
	tests := []struct {
		name    string
		data    []map[string]interface{}
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []map[string]interface{}{},
			wantErr: false,
		},
		{
			name: "single column",
			data: []map[string]interface{}{
				{"name": "test"},
			},
			wantErr: false,
		},
		{
			name: "multiple columns",
			data: []map[string]interface{}{
				{"id": "123", "name": "test", "status": "active"},
			},
			wantErr: false,
		},
		{
			name: "with nil values",
			data: []map[string]interface{}{
				{"id": "123", "name": nil, "status": "active"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			p := NewPrinter("table")
			err := p.printMapSliceTable(tt.data)

			// Restore stdout
			w.Close()
			os.Stdout = old

			if (err != nil) != tt.wantErr {
				t.Errorf("printMapSliceTable() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Read output
			io.ReadAll(r)
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"nil value", nil, ""},
		{"string value", "test", "test"},
		{"int value", 123, "123"},
		{"bool value", true, "true"},
		{"float value", 3.14, "3.14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatValue(tt.value); got != tt.want {
				t.Errorf("formatValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrinter_PrintLedgerList(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		ledgers interface{}
		wantErr bool
	}{
		{
			name:   "json format",
			format: "json",
			ledgers: []interface{}{
				map[string]interface{}{"id": "123", "name": "test"},
			},
			wantErr: false,
		},
		{
			name:    "empty ledgers",
			format:  "table",
			ledgers: []interface{}{},
			wantErr: false,
		},
		{
			name:   "table format with data",
			format: "table",
			ledgers: []interface{}{
				map[string]interface{}{
					"id":         "123-456",
					"name":       "test-ledger",
					"region":     "us-east-1",
					"status":     "active",
					"created_at": "2025-11-25",
				},
			},
			wantErr: false,
		},
		{
			name:   "long id truncation",
			format: "table",
			ledgers: []interface{}{
				map[string]interface{}{
					"id":         "123456789012345678901234567890123456789012345678",
					"name":       "test",
					"region":     "us-east-1",
					"status":     "active",
					"created_at": "2025-11-25",
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid ledgers type",
			format:  "table",
			ledgers: "invalid",
			wantErr: false, // Falls back to Print()
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			p := NewPrinter(tt.format)
			err := p.PrintLedgerList(tt.ledgers)

			// Restore stdout
			w.Close()
			os.Stdout = old

			if (err != nil) != tt.wantErr {
				t.Errorf("PrintLedgerList() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Read output
			out, _ := io.ReadAll(r)
			if len(out) == 0 && !tt.wantErr {
				// Some test cases expect output
			}
		})
	}
}

func TestPrinter_PrintLedgerDetails(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		ledger  interface{}
		wantErr bool
	}{
		{
			name:   "json format",
			format: "json",
			ledger: map[string]interface{}{
				"id":                "123",
				"name":              "test-ledger",
				"region":            "us-east-1",
				"status":            "active",
				"helm_release_name": "release-1",
				"helm_chart":        "chart-1",
				"helm_namespace":    "namespace-1",
				"created_at":        "2025-11-25",
				"updated_at":        "2025-11-25",
			},
			wantErr: false,
		},
		{
			name:   "table format",
			format: "table",
			ledger: map[string]interface{}{
				"id":                "123",
				"name":              "test-ledger",
				"region":            "us-east-1",
				"status":            "active",
				"helm_release_name": "release-1",
				"helm_chart":        "chart-1",
				"helm_namespace":    "namespace-1",
				"created_at":        "2025-11-25",
				"updated_at":        "2025-11-25",
			},
			wantErr: false,
		},
		{
			name:    "invalid ledger type",
			format:  "json", // Use JSON format for invalid type test
			ledger:  "invalid",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			p := NewPrinter(tt.format)
			err := p.PrintLedgerDetails(tt.ledger)

			// Restore stdout
			w.Close()
			os.Stdout = old

			if (err != nil) != tt.wantErr {
				t.Errorf("PrintLedgerDetails() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Read output
			out, _ := io.ReadAll(r)
			if tt.format == "table" && len(out) > 0 {
				output := string(out)
				if !strings.Contains(output, "Ledger Details") {
					t.Errorf("PrintLedgerDetails() output doesn't contain expected header")
				}
			}
		})
	}
}

func TestPrinter_Integration(t *testing.T) {
	// Test full workflow
	data := []map[string]interface{}{
		{"id": "1", "name": "item1", "value": 100},
		{"id": "2", "name": "item2", "value": 200},
	}

	formats := []string{"json", "yaml", "table"}
	for _, format := range formats {
		t.Run("format_"+format, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			p := NewPrinter(format)
			err := p.Print(data)

			// Restore stdout
			w.Close()
			os.Stdout = old

			if err != nil {
				t.Errorf("Integration test for format %s failed: %v", format, err)
			}

			// Read output
			out, _ := io.ReadAll(r)
			if len(out) == 0 {
				t.Errorf("Integration test for format %s produced no output", format)
			}
		})
	}
}

// Helper function to capture stdout for testing
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestCaptureOutputHelper(t *testing.T) {
	output := captureOutput(func() {
		p := NewPrinter("json")
		p.Print(map[string]string{"test": "value"})
	})

	if !strings.Contains(output, "test") {
		t.Errorf("captureOutput() helper not working correctly")
	}
}
