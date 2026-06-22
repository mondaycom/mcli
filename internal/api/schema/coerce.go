package apischema

import "encoding/json"

// CoerceArgs inspects argDefs and for any arg where IsJSON==true, if the
// corresponding value in vars is a map[string]any or []any, json.Marshal it
// to a string. monday.com's JSON scalar expects a stringified JSON value on
// the wire.
func CoerceArgs(argDefs []ArgDef, vars map[string]any) {
	if len(vars) == 0 {
		return
	}
	for _, ad := range argDefs {
		if !ad.IsJSON {
			continue
		}
		v, ok := vars[ad.Name]
		if !ok || v == nil {
			continue
		}
		switch v.(type) {
		case map[string]any, []any:
			if b, err := json.Marshal(v); err == nil {
				vars[ad.Name] = string(b)
			}
		}
	}
}
