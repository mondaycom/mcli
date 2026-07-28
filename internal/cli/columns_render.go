package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/mondaycom/mcli/internal/api/items/columns"
)

// formatColumnValue renders a decoded column value as a compact, human-readable
// string for text output modes (pretty/terse/csv). JSON output uses the
// structured value directly and never passes through here.
func formatColumnValue(rc renderedColumn) string {
	switch v := rc.Value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case columns.TimelineVal:
		return formatTimeline(v)
	case columns.LinkVal:
		if v.Text != "" {
			return v.Text
		}
		return v.URL
	case columns.EmailVal:
		if v.Email != "" {
			return v.Email
		}
		return v.Text
	case columns.PhoneVal:
		if v.Country != "" && v.Phone != "" {
			return v.Phone + " (" + v.Country + ")"
		}
		return v.Phone
	case []string:
		return strings.Join(v, ", ")
	case []columns.Person:
		parts := make([]string, len(v))
		for i, p := range v {
			parts[i] = p.Kind + ":" + p.ID
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatTimeline renders a timeline as "from→to", or a single endpoint when
// only one side is set.
func formatTimeline(t columns.TimelineVal) string {
	switch {
	case t.From != "" && t.To != "":
		return t.From + "→" + t.To
	case t.From != "":
		return t.From
	default:
		return t.To
	}
}

// columnLabel returns the human label for a column: its title, falling back to
// its id when the title is empty.
func columnLabel(rc renderedColumn) string {
	if rc.Title != "" {
		return rc.Title
	}
	return rc.ID
}

// itemColumnHeader identifies one column in a dynamic item table.
type itemColumnHeader struct {
	id    string
	label string
}

// collectItemColumns returns the columns present across items in first-seen
// order, so a board's items render as a uniform table even when individual
// items omit an (empty) column value.
func collectItemColumns(items []itemListOutputItem) []itemColumnHeader {
	seen := make(map[string]bool)
	var headers []itemColumnHeader
	for _, it := range items {
		for _, c := range it.Columns {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			headers = append(headers, itemColumnHeader{id: c.ID, label: columnLabel(c)})
		}
	}
	return headers
}

// itemColumnValues maps column id → formatted value for a single item.
func itemColumnValues(it itemListOutputItem) map[string]string {
	m := make(map[string]string, len(it.Columns))
	for _, c := range it.Columns {
		m[c.ID] = formatColumnValue(c)
	}
	return m
}

// writeItemsTable writes a tab-aligned table of items with one table column per
// board column (values formatted, blank when unset). It flushes its own
// tabwriter; callers own any surrounding text (headers, cursor lines).
func writeItemsTable(w io.Writer, items []itemListOutputItem) error {
	headers := collectItemColumns(items)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	cols := []string{"ID", "NAME", "STATE", "GROUP"}
	for _, h := range headers {
		cols = append(cols, h.label)
	}
	_, _ = fmt.Fprintln(tw, strings.Join(cols, "\t"))

	for _, it := range items {
		vals := itemColumnValues(it)
		row := []string{it.ID, it.Name, it.State, it.Group.Title}
		for _, h := range headers {
			row = append(row, vals[h.id])
		}
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}
