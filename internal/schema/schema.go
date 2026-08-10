// Package schema checks documents against the JSON Schema subset the files in
// schemas/ actually use.
//
// It exists because the Go tests could compare schema field names but could not
// enforce the schemas, and no JSON Schema library is worth a dependency in a
// module that carries one. The gap was not theoretical: `provenance.confidence`
// shipped a `declared` that the enum did not list, and only a hand-run
// round-trip through a Python validator caught it.
//
// This is deliberately a subset, and it refuses every keyword it does not
// implement rather than ignoring it. A checker that silently skips what it does
// not understand is worse than no checker, because it reports success for
// documents it never examined.
package schema

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// annotations carry no constraint and are recognised so they do not trip the
// unknown-keyword guard.
var annotations = map[string]struct{}{
	"$id": {}, "$schema": {}, "title": {}, "description": {}, "$comment": {},
	"examples": {}, "default": {}, "deprecated": {},
}

// constraints are the keywords this package enforces.
var constraints = map[string]struct{}{
	"type": {}, "enum": {}, "const": {}, "required": {}, "properties": {},
	"items": {}, "$ref": {}, "$defs": {}, "additionalProperties": {},
	"minimum": {}, "minLength": {}, "pattern": {}, "format": {},
}

type validator struct {
	root map[string]any
}

// Validate reports whether document conforms to schema. Both are values decoded
// from JSON: map[string]any, []any, string, float64, bool or nil.
func Validate(schema, document any) error {
	root, ok := schema.(map[string]any)
	if !ok {
		return fmt.Errorf("schema must be an object, got %T", schema)
	}
	v := validator{root: root}
	return v.check("", root, document)
}

// ValidateJSON is Validate over raw JSON bytes.
func ValidateJSON(schema, document []byte) error {
	var s, d any
	if err := json.Unmarshal(schema, &s); err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}
	if err := json.Unmarshal(document, &d); err != nil {
		return fmt.Errorf("parse document: %w", err)
	}
	return Validate(s, d)
}

func (v validator) check(path string, schema map[string]any, value any) error {
	if ref, ok := schema["$ref"]; ok {
		resolved, err := v.resolve(ref)
		if err != nil {
			return at(path, err)
		}
		return v.check(path, resolved, value)
	}
	for keyword := range schema {
		if _, ok := annotations[keyword]; ok {
			continue
		}
		if _, ok := constraints[keyword]; !ok {
			// Refusing here is the point: an unimplemented keyword must stop the
			// run rather than be waved through as conformance.
			return at(path, fmt.Errorf("schema uses unsupported keyword %q; teach internal/schema or simplify the schema", keyword))
		}
	}

	for _, rule := range []func(string, map[string]any, any) error{
		v.checkType, v.checkEnum, v.checkConst, v.checkFormat,
		v.checkNumber, v.checkString, v.checkObject, v.checkArray,
	} {
		if err := rule(path, schema, value); err != nil {
			return err
		}
	}
	return nil
}

func (v validator) resolve(ref any) (map[string]any, error) {
	pointer, ok := ref.(string)
	if !ok {
		return nil, fmt.Errorf("$ref must be a string, got %T", ref)
	}
	name, ok := strings.CutPrefix(pointer, "#/$defs/")
	if !ok {
		return nil, fmt.Errorf("only local #/$defs/ references are supported, got %q", pointer)
	}
	defs, ok := v.root["$defs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("$ref %q but the schema declares no $defs", pointer)
	}
	target, ok := defs[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("$ref %q does not resolve", pointer)
	}
	return target, nil
}

func (v validator) checkType(path string, schema map[string]any, value any) error {
	declared, ok := schema["type"]
	if !ok {
		return nil
	}
	name, ok := declared.(string)
	if !ok {
		return at(path, fmt.Errorf("type must be a string, got %T", declared))
	}
	if matchesType(name, value) {
		return nil
	}
	return at(path, fmt.Errorf("expected %s, got %s", name, jsonType(value)))
}

func matchesType(name string, value any) bool {
	switch name {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == math.Trunc(number)
	case "null":
		return value == nil
	default:
		return false
	}
}

func (v validator) checkEnum(path string, schema map[string]any, value any) error {
	declared, ok := schema["enum"]
	if !ok {
		return nil
	}
	allowed, ok := declared.([]any)
	if !ok {
		return at(path, fmt.Errorf("enum must be an array, got %T", declared))
	}
	for _, candidate := range allowed {
		if equalJSON(candidate, value) {
			return nil
		}
	}
	return at(path, fmt.Errorf("%v is not one of %v", value, allowed))
}

