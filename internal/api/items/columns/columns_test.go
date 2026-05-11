package columns_test

import (
	"testing"

	"github.com/mondaycom/mcli/internal/api/items/columns"
)

// helper asserting no error and matching expected value.
func mustDecode(t *testing.T, columnType, settingsStr, valueJSON string) columns.Decoded {
	t.Helper()
	d, err := columns.Decode(columnType, settingsStr, valueJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return d
}

// ---- text ----

func TestText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      any
	}{
		{"bare string", `"hello world"`, "hello world"},
		{"object form", `{"text":"hello object"}`, "hello object"},
		{"empty bare string", `""`, ""},
		{"empty object", `{"text":""}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "text", "", tc.valueJSON)
			if d.Type != "text" {
				t.Errorf("Type = %q, want %q", d.Type, "text")
			}
			if d.Value != tc.want {
				t.Errorf("Value = %q, want %q", d.Value, tc.want)
			}
		})
	}
}

func TestText_NullEmpty(t *testing.T) {
	t.Parallel()
	for _, vj := range []string{"", "null"} {
		d := mustDecode(t, "text", "", vj)
		if d.Value != nil {
			t.Errorf("valueJSON=%q: Value = %v, want nil", vj, d.Value)
		}
	}
}

func TestText_MalformedJSON(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("text", "", `{"text": BROKEN`)
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

// ---- long-text ----

func TestLongText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      string
	}{
		{"multiline", `{"text":"line1\nline2"}`, "line1\nline2"},
		{"empty", `{"text":""}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "long-text", "", tc.valueJSON)
			if d.Value != tc.want {
				t.Errorf("Value = %q, want %q", d.Value, tc.want)
			}
		})
	}
}

func TestLongText_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "long-text", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestLongText_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("long-text", "", `{bad json}`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- status ----

const statusSettings = `{"labels":{"0":"Not started","1":"In progress","2":"Done"}}`

func TestStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		settingsStr string
		valueJSON   string
		want        string
	}{
		{
			"label from value",
			statusSettings,
			`{"label":"Done","index":2,"post_id":null,"changed_at":"2026-05-10T12:00:00Z"}`,
			"Done",
		},
		{
			"label from settings fallback",
			statusSettings,
			`{"label":"","index":1}`,
			"In progress",
		},
		{
			"no label no settings",
			"",
			`{"label":"","index":0}`,
			"",
		},
		{
			"label override ignores settings",
			statusSettings,
			`{"label":"Custom","index":0}`,
			"Custom",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "status", tc.settingsStr, tc.valueJSON)
			if d.Value != tc.want {
				t.Errorf("Value = %q, want %q", d.Value, tc.want)
			}
		})
	}
}

func TestStatus_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "status", statusSettings, "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestStatus_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("status", "", `{broken`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- date ----

func TestDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      string
	}{
		{"date only", `{"date":"2026-05-10","time":null,"icon":null}`, "2026-05-10"},
		{"date and time", `{"date":"2026-05-10","time":"14:30:00"}`, "2026-05-10T14:30:00Z"},
		{"empty date", `{"date":"","time":null}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "date", "", tc.valueJSON)
			if d.Value != tc.want {
				t.Errorf("Value = %q, want %q", d.Value, tc.want)
			}
		})
	}
}

func TestDate_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "date", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestDate_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("date", "", `{"date": BROKEN}`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- datetime ----

func TestDatetime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      string
	}{
		{"with time", `{"date":"2026-05-10","time":"14:30:00"}`, "2026-05-10T14:30:00Z"},
		{"date only", `{"date":"2026-05-10","time":null}`, "2026-05-10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "datetime", "", tc.valueJSON)
			if d.Value != tc.want {
				t.Errorf("Value = %q, want %q", d.Value, tc.want)
			}
		})
	}
}

func TestDatetime_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "datetime", "", "")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

// ---- people ----

func TestPeople(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      []columns.Person
	}{
		{
			"person and team",
			`{"personsAndTeams":[{"id":50353184,"kind":"person"},{"id":42,"kind":"team"}]}`,
			[]columns.Person{
				{ID: "50353184", Kind: "person"},
				{ID: "42", Kind: "team"},
			},
		},
		{
			"empty list",
			`{"personsAndTeams":[]}`,
			[]columns.Person{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "people", "", tc.valueJSON)
			got, ok := d.Value.([]columns.Person)
			if !ok {
				t.Fatalf("Value is %T, want []columns.Person", d.Value)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tc.want), got)
			}
			for i, p := range got {
				if p != tc.want[i] {
					t.Errorf("[%d] got %+v, want %+v", i, p, tc.want[i])
				}
			}
		})
	}
}

func TestPeople_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "people", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestPeople_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("people", "", `{"personsAndTeams": BROKEN}`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- dropdown ----

const dropdownSettings = `{"labels":[{"id":1,"name":"Option A"},{"id":2,"name":"Option B"},{"id":3,"name":"Option C"}]}`

func TestDropdown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		settingsStr string
		valueJSON   string
		want        []string
	}{
		{
			"labels resolved",
			dropdownSettings,
			`{"ids":[1,2]}`,
			[]string{"Option A", "Option B"},
		},
		{
			"missing label falls back to id",
			dropdownSettings,
			`{"ids":[99]}`,
			[]string{"99"},
		},
		{
			"no settings returns ids",
			"",
			`{"ids":[1,2,3]}`,
			[]string{"1", "2", "3"},
		},
		{
			"empty ids",
			dropdownSettings,
			`{"ids":[]}`,
			[]string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "dropdown", tc.settingsStr, tc.valueJSON)
			got, ok := d.Value.([]string)
			if !ok {
				t.Fatalf("Value is %T, want []string", d.Value)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tc.want), got)
			}
			for i, v := range got {
				if v != tc.want[i] {
					t.Errorf("[%d] got %q, want %q", i, v, tc.want[i])
				}
			}
		})
	}
}

func TestDropdown_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "dropdown", dropdownSettings, "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestDropdown_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("dropdown", "", `{"ids": BROKEN}`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- numeric ----

func TestNumeric(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      any
	}{
		{"integer string", `"42"`, int64(42)},
		{"float string", `"42.5"`, 42.5},
		{"zero", `"0"`, int64(0)},
		{"negative", `"-7"`, int64(-7)},
		{"raw float", `3.14`, 3.14},
		{"raw integer", `100`, int64(100)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "numeric", "", tc.valueJSON)
			if d.Value != tc.want {
				t.Errorf("Value = %v (%T), want %v (%T)", d.Value, d.Value, tc.want, tc.want)
			}
		})
	}
}

func TestNumeric_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "numeric", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestNumeric_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("numeric", "", `"not-a-number"`)
	if err == nil {
		t.Error("expected error for non-numeric string")
	}
}

// ---- link ----

func TestLink(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      columns.LinkVal
	}{
		{
			"with text",
			`{"url":"https://example.com","text":"Example"}`,
			columns.LinkVal{URL: "https://example.com", Text: "Example"},
		},
		{
			"no text",
			`{"url":"https://example.com","text":""}`,
			columns.LinkVal{URL: "https://example.com", Text: ""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "link", "", tc.valueJSON)
			got, ok := d.Value.(columns.LinkVal)
			if !ok {
				t.Fatalf("Value is %T, want LinkVal", d.Value)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLink_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "link", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestLink_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("link", "", `{bad`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- email ----

func TestEmail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      columns.EmailVal
	}{
		{
			"with text",
			`{"email":"x@y.com","text":"Display"}`,
			columns.EmailVal{Email: "x@y.com", Text: "Display"},
		},
		{
			"no text",
			`{"email":"x@y.com","text":""}`,
			columns.EmailVal{Email: "x@y.com", Text: ""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "email", "", tc.valueJSON)
			got, ok := d.Value.(columns.EmailVal)
			if !ok {
				t.Fatalf("Value is %T, want EmailVal", d.Value)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestEmail_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "email", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestEmail_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("email", "", `{bad}`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- phone ----

func TestPhone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      columns.PhoneVal
	}{
		{
			"US number",
			`{"phone":"+12025551234","countryShortName":"US"}`,
			columns.PhoneVal{Phone: "+12025551234", Country: "US"},
		},
		{
			"no country",
			`{"phone":"+442012345678","countryShortName":""}`,
			columns.PhoneVal{Phone: "+442012345678", Country: ""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "phone", "", tc.valueJSON)
			got, ok := d.Value.(columns.PhoneVal)
			if !ok {
				t.Fatalf("Value is %T, want PhoneVal", d.Value)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPhone_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "phone", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestPhone_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("phone", "", `{bad`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- timeline ----

func TestTimeline(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      columns.TimelineVal
	}{
		{
			"full range",
			`{"from":"2026-05-01","to":"2026-05-15"}`,
			columns.TimelineVal{From: "2026-05-01", To: "2026-05-15"},
		},
		{
			"empty range",
			`{"from":"","to":""}`,
			columns.TimelineVal{From: "", To: ""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "timeline", "", tc.valueJSON)
			got, ok := d.Value.(columns.TimelineVal)
			if !ok {
				t.Fatalf("Value is %T, want TimelineVal", d.Value)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTimeline_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "timeline", "", "")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestTimeline_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("timeline", "", `{bad`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- tags ----

func TestTags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      []string
	}{
		{"three tags", `{"tag_ids":[1,2,3]}`, []string{"1", "2", "3"}},
		{"one tag", `{"tag_ids":[42]}`, []string{"42"}},
		{"empty", `{"tag_ids":[]}`, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "tags", "", tc.valueJSON)
			got, ok := d.Value.([]string)
			if !ok {
				t.Fatalf("Value is %T, want []string", d.Value)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tc.want), got)
			}
			for i, v := range got {
				if v != tc.want[i] {
					t.Errorf("[%d] got %q, want %q", i, v, tc.want[i])
				}
			}
		})
	}
}

func TestTags_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "tags", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestTags_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("tags", "", `{"tag_ids": [BROKEN]}`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- checkbox ----

func TestCheckbox(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		valueJSON string
		want      bool
	}{
		{"string true", `{"checked":"true"}`, true},
		{"string false", `{"checked":"false"}`, false},
		{"bool true", `{"checked":true}`, true},
		{"bool false", `{"checked":false}`, false},
		{"empty string", `{"checked":""}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecode(t, "checkbox", "", tc.valueJSON)
			got, ok := d.Value.(bool)
			if !ok {
				t.Fatalf("Value is %T, want bool", d.Value)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckbox_Null(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "checkbox", "", "null")
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

func TestCheckbox_Malformed(t *testing.T) {
	t.Parallel()
	_, err := columns.Decode("checkbox", "", `{bad`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---- unknown type passthrough ----

func TestUnknownTypePassthrough(t *testing.T) {
	t.Parallel()
	const raw = `{"some":"proprietary","structure":42}`
	d, err := columns.Decode("mirror", "", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Type != "mirror" {
		t.Errorf("Type = %q, want %q", d.Type, "mirror")
	}
	if d.Value != raw {
		t.Errorf("Value = %v, want %q", d.Value, raw)
	}
}

func TestUnknownTypeNullValue(t *testing.T) {
	t.Parallel()
	d := mustDecode(t, "mirror", "", "null")
	if d.Type != "mirror" {
		t.Errorf("Type = %q, want %q", d.Type, "mirror")
	}
	// null short-circuits before passthrough; Value should be nil.
	if d.Value != nil {
		t.Errorf("Value = %v, want nil", d.Value)
	}
}

// ---- null / empty across representative types ----

func TestNullAndEmptyAcrossTypes(t *testing.T) {
	t.Parallel()
	types := []string{"text", "long-text", "status", "date", "datetime",
		"people", "dropdown", "numeric", "link", "email",
		"phone", "timeline", "tags", "checkbox"}
	for _, ct := range types {
		for _, vj := range []string{"", "null"} {
			t.Run(ct+"/"+vj, func(t *testing.T) {
				d, err := columns.Decode(ct, "", vj)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if d.Type != ct {
					t.Errorf("Type = %q, want %q", d.Type, ct)
				}
				if d.Value != nil {
					t.Errorf("Value = %v, want nil", d.Value)
				}
			})
		}
	}
}
