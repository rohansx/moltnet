package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Canonicalize renders v into deterministic JSON: object keys sorted
// lexicographically, minimal separators, no insignificant whitespace. This is
// a JCS-compatible (RFC 8785) serializer for the subset of JSON that MoltNet
// cards and attestations use (objects, arrays, strings, bools, null, integers).
//
// Full RFC 8785 floating-point canonicalization is not implemented; MoltNet
// signed content avoids non-integer numbers. See spec/card-v0.1.md.
func Canonicalize(v any) ([]byte, error) {
	// Round-trip through generic JSON so struct field order can't affect output.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CanonicalizeWithout canonicalizes v after removing the given top-level keys.
// Used to derive the signing payload (a record minus its signature fields).
func CanonicalizeWithout(v any, dropKeys ...string) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		// Not an object; nothing to drop.
		return Canonicalize(v)
	}
	for _, k := range dropKeys {
		delete(m, k)
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, k)
			buf.WriteByte(':')
			if err := writeCanonical(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		writeJSONString(buf, val)
	case json.Number:
		buf.WriteString(val.String())
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	case float64:
		// Only reached if UseNumber was bypassed; keep it deterministic.
		buf.WriteString(fmt.Sprintf("%v", val))
	default:
		return fmt.Errorf("canonicalize: unsupported type %T", v)
	}
	return nil
}

func writeJSONString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				buf.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}
