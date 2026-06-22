package apischema

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Parallel()
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Query == nil {
		t.Fatal("Query type is nil after load")
	}
}

func TestQueryFields(t *testing.T) {
	t.Parallel()
	fields, err := QueryFields()
	if err != nil {
		t.Fatalf("QueryFields: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("expected non-empty query fields")
	}
	for _, f := range fields {
		if f.Name == "" {
			t.Error("found query field with empty Name")
		}
		if !f.IsQuery {
			t.Errorf("field %q: IsQuery should be true", f.Name)
		}
	}
}

func TestMutationFields(t *testing.T) {
	t.Parallel()
	fields, err := MutationFields()
	if err != nil {
		t.Fatalf("MutationFields: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("expected non-empty mutation fields")
	}
	for _, f := range fields {
		if f.Name == "" {
			t.Error("found mutation field with empty Name")
		}
		if f.IsQuery {
			t.Errorf("field %q: IsQuery should be false for mutations", f.Name)
		}
	}
}

func TestIsJSONScalar(t *testing.T) {
	t.Parallel()
	fields, err := MutationFields()
	if err != nil {
		t.Fatalf("MutationFields: %v", err)
	}

	var found bool
	for _, f := range fields {
		if f.Name != "change_column_value" {
			continue
		}
		for _, a := range f.Args {
			if a.Name == "value" {
				found = true
				if !a.IsJSON {
					t.Errorf("change_column_value.value: expected IsJSON=true, got false (TypeString=%q)", a.TypeString)
				}
			}
		}
	}
	if !found {
		t.Fatal("change_column_value mutation or its 'value' arg not found")
	}
}

func TestDefaultSelection_depth1(t *testing.T) {
	t.Parallel()
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sel := DefaultSelection("Board", s, 1)
	if !strings.Contains(sel, "id") {
		t.Errorf("depth-1 Board selection missing 'id': %s", sel)
	}
	if !strings.Contains(sel, "name") {
		t.Errorf("depth-1 Board selection missing 'name': %s", sel)
	}
}

func TestCoerceArgs(t *testing.T) {
	t.Parallel()

	argDefs := []ArgDef{
		{Name: "board_id", IsJSON: false},
		{Name: "value", IsJSON: true},
	}

	t.Run("map_gets_marshalled", func(t *testing.T) {
		vars := map[string]any{
			"board_id": "123",
			"value":    map[string]any{"label": "Done"},
		}
		CoerceArgs(argDefs, vars)
		s, ok := vars["value"].(string)
		if !ok {
			t.Fatalf("expected string, got %T: %v", vars["value"], vars["value"])
		}
		if s != `{"label":"Done"}` {
			t.Errorf("value = %q, want {\"label\":\"Done\"}", s)
		}
		if vars["board_id"] != "123" {
			t.Errorf("board_id was modified: %v", vars["board_id"])
		}
	})

	t.Run("string_left_alone", func(t *testing.T) {
		vars := map[string]any{
			"value": `{"already":"string"}`,
		}
		CoerceArgs(argDefs, vars)
		if vars["value"] != `{"already":"string"}` {
			t.Errorf("string value was modified: %v", vars["value"])
		}
	})
}