func (v validator) checkConst(path string, schema map[string]any, value any) error {
	declared, ok := schema["const"]
	if !ok {
		return nil
	}
	if !equalJSON(declared, value) {
		return at(path, fmt.Errorf("%v does not equal the required %v", value, declared))
	}
	return nil
}

func (v validator) checkFormat(path string, schema map[string]any, value any) error {
	declared, ok := schema["format"]
	if !ok {
		return nil
	}
	name, _ := declared.(string)
	text, isText := value.(string)
	if !isText {
		return nil
	}
	switch name {
	case "date":
		if _, err := time.Parse(time.DateOnly, text); err != nil {
			return at(path, fmt.Errorf("%q is not a %s", text, name))
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			return at(path, fmt.Errorf("%q is not a %s", text, name))
		}
	default:
		return at(path, fmt.Errorf("unsupported format %q", name))
	}
	return nil
}

func (v validator) checkNumber(path string, schema map[string]any, value any) error {
	declared, ok := schema["minimum"]
	if !ok {
		return nil
	}
	number, isNumber := value.(float64)
	if !isNumber {
		return nil
	}
	minimum, ok := declared.(float64)
	if !ok {
		return at(path, fmt.Errorf("minimum must be a number, got %T", declared))
	}
	if number < minimum {
		return at(path, fmt.Errorf("%v is below the minimum %v", number, minimum))
	}
	return nil
}

func (v validator) checkString(path string, schema map[string]any, value any) error {
	text, isText := value.(string)
	if !isText {
		return nil
	}
	if declared, ok := schema["minLength"]; ok {
		minimum, ok := declared.(float64)
		if !ok {
			return at(path, fmt.Errorf("minLength must be a number, got %T", declared))
		}
		if float64(len(text)) < minimum {
			return at(path, fmt.Errorf("%q is shorter than the minimum %v", text, minimum))
		}
	}
	if declared, ok := schema["pattern"]; ok {
		expression, ok := declared.(string)
		if !ok {
			return at(path, fmt.Errorf("pattern must be a string, got %T", declared))
		}
		matcher, err := regexp.Compile(expression)
		if err != nil {
			return at(path, fmt.Errorf("pattern %q does not compile: %w", expression, err))
		}
		if !matcher.MatchString(text) {
			return at(path, fmt.Errorf("%q does not match %q", text, expression))
		}
	}
	return nil
}

func (v validator) checkObject(path string, schema map[string]any, value any) error {
	object, isObject := value.(map[string]any)
	if !isObject {
		return nil
	}
	if declared, ok := schema["required"]; ok {
		names, ok := declared.([]any)
		if !ok {
			return at(path, fmt.Errorf("required must be an array, got %T", declared))
		}
		for _, entry := range names {
			name, _ := entry.(string)
			if _, present := object[name]; !present {
				return at(path, fmt.Errorf("required property %q is missing", name))
			}
		}
	}

	properties, _ := schema["properties"].(map[string]any)
	// Sorted so a document with several problems always reports the same one.
	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		declared, ok := properties[name]
		if !ok {
			if allowed, present := schema["additionalProperties"]; present {
				if permitted, isBool := allowed.(bool); isBool && !permitted {
					return at(path, fmt.Errorf("property %q is not allowed", name))
				}
			}
			continue
		}
		sub, ok := declared.(map[string]any)
		if !ok {
			return at(path, fmt.Errorf("property %q has a non-object schema", name))
		}
		if err := v.check(join(path, name), sub, object[name]); err != nil {
			return err
		}
	}
	return nil
}

func (v validator) checkArray(path string, schema map[string]any, value any) error {
	array, isArray := value.([]any)
	if !isArray {
		return nil
	}
	declared, ok := schema["items"]
	if !ok {
		return nil
	}
	sub, ok := declared.(map[string]any)
	if !ok {
		return at(path, fmt.Errorf("items must be an object schema, got %T", declared))
	}
	for i, element := range array {
		if err := v.check(fmt.Sprintf("%s[%d]", path, i), sub, element); err != nil {
			return err
		}
	}
	return nil
}

func equalJSON(a, b any) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	return errLeft == nil && errRight == nil && string(left) == string(right)
}

func jsonType(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func at(path string, err error) error {
	if path == "" {
		return err
	}
	return fmt.Errorf("%s: %w", path, err)
}
