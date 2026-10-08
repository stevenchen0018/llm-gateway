package docs

import (
	"strings"
	"testing"
)

// Every $ref in the spec must resolve, otherwise docs pages and SDK
// generators break on an otherwise valid-looking document.
func TestSpecRefsResolve(t *testing.T) {
	comps := parsed["components"].(map[string]any)
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if ref, ok := x["$ref"].(string); ok {
				var section, name string
				if !splitRef(ref, &section, &name) {
					t.Errorf("unexpected $ref format %q", ref)
					return
				}
				sec, _ := comps[section].(map[string]any)
				if _, ok := sec[name]; !ok {
					t.Errorf("unresolved $ref %q", ref)
				}
			}
			for _, vv := range x {
				walk(vv)
			}
		case []any:
			for _, vv := range x {
				walk(vv)
			}
		}
	}
	walk(parsed)
	for _, p := range []string{"/chat/completions", "/completions", "/embeddings", "/models"} {
		if _, ok := parsed["paths"].(map[string]any)[p]; !ok {
			t.Errorf("gateway endpoint %s is not documented", p)
		}
	}
}

// splitRef splits "#/components/<section>/<name>".
func splitRef(ref string, section, name *string) bool {
	if !strings.HasPrefix(ref, "#/components/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(ref, "#/components/"), "/")
	if len(parts) != 2 {
		return false
	}
	*section, *name = parts[0], parts[1]
	return true
}
