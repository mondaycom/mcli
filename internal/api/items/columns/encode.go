package columns

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mondaycom/mcli/internal/errs"
)

// encoderFn builds the monday wire JSON for one column type from a human-friendly
// input string. settingsStr is the column's settings_str, used for validation.
type encoderFn func(settingsStr, input string) (json.RawMessage, error)

// encoders maps monday column types to write-side encoders. Every key here must
// also have a read-side decoder in registry, so the two halves of a column type
// cannot drift apart (enforced by TestEncoders_HaveDecoders).
//
// This set is deliberately small: it covers the types whose wire shape is
// unambiguous from a single scalar. Everything else is written through the raw
// escape hatch, where the caller supplies the JSON.
var encoders = map[string]encoderFn{
	"text":     encodeText,
	"status":   encodeStatus,
	"date":     encodeDate,
	"numbers":  encodeNumbers,
	"checkbox": encodeCheckbox,
}

// EncodableTypes returns the column types Encode supports, sorted.
func EncodableTypes() []string {
	out := make([]string, 0, len(encoders))
	for t := range encoders {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Encode builds the monday wire JSON for a column write from a human-friendly value.
//
// columnType is the column's type as monday reports it; settingsStr is the column's
// settings_str; input is the caller's value (e.g. "Done", "2026-05-10", "true").
//
// Validation is the point, not a bonus: every mcli item mutation sends
// create_labels_if_missing, so an unvalidated status typo silently creates a new
// label on the board instead of failing. Encode rejects values the column cannot
// hold, with errs.Usage, before any request is built.
func Encode(columnType, settingsStr, input string) (json.RawMessage, error) {
	fn, ok := encoders[columnType]
	if !ok {
		return nil, errs.Usage("no shorthand for column type %q: use --col <id>=<json>", columnType)
	}
	return fn(settingsStr, input)
}

// encodeText writes a "text" column. Wire shape: a bare JSON string.
func encodeText(_ string, input string) (json.RawMessage, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return nil, errs.Usage("text: %v", err)
	}
	return b, nil
}

// encodeStatus writes a "status" column. Wire shape: {"label":"Done"}.
// The label is matched against the column's configured labels — exactly first, then
// case-insensitively, adopting the board's own casing on a case-insensitive hit.
func encodeStatus(settingsStr, input string) (json.RawMessage, error) {
	label := strings.TrimSpace(input)
	if label == "" {
		return nil, errs.Usage("status: label must not be empty")
	}

	// An unparseable or absent settings_str means we cannot validate. Write anyway
	// rather than blocking the caller on metadata we failed to read.
	if labels := statusLabels(settingsStr); len(labels) > 0 {
		match, ok := matchLabel(labels, label)
		if !ok {
			return nil, errs.Usage("status: %q is not a label on this column; available: %s",
				label, strings.Join(labels, ", "))
		}
		label = match
	}

	return json.Marshal(map[string]string{"label": label})
}

// statusLabels returns a status column's labels in index order. It handles both
// shapes monday uses: the index→name object and the dropdown-style array.
func statusLabels(settingsStr string) []string {
	if settingsStr == "" {
		return nil
	}

	var asObject struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(settingsStr), &asObject); err == nil && len(asObject.Labels) > 0 {
		keys := make([]string, 0, len(asObject.Labels))
		for k := range asObject.Labels {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			ni, erri := strconv.Atoi(keys[i])
			nj, errj := strconv.Atoi(keys[j])
			if erri == nil && errj == nil {
				return ni < nj
			}
			return keys[i] < keys[j]
		})
		out := make([]string, 0, len(keys))
		for _, k := range keys {
			if name := asObject.Labels[k]; name != "" {
				out = append(out, name)
			}
		}
		return out
	}

	var asArray dropdownSettings
	if err := json.Unmarshal([]byte(settingsStr), &asArray); err == nil {
		out := make([]string, 0, len(asArray.Labels))
		for _, l := range asArray.Labels {
			if l.Name != "" {
				out = append(out, l.Name)
			}
		}
		return out
	}

	return nil
}

// matchLabel finds want among labels, preferring an exact match and falling back to
// a case-insensitive one. It returns the board's spelling, not the caller's.
func matchLabel(labels []string, want string) (string, bool) {
	for _, l := range labels {
		if l == want {
			return l, true
		}
	}
	for _, l := range labels {
		if strings.EqualFold(l, want) {
			return l, true
		}
	}
	return "", false
}

// dateLayouts are the input forms encodeDate accepts, most specific first.
var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

// encodeDate writes a "date" column. Wire shape: {"date":"2026-05-10"}, plus
// {"time":"14:30:00"} when the input carries a time. monday stores date-column
// times in UTC, so a zoned input is converted.
func encodeDate(_ string, input string) (json.RawMessage, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, errs.Usage("date: value must not be empty")
	}

	for _, layout := range dateLayouts {
		t, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		if layout == "2006-01-02" {
			return json.Marshal(map[string]string{"date": t.Format("2006-01-02")})
		}
		t = t.UTC()
		return json.Marshal(map[string]string{
			"date": t.Format("2006-01-02"),
			"time": t.Format("15:04:05"),
		})
	}

	return nil, errs.Usage("date: %q is not a date; use 2026-05-10, 2026-05-10T14:30, or an RFC3339 timestamp", s)
}

// encodeNumbers writes a "numbers" column. Wire shape: a JSON string holding the
// number, e.g. "42". The caller's own formatting is preserved once it parses.
func encodeNumbers(_ string, input string) (json.RawMessage, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		// An empty numbers column is written as an empty string, which clears it.
		return json.RawMessage(`""`), nil
	}
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return nil, errs.Usage("number: %q is not numeric", s)
	}
	return json.Marshal(s)
}

// encodeCheckbox writes a "checkbox" column. Wire shape: {"checked":"true"}.
func encodeCheckbox(_ string, input string) (json.RawMessage, error) {
	s := strings.ToLower(strings.TrimSpace(input))
	switch s {
	case "yes", "y":
		s = "true"
	case "no", "n":
		s = "false"
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return nil, errs.Usage("checkbox: %q is not a boolean; use true or false", input)
	}
	return json.Marshal(map[string]string{"checked": strconv.FormatBool(b)})
}
