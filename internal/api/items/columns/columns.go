// Package columns decodes monday.com column values from their raw JSON representation
// into clean, typed Go values suitable for JSON output.
//
// The write path is JSON-passthrough (handled elsewhere). This package is read-path only.
package columns

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mondaycom/mcli/internal/errs"
)

// Decoded is the result of Decode: a clean value plus the canonical type name.
type Decoded struct {
	Type  string `json:"type"`            // e.g. "status", "date", "text"
	Value any    `json:"value,omitempty"` // type-specific shape; see DecodeXxx funcs
}

// Person represents a single person or team in a people column value.
type Person struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// LinkVal holds a URL and its display text.
type LinkVal struct {
	URL  string `json:"url"`
	Text string `json:"text"`
}

// EmailVal holds an email address and its display label.
type EmailVal struct {
	Email string `json:"email"`
	Text  string `json:"text"`
}

// PhoneVal holds a phone number and country code.
type PhoneVal struct {
	Phone   string `json:"phone"`
	Country string `json:"country"`
}

// TimelineVal holds the from/to dates of a timeline column.
type TimelineVal struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// decoderFn is a function that decodes a raw JSON value string using optional settings.
type decoderFn func(settingsStr, valueJSON string) (any, error)

// registry maps monday column type strings to decoder functions.
var registry = map[string]decoderFn{
	"text":      decodeText,
	"long_text": decodeLongText,
	"status":    decodeStatus,
	"date":      decodeDate,
	"datetime":  decodeDate, // synthetic alias; real date-with-time arrives as "date"
	"people":    decodePeople,
	"dropdown":  decodeDropdown,
	"numbers":   decodeNumeric,
	"link":      decodeLink,
	"email":     decodeEmail,
	"phone":     decodePhone,
	"timeline":  decodeTimeline,
	"tags":      decodeTags,
	"checkbox":  decodeCheckbox,
}

// Decode renders a monday column value into a clean Decoded.
//
// columnType is the column's type as monday reports it (e.g. "status", "date").
// settingsStr is the column's settings_str (used to resolve labels for status/dropdown).
// valueJSON is the raw column-value JSON from monday's API ("" or "null" if no value set).
//
// For unknown column types, Decoded.Type is the raw type string and Decoded.Value is
// the raw JSON string (passthrough). No error is returned for unknown types.
func Decode(columnType, settingsStr, valueJSON string) (Decoded, error) {
	// Empty / null handling.
	if valueJSON == "" || valueJSON == "null" {
		return Decoded{Type: columnType}, nil
	}

	fn, ok := registry[columnType]
	if !ok {
		// Unknown type: passthrough raw JSON string.
		return Decoded{Type: columnType, Value: valueJSON}, nil
	}

	val, err := fn(settingsStr, valueJSON)
	if err != nil {
		return Decoded{}, err
	}
	return Decoded{Type: columnType, Value: val}, nil
}

// ---- per-type decoders ----

// decodeText handles "text" columns.
// Monday returns either a bare JSON string ("hello") or {"text":"hello"}.
func decodeText(_ string, valueJSON string) (any, error) {
	// Try bare string first.
	var s string
	if err := json.Unmarshal([]byte(valueJSON), &s); err == nil {
		return s, nil
	}
	// Try object with text field.
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("text column: malformed JSON: %v", err)
	}
	return obj.Text, nil
}

// decodeLongText handles "long-text" columns.
// Shape: {"text":"line1\nline2"}
func decodeLongText(_ string, valueJSON string) (any, error) {
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("long-text column: malformed JSON: %v", err)
	}
	return obj.Text, nil
}

// statusSettings is the shape of a status column's settings_str.
// Monday uses both "labels" as object ({"0":"Not started","1":"Done"}) and
// other formats. We support both map[string]string and the label_style object.
type statusSettings struct {
	Labels map[string]string `json:"labels"`
}

// decodeStatus handles "status" columns.
// Value shape: {"label":"Done","index":1,"post_id":null,"changed_at":"..."}
// Label comes directly from the value JSON; settings_str is a fallback.
func decodeStatus(settingsStr, valueJSON string) (any, error) {
	var obj struct {
		Label *string `json:"label"`
		Index *int    `json:"index"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("status column: malformed JSON: %v", err)
	}

	// Prefer label directly from value.
	if obj.Label != nil && *obj.Label != "" {
		return *obj.Label, nil
	}

	// Fall back to settings_str label resolution by index.
	if obj.Index != nil && settingsStr != "" {
		var settings statusSettings
		if err := json.Unmarshal([]byte(settingsStr), &settings); err == nil {
			key := strconv.Itoa(*obj.Index)
			if label, ok := settings.Labels[key]; ok && label != "" {
				return label, nil
			}
		}
	}

	// Nothing found: return empty string (blank status).
	return "", nil
}

// decodeDate handles both "date" and "datetime" columns.
// Value shape: {"date":"2026-05-10","time":"14:30:00"} or {"date":"2026-05-10","time":null}
// Returns "2026-05-10T14:30:00Z" when time is present and non-empty; "2026-05-10" otherwise.
func decodeDate(_ string, valueJSON string) (any, error) {
	var obj struct {
		Date string  `json:"date"`
		Time *string `json:"time"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("date column: malformed JSON: %v", err)
	}

	if obj.Date == "" {
		return "", nil
	}

	if obj.Time != nil && *obj.Time != "" {
		return fmt.Sprintf("%sT%sZ", obj.Date, *obj.Time), nil
	}
	return obj.Date, nil
}

