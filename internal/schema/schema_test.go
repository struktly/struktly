package schema

import (
	"strings"
	"testing"
)

func TestValidateAcceptsAConformingDocument(t *testing.T) {
	const doc = `{"schema":"struktly/x/v1","count":2,"tags":["a"],"kind":"one","when":"2026-08-08"}`
	if err := ValidateJSON([]byte(sampleSchema), []byte(doc)); err != nil {
		t.Fatalf("conforming document rejected: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	for name, test := range map[string]struct{ doc, want string }{
		"wrong const":        {`{"schema":"other","count":2,"tags":[],"kind":"one","when":"2026-08-08"}`, "does not equal"},
		"value outside enum": {`{"schema":"struktly/x/v1","count":2,"tags":[],"kind":"three","when":"2026-08-08"}`, "is not one of"},
		"missing required":   {`{"count":2,"tags":[],"kind":"one","when":"2026-08-08"}`, `required property "schema" is missing`},
		"wrong type":         {`{"schema":"struktly/x/v1","count":"two","tags":[],"kind":"one","when":"2026-08-08"}`, "expected integer"},
		"below minimum":      {`{"schema":"struktly/x/v1","count":-1,"tags":[],"kind":"one","when":"2026-08-08"}`, "below the minimum"},
		"bad array element":  {`{"schema":"struktly/x/v1","count":2,"tags":[7],"kind":"one","when":"2026-08-08"}`, "tags[0]: expected string"},
		"bad date":           {`{"schema":"struktly/x/v1","count":2,"tags":[],"kind":"one","when":"08/08/2026"}`, "is not a date"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateJSON([]byte(sampleSchema), []byte(test.doc))
			if err == nil {
				t.Fatal("non-conforming document accepted")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not contain %q", err, test.want)
			}
		})
	}
}

// The guard that makes this checker worth trusting. A validator that ignores
// what it does not implement reports success for documents it never examined,
// which is worse than having no validator at all.
func TestValidateRefusesKeywordsItDoesNotImplement(t *testing.T) {
	const schema = `{"type":"object","properties":{"a":{"type":"number","multipleOf":2}}}`
	err := ValidateJSON([]byte(schema), []byte(`{"a":3}`))
	if err == nil {
		t.Fatal("an unimplemented keyword was silently ignored")
	}
	if !strings.Contains(err.Error(), "multipleOf") {
		t.Fatalf("error does not name the keyword: %v", err)
	}
}

func TestValidateResolvesLocalRefs(t *testing.T) {
	const schema = `{
      "type":"object",
      "properties":{"item":{"$ref":"#/$defs/item"}},
      "$defs":{"item":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}
    }`
	if err := ValidateJSON([]byte(schema), []byte(`{"item":{"id":"x"}}`)); err != nil {
		t.Fatalf("valid $ref document rejected: %v", err)
	}
	err := ValidateJSON([]byte(schema), []byte(`{"item":{}}`))
	if err == nil || !strings.Contains(err.Error(), `item: required property "id"`) {
		t.Fatalf("$ref constraint not enforced: %v", err)
	}
}

func TestValidateRejectsRemoteRefs(t *testing.T) {
	const schema = `{"type":"object","properties":{"a":{"$ref":"https://example.invalid/s.json"}}}`
	err := ValidateJSON([]byte(schema), []byte(`{"a":1}`))
	if err == nil || !strings.Contains(err.Error(), "only local") {
		t.Fatalf("a remote $ref was not refused: %v", err)
	}
}

const sampleSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "struktly/x/v1",
  "title": "Sample",
  "type": "object",
  "properties": {
    "schema": { "const": "struktly/x/v1" },
    "count": { "type": "integer", "minimum": 0 },
    "tags": { "type": "array", "items": { "type": "string" } },
    "kind": { "enum": ["one", "two"] },
    "when": { "type": "string", "format": "date" }
  },
  "required": ["schema", "count", "tags", "kind", "when"],
  "additionalProperties": true
}`
