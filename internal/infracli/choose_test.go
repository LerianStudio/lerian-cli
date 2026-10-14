package infracli

import (
	"bytes"
	"os"
	"testing"
)

// The menu needs both ends: somebody at the keyboard, and a screen they can see
// it on. Stdin alone is not enough — with output redirected the menu is painted
// into the file while the terminal waits for a keypress, so the operator sees a
// command that has stopped and nothing saying why.
//
// This pins the half CanAsk added, and it is the half that can be tested: under
// go test stdin is never a terminal, so a test of CanAsk itself returns false
// whichever way it is written and would pass against the version that only
// looked at stdin.
func TestWhatCountsAsAScreenTheMenuCanBePaintedOn(t *testing.T) {
	regular, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = regular.Close() }()

	tests := []struct {
		name string
		out  interface{ Write([]byte) (int, error) }
	}{
		{"a buffer, which is what a test or a tee passes", &bytes.Buffer{}},
		{"a redirect, which is a file but not a terminal", regular},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if writerIsTerminal(test.out) {
				t.Error("taken for a terminal; the menu would be painted where nobody can see it")
			}
		})
	}
}

// CanAsk has to consult it. Stdin is not a terminal here, so this cannot prove
// the output half is consulted — it only pins that a writer nobody can read
// never yields a menu.
func TestCanAskRefusesAWriterNobodyCanRead(t *testing.T) {
	if CanAsk(&bytes.Buffer{}) {
		t.Error("CanAsk said yes for a buffer")
	}
}

// A row that must not be chosen has to arrive at the selector saying so. The
// caller marks it; this is the only place that mark can be lost.
func TestChoosePreservesDisabled(t *testing.T) {
	options := optionsFrom([]Choice{
		{Value: "works", Label: "works", Note: "fine"},
		{Value: "broken", Label: "broken", Note: "not available yet", Disabled: true},
	})

	if len(options) != 2 {
		t.Fatalf("got %d options, want 2", len(options))
	}
	if options[0].disabled {
		t.Error("a row that can be chosen was disabled")
	}
	if !options[1].disabled {
		t.Error("a disabled row arrived selectable")
	}
	if options[1].note != "not available yet" {
		t.Errorf("note = %q", options[1].note)
	}
}
