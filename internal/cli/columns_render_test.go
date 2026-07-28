package cli

import (
	"testing"

	"github.com/mondaycom/mcli/internal/api/items/columns"
)

func TestFormatColumnValue(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want string
	}{
		{"nil", nil, ""},
		{"string", "Done", "Done"},
		{"int", int64(8000), "8000"},
		{"float", 3.5, "3.5"},
		{"bool", false, "false"},
		{"timeline_both", columns.TimelineVal{From: "2026-07-10", To: "2026-07-20"}, "2026-07-10→2026-07-20"},
		{"timeline_from_only", columns.TimelineVal{From: "2026-07-10"}, "2026-07-10"},
		{"timeline_to_only", columns.TimelineVal{To: "2026-07-20"}, "2026-07-20"},
		{"link_text", columns.LinkVal{URL: "https://x", Text: "Home"}, "Home"},
		{"link_url_only", columns.LinkVal{URL: "https://x"}, "https://x"},
		{"email", columns.EmailVal{Email: "a@b.com", Text: "A"}, "a@b.com"},
		{"phone_with_country", columns.PhoneVal{Phone: "+123", Country: "US"}, "+123 (US)"},
		{"phone_no_country", columns.PhoneVal{Phone: "+123"}, "+123"},
		{"strings", []string{"Marketing", "Dev"}, "Marketing, Dev"},
		{"people", []columns.Person{{ID: "1", Kind: "person"}, {ID: "2", Kind: "team"}}, "person:1, team:2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatColumnValue(renderedColumn{Value: tc.val})
			if got != tc.want {
				t.Errorf("formatColumnValue(%v) = %q, want %q", tc.val, got, tc.want)
			}
		})
	}
}

func TestColumnLabel(t *testing.T) {
	if got := columnLabel(renderedColumn{ID: "color_x", Title: "Status"}); got != "Status" {
		t.Errorf("with title: got %q, want %q", got, "Status")
	}
	if got := columnLabel(renderedColumn{ID: "color_x"}); got != "color_x" {
		t.Errorf("without title: got %q, want %q", got, "color_x")
	}
}
