package llm

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Schema is a named JSON schema for structured output.
type Schema struct {
	Name       string
	Definition json.RawMessage
}

type node struct {
	Type                 any        `json:"type,omitempty"`
	Description          string     `json:"description,omitempty"`
	Enum                 []any      `json:"enum,omitempty"`
	Items                *node      `json:"items,omitempty"`
	Properties           properties `json:"properties,omitempty"`
	Required             []string   `json:"required,omitempty"`
	AdditionalProperties *bool      `json:"additionalProperties,omitempty"`
	AnyOf                []*node    `json:"anyOf,omitempty"`
}

type property struct {
	name   string
	schema *node
}

// properties keeps struct field order: models generate fields in schema order.
type properties []property

func (p properties) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, prop := range p {
		if i > 0 {
			buf.WriteByte(',')
		}
		name, err := json.Marshal(prop.name)
		if err != nil {
			return nil, err
		}
		schema, err := json.Marshal(prop.schema)
		if err != nil {
			return nil, err
		}
		buf.Write(name)
		buf.WriteByte(':')
		buf.Write(schema)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	invalidNameChars  = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
)

// SchemaFor derives an OpenAI strict-mode JSON schema from struct type T:
// every property is required, objects reject additional properties, and
// pointer or omitempty fields are nullable. Field names come from json tags;
// an optional `llm:"desc=...,enum=a|b"` tag adds a description and an enum.
func SchemaFor[T any]() (Schema, error) {
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Struct {
		return Schema{}, fmt.Errorf("llm: schema root must be a struct, got %s", t)
	}
	root, err := schemaOf(t, map[reflect.Type]bool{})
	if err != nil {
		return Schema{}, fmt.Errorf("llm: schema for %s: %w", t, err)
	}
	def, err := json.Marshal(root)
	if err != nil {
		return Schema{}, fmt.Errorf("llm: encoding schema for %s: %w", t, err)
	}
	return Schema{Name: schemaName(t), Definition: def}, nil
}

func MustSchemaFor[T any]() Schema {
	s, err := SchemaFor[T]()
	if err != nil {
		panic(err)
	}
	return s
}

func schemaName(t reflect.Type) string {
	name := strings.Trim(invalidNameChars.ReplaceAllString(t.Name(), "_"), "_")
	if name == "" {
		name = "output"
	}
	return name[:min(len(name), 64)]
}

func schemaOf(t reflect.Type, visiting map[reflect.Type]bool) (*node, error) {
	if t.Kind() != reflect.Pointer && (t.Implements(jsonMarshalerType) || t.Implements(textMarshalerType) ||
		reflect.PointerTo(t).Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)) {
		return nil, fmt.Errorf("unsupported type %s: custom JSON encoding", t)
	}
	switch t.Kind() {
	case reflect.String:
		return &node{Type: "string"}, nil
	case reflect.Bool:
		return &node{Type: "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &node{Type: "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return &node{Type: "number"}, nil
	case reflect.Pointer:
		elem, err := schemaOf(t.Elem(), visiting)
		if err != nil {
			return nil, err
		}
		return nullable(elem), nil
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return nil, fmt.Errorf("unsupported type %s: byte slices encode as base64", t)
		}
		items, err := schemaOf(t.Elem(), visiting)
		if err != nil {
			return nil, err
		}
		return &node{Type: "array", Items: items}, nil
	case reflect.Struct:
		return objectSchema(t, visiting)
	default:
		return nil, fmt.Errorf("unsupported type %s", t)
	}
}

func objectSchema(t reflect.Type, visiting map[reflect.Type]bool) (*node, error) {
	if visiting[t] {
		return nil, fmt.Errorf("recursive type %s", t)
	}
	visiting[t] = true
	defer delete(visiting, t)

	props, err := structProperties(t, visiting)
	if err != nil {
		return nil, err
	}
	if len(props) == 0 {
		return nil, fmt.Errorf("struct %s has no exported fields", t)
	}
	seen := map[string]bool{}
	required := make([]string, 0, len(props))
	for _, p := range props {
		if seen[p.name] {
			return nil, fmt.Errorf("duplicate property %q in %s", p.name, t)
		}
		seen[p.name] = true
		required = append(required, p.name)
	}
	noExtra := false
	return &node{Type: "object", Properties: props, Required: required, AdditionalProperties: &noExtra}, nil
}

func structProperties(t reflect.Type, visiting map[reflect.Type]bool) (properties, error) {
	var props properties
	for f := range t.Fields() {
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			embedded, err := structProperties(f.Type, visiting)
			if err != nil {
				return nil, err
			}
			props = append(props, embedded...)
			continue
		}
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Pointer {
			return nil, fmt.Errorf("embedded pointer field %s.%s is not supported", t, f.Name)
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		if hasOption(opts, "string") {
			return nil, fmt.Errorf("field %s.%s: the json \",string\" option is not supported", t, f.Name)
		}
		schema, err := fieldSchema(f, hasOption(opts, "omitempty") || hasOption(opts, "omitzero"), visiting)
		if err != nil {
			return nil, fmt.Errorf("field %s.%s: %w", t, f.Name, err)
		}
		props = append(props, property{name: name, schema: schema})
	}
	return props, nil
}

func fieldSchema(f reflect.StructField, optional bool, visiting map[reflect.Type]bool) (*node, error) {
	desc, enum, err := parseLLMTag(f.Tag.Get("llm"))
	if err != nil {
		return nil, err
	}
	schema, err := schemaOf(f.Type, visiting)
	if err != nil {
		return nil, err
	}
	if len(enum) > 0 {
		if f.Type.Kind() != reflect.String && (f.Type.Kind() != reflect.Pointer || f.Type.Elem().Kind() != reflect.String) {
			return nil, fmt.Errorf("enum requires a string field, got %s", f.Type)
		}
		schema.Enum = make([]any, 0, len(enum)+1)
		for _, v := range enum {
			schema.Enum = append(schema.Enum, v)
		}
	}
	if optional {
		schema = nullable(schema)
	}
	if isNullable(schema) && schema.Enum != nil {
		schema.Enum = append(schema.Enum, nil)
	}
	schema.Description = desc
	return schema, nil
}

func nullable(n *node) *node {
	if isNullable(n) {
		return n
	}
	if n.Type == "object" {
		return &node{AnyOf: []*node{n, {Type: "null"}}}
	}
	cp := *n
	cp.Type = []string{n.Type.(string), "null"}
	return &cp
}

func isNullable(n *node) bool {
	_, isList := n.Type.([]string)
	return isList || n.AnyOf != nil
}

func hasOption(opts, want string) bool {
	for opt := range strings.SplitSeq(opts, ",") {
		if opt == want {
			return true
		}
	}
	return false
}

// parseLLMTag splits on commas that start a known key, so descriptions may
// contain commas themselves.
func parseLLMTag(tag string) (desc string, enum []string, err error) {
	if tag == "" {
		return "", nil, nil
	}
	var parts []string
	for seg := range strings.SplitSeq(tag, ",") {
		if len(parts) > 0 && !strings.HasPrefix(seg, "desc=") && !strings.HasPrefix(seg, "enum=") {
			parts[len(parts)-1] += "," + seg
			continue
		}
		parts = append(parts, seg)
	}
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		switch {
		case ok && key == "desc":
			desc = value
		case ok && key == "enum" && value != "":
			enum = strings.Split(value, "|")
		default:
			return "", nil, fmt.Errorf("invalid llm tag %q", tag)
		}
	}
	return desc, enum, nil
}
