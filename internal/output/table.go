package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// Printer handles different output formats
type Printer struct {
	Format string
}

// NewPrinter creates a new output printer
func NewPrinter(format string) *Printer {
	return &Printer{Format: format}
}

// Print outputs data in the specified format
func (p *Printer) Print(data interface{}) error {
	switch strings.ToLower(p.Format) {
	case "json":
		return p.printJSON(data)
	case "yaml":
		return p.printYAML(data)
	case "table":
		return p.printTable(data)
	default:
		return fmt.Errorf("unsupported output format: %s", p.Format)
	}
}

// printJSON outputs data as JSON
func (p *Printer) printJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// printYAML outputs data as YAML (simplified JSON for now)
func (p *Printer) printYAML(data interface{}) error {
	// For now, just use JSON format
	// In a real implementation, you'd use gopkg.in/yaml.v3
	return p.printJSON(data)
}

// printTable outputs data as a table
func (p *Printer) printTable(data interface{}) error {
	// Type assertion based on known types
	switch v := data.(type) {
	case []map[string]interface{}:
		return p.printMapSliceTable(v)
	default:
		// Fallback to JSON for unknown types
		return p.printJSON(data)
	}
}

// printMapSliceTable prints a slice of maps as a table
func (p *Printer) printMapSliceTable(data []map[string]interface{}) error {
	if len(data) == 0 {
		fmt.Println("No data to display")
		return nil
	}

	// Get all unique keys
	keySet := make(map[string]bool)
	for _, row := range data {
		for key := range row {
			keySet[key] = true
		}
	}

	// Convert to slice and sort (simple approach)
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}

	// Create tabwriter
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	// Print header
	_, _ = fmt.Fprint(w, strings.ToUpper(keys[0]))
	for i := 1; i < len(keys); i++ {
		_, _ = fmt.Fprintf(w, "\t%s", strings.ToUpper(keys[i]))
	}
	_, _ = fmt.Fprintln(w)

	// Print rows
	for _, row := range data {
		_, _ = fmt.Fprint(w, formatValue(row[keys[0]]))
		for i := 1; i < len(keys); i++ {
			_, _ = fmt.Fprintf(w, "\t%s", formatValue(row[keys[i]]))
		}
		_, _ = fmt.Fprintln(w)
	}

	return w.Flush()
}

// formatValue converts a value to a string for display
func formatValue(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// PrintLedgerList prints ledger list in table format
func (p *Printer) PrintLedgerList(ledgers interface{}) error {
	if p.Format == "json" {
		return p.printJSON(ledgers)
	}

	// Convert to map slice for table printing
	data, ok := ledgers.([]interface{})
	if !ok {
		return p.Print(ledgers)
	}

	if len(data) == 0 {
		fmt.Println("No ledgers found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tREGION\tSTATUS\tCREATED")

	for _, item := range data {
		ledger, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		id := formatValue(ledger["id"])
		name := formatValue(ledger["name"])
		region := formatValue(ledger["region"])
		status := formatValue(ledger["status"])
		createdAt := formatValue(ledger["created_at"])

		// Truncate long IDs for better display
		if len(id) > 36 {
			id = id[:36]
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, name, region, status, createdAt)
	}

	return w.Flush()
}

// PrintLedgerDetails prints detailed ledger information
func (p *Printer) PrintLedgerDetails(ledger interface{}) error {
	if p.Format == "json" {
		return p.printJSON(ledger)
	}

	data, ok := ledger.(map[string]interface{})
	if !ok {
		return p.Print(ledger)
	}

	fmt.Println("Ledger Details:")
	fmt.Println("===============")
	fmt.Printf("ID:                %s\n", formatValue(data["id"]))
	fmt.Printf("Name:              %s\n", formatValue(data["name"]))
	fmt.Printf("Region:            %s\n", formatValue(data["region"]))
	fmt.Printf("Status:            %s\n", formatValue(data["status"]))
	fmt.Printf("Helm Release:      %s\n", formatValue(data["helm_release_name"]))
	fmt.Printf("Helm Chart:        %s\n", formatValue(data["helm_chart"]))
	fmt.Printf("Helm Namespace:    %s\n", formatValue(data["helm_namespace"]))
	fmt.Printf("Created At:        %s\n", formatValue(data["created_at"]))
	fmt.Printf("Updated At:        %s\n", formatValue(data["updated_at"]))

	return nil
}
