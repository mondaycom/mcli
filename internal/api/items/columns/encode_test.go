package columns

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mondaycom/mcli/internal/errs"
)

// codeOf returns an error's errs code, or "" if it is not an *errs.Error.
func codeOf(err error) errs.Code {
	if e, ok := errors.AsType[*errs.Error](err); ok {
		return e.Code
	}
	return ""
}

// statusSettingsObject is the index→name form monday uses for status columns.
const statusSettingsObject = `{"labels":{"0":"Not Started","1":"Working on it","2":"Done"}}`

// statusSettingsArray is the id/name array form some columns report instead.
const statusSettingsArray = `{"labels":[{"id":1,"name":"Open"},{"id":2,"name":"Closed"}]}`

func TestEncode_ok(t *testing.T) {
	tests := []struct {
		name       string
		columnType string
		settings   string
		input      string
		want       string
	}{
		{name: "text", columnType: "text", input: "Follow up", want: `"Follow up"`},
		{name: "text empty clears", columnType: "text", input: "", want: `""`},
		{name: "status exact", columnType: "status", settings: statusSettingsObject, input: "Done", want: `{"label":"Done"}`},
		{
			name: "status case-insensitive adopts board casing", columnType: "status",
			settings: statusSettingsObject, input: "done", want: `{"label":"Done"}`,
		},
		{name: "status array settings", columnType: "status", settings: statusSettingsArray, input: "closed", want: `{"label":"Closed"}`},
		{name: "status unvalidated without settings", columnType: "status", input: "Whatever", want: `{"label":"Whatever"}`},
		{name: "date only", columnType: "date", input: "2026-05-10", want: `{"date":"2026-05-10"}`},
		{name: "date and time", columnType: "date", input: "2026-05-10T14:30", want: `{"date":"2026-05-10","time":"14:30:00"}`},
		{name: "date space separated", columnType: "date", input: "2026-05-10 14:30:05", want: `{"date":"2026-05-10","time":"14:30:05"}`},
		{
			name: "rfc3339 converts to utc", columnType: "date",
			input: "2026-05-10T14:30:00+03:00", want: `{"date":"2026-05-10","time":"11:30:00"}`,
		},
		{name: "numbers int", columnType: "numbers", input: "42", want: `"42"`},
		{name: "numbers float", columnType: "numbers", input: "42.5", want: `"42.5"`},
		{name: "numbers negative", columnType: "numbers", input: "-7", want: `"-7"`},
		{name: "numbers empty clears", columnType: "numbers", input: "", want: `""`},
		{name: "checkbox true", columnType: "checkbox", input: "true", want: `{"checked":"true"}`},
		{name: "checkbox yes", columnType: "checkbox", input: "yes", want: `{"checked":"true"}`},
		{name: "checkbox n", columnType: "checkbox", input: "n", want: `{"checked":"false"}`},
		{name: "checkbox 0", columnType: "checkbox", input: "0", want: `{"checked":"false"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Encode(tt.columnType, tt.settings, tt.input)
			if err != nil {
				t.Fatalf("Encode(%q, %q) error: %v", tt.columnType, tt.input, err)
			}
			if string(got) != tt.want {
				t.Errorf("Encode(%q, %q) = %s, want %s", tt.columnType, tt.input, got, tt.want)
			}
			if !json.Valid(got) {
				t.Errorf("Encode(%q, %q) produced invalid JSON: %s", tt.columnType, tt.input, got)
			}
		})
	}
}

func TestEncode_usageErrors(t *testing.T) {
	tests := []struct {
		name       string
		columnType string
		settings   string
		input      string
	}{
		{name: "unknown column type", columnType: "mirror", input: "x"},
		{name: "status label not on column", columnType: "status", settings: statusSettingsObject, input: "Nearly Done"},
		{name: "status empty", columnType: "status", settings: statusSettingsObject, input: ""},
		{name: "date empty", columnType: "date", input: ""},
		{name: "date malformed", columnType: "date", input: "next tuesday"},
		{name: "date day-month order", columnType: "date", input: "10/05/2026"},
		{name: "numbers not numeric", columnType: "numbers", input: "many"},
		{name: "checkbox not boolean", columnType: "checkbox", input: "maybe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Encode(tt.columnType, tt.settings, tt.input)
			if err == nil {
				t.Fatalf("Encode(%q, %q) = %s, want an error", tt.columnType, tt.input, got)
			}
			// The caller maps these straight to an exit code; a mislabelled encoder
			// error would exit 5 (internal) on what is a caller mistake.
			if code := codeOf(err); code != errs.CodeUsage {
				t.Errorf("Encode(%q, %q) code = %s, want %s", tt.columnType, tt.input, code, errs.CodeUsage)
			}
		})
	}
}

// TestEncode_statusErrorListsLabels checks the error is actionable: an agent that
// guessed a label needs to see the real ones, not just "invalid".
func TestEncode_statusErrorListsLabels(t *testing.T) {
	_, err := Encode("status", statusSettingsObject, "Nearly Done")
	if err == nil {
		t.Fatal("want an error")
	}
	for _, label := range []string{"Not Started", "Working on it", "Done"} {
		if !strings.Contains(err.Error(), label) {
			t.Errorf("error %q does not name label %q", err.Error(), label)
		}
	}
}

// TestEncoders_HaveDecoders guards the invariant stated in encoders' doc comment: a
// type mcli can write is a type mcli can read back, so a round-trip cannot lose data.
func TestEncoders_HaveDecoders(t *testing.T) {
	for _, ct := range EncodableTypes() {
		if _, ok := registry[ct]; !ok {
			t.Errorf("column type %q has an encoder but no decoder", ct)
		}
	}
}

// TestStatusLabels covers settings_str shapes, including the ones we cannot parse —
// there, no labels means "write it unvalidated" rather than "reject everything".
func TestStatusLabels(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		want     []string
	}{
		{name: "empty", settings: "", want: nil},
		{name: "object form in index order", settings: statusSettingsObject, want: []string{"Not Started", "Working on it", "Done"}},
		{name: "array form", settings: statusSettingsArray, want: []string{"Open", "Closed"}},
		{name: "unparseable", settings: "not json", want: nil},
		{name: "no labels key", settings: `{"labels_colors":{}}`, want: nil},
		{name: "blank labels dropped", settings: `{"labels":{"0":"","1":"Done"}}`, want: []string{"Done"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusLabels(tt.settings)
			if len(got) != len(tt.want) {
				t.Fatalf("statusLabels(%q) = %v, want %v", tt.settings, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("statusLabels(%q) = %v, want %v", tt.settings, got, tt.want)
				}
			}
		})
	}
}

// TestEncode_statusRoundTripsThroughDecode is the practical form of the
// encoder/decoder pairing: what we write is what a later read reports.
func TestEncode_statusRoundTripsThroughDecode(t *testing.T) {
	raw, err := Encode("status", statusSettingsObject, "working on IT")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode("status", statusSettingsObject, string(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if decoded.Value != "Working on it" {
		t.Errorf("round trip = %v, want %q", decoded.Value, "Working on it")
	}
}