// decodePeople handles "people" columns.
// Value shape: {"personsAndTeams":[{"id":50353184,"kind":"person"},{"id":42,"kind":"team"}]}
func decodePeople(_ string, valueJSON string) (any, error) {
	var obj struct {
		PersonsAndTeams []struct {
			ID   json.Number `json:"id"`
			Kind string      `json:"kind"`
		} `json:"personsAndTeams"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("people column: malformed JSON: %v", err)
	}
	people := make([]Person, 0, len(obj.PersonsAndTeams))
	for _, p := range obj.PersonsAndTeams {
		people = append(people, Person{ID: p.ID.String(), Kind: p.Kind})
	}
	return people, nil
}

// dropdownSettings covers the shape monday uses for dropdown column settings.
// Monday uses an array: {"labels":[{"id":1,"name":"Option A"},...]}
type dropdownSettings struct {
	Labels []struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	} `json:"labels"`
}

// decodeDropdown handles "dropdown" columns.
// Value shape: {"ids":[1,2,3]}; labels resolved from settings_str.
func decodeDropdown(settingsStr, valueJSON string) (any, error) {
	var obj struct {
		IDs []json.Number `json:"ids"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("dropdown column: malformed JSON: %v", err)
	}

	// Try to build id→label map from settings.
	idToLabel := make(map[string]string)
	if settingsStr != "" {
		var settings dropdownSettings
		if err := json.Unmarshal([]byte(settingsStr), &settings); err == nil {
			for _, l := range settings.Labels {
				idToLabel[l.ID.String()] = l.Name
			}
		}
	}

	labels := make([]string, 0, len(obj.IDs))
	for _, id := range obj.IDs {
		if label, ok := idToLabel[id.String()]; ok {
			labels = append(labels, label)
		} else {
			labels = append(labels, id.String())
		}
	}
	return labels, nil
}

// decodeNumeric handles "numeric" (numbers) columns.
// Monday returns a quoted number string e.g. "42.5" or "42".
func decodeNumeric(_ string, valueJSON string) (any, error) {
	// valueJSON may be a JSON string ("42.5") or a raw number (42.5).
	// Unmarshal as string first, then parse.
	var s string
	if err := json.Unmarshal([]byte(valueJSON), &s); err != nil {
		// Try raw number.
		var f float64
		if err2 := json.Unmarshal([]byte(valueJSON), &f); err2 != nil {
			return nil, errs.API("numeric column: malformed JSON: %v", err2)
		}
		return normaliseFloat(f), nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, errs.API("numeric column: cannot parse number %q: %v", s, err)
	}
	return normaliseFloat(f), nil
}

// normaliseFloat returns int64 when the float has no fractional part, float64 otherwise.
func normaliseFloat(f float64) any {
	if f == float64(int64(f)) {
		return int64(f)
	}
	return f
}

// decodeLink handles "link" columns.
// Value shape: {"url":"https://x.com","text":"Display"}
func decodeLink(_ string, valueJSON string) (any, error) {
	var obj struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("link column: malformed JSON: %v", err)
	}
	return LinkVal{URL: obj.URL, Text: obj.Text}, nil
}

// decodeEmail handles "email" columns.
// Value shape: {"email":"x@y.com","text":"Display"}
// Note: the schema calls the display field "label" but the raw value JSON uses "text".
func decodeEmail(_ string, valueJSON string) (any, error) {
	var obj struct {
		Email string `json:"email"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("email column: malformed JSON: %v", err)
	}
	return EmailVal{Email: obj.Email, Text: obj.Text}, nil
}

// decodePhone handles "phone" columns.
// Value shape: {"phone":"+1234567890","countryShortName":"US"}
func decodePhone(_ string, valueJSON string) (any, error) {
	var obj struct {
		Phone            string `json:"phone"`
		CountryShortName string `json:"countryShortName"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("phone column: malformed JSON: %v", err)
	}
	return PhoneVal{Phone: obj.Phone, Country: obj.CountryShortName}, nil
}

// decodeTimeline handles "timeline" columns.
// Value shape: {"from":"2026-05-01","to":"2026-05-15"}
func decodeTimeline(_ string, valueJSON string) (any, error) {
	var obj struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("timeline column: malformed JSON: %v", err)
	}
	return TimelineVal{From: obj.From, To: obj.To}, nil
}

// decodeTags handles "tags" columns.
// Value shape: {"tag_ids":[1,2,3]}
func decodeTags(_ string, valueJSON string) (any, error) {
	var obj struct {
		TagIDs []json.Number `json:"tag_ids"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("tags column: malformed JSON: %v", err)
	}
	ids := make([]string, 0, len(obj.TagIDs))
	for _, id := range obj.TagIDs {
		ids = append(ids, id.String())
	}
	return ids, nil
}

// decodeCheckbox handles "checkbox" columns.
// Value shape: {"checked":"true"} or {"checked":true}
func decodeCheckbox(_ string, valueJSON string) (any, error) {
	// checked field may be a boolean or a string.
	var obj struct {
		Checked json.RawMessage `json:"checked"`
	}
	if err := json.Unmarshal([]byte(valueJSON), &obj); err != nil {
		return nil, errs.API("checkbox column: malformed JSON: %v", err)
	}
	if len(obj.Checked) == 0 {
		return false, nil
	}
	// Try boolean.
	var b bool
	if err := json.Unmarshal(obj.Checked, &b); err == nil {
		return b, nil
	}
	// Try string.
	var s string
	if err := json.Unmarshal(obj.Checked, &s); err != nil {
		return nil, errs.API("checkbox column: cannot parse checked field: %v", err)
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true":
		return true, nil
	case "false", "":
		return false, nil
	default:
		return nil, errs.API("checkbox column: unexpected checked value %q", s)
	}
}
