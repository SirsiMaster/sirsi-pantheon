package namespec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// canonicalize implements enough of RFC 8785 (JSON Canonicalization Scheme)
// for ADR-072 C2's registry schema documents: UTF-8 input, object keys sorted
// by UTF-16 code unit value, no insignificant whitespace, and integer numbers
// in their shortest round-trip form. The registry schema contains only
// strings, booleans, objects, arrays, and small integers (schema_version) —
// this does not implement general ECMA-262 float serialization, because no
// schema document has ever needed it; a non-integer number is a load error,
// not a silently-approximated one (A35: scope the check to the claim).
//
// It also rejects what the plain encoding/json decoder would silently accept:
// a duplicate key within one object (last-wins in the standard decoder, which
// would let a schema's hashed meaning diverge from what a reviewer read), and
// non-UTF-8 input.
func canonicalize(raw []byte) ([]byte, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("namespec: schema document is not valid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, fmt.Errorf("namespec: parse for canonicalization: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		// Anything after the single top-level value is malformed JSON.
		return nil, fmt.Errorf("namespec: trailing content after top-level value")
	}
	var buf bytes.Buffer
	if err := encodeCanonical(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SchemaHash computes the ADR-072 C2 hash: sha256 over the JCS-canonicalized
// bytes of raw with the top-level "schema_hash" field excluded from the
// hashed payload (a hash that includes itself proves nothing). raw must be a
// JSON object; any other top-level shape is a load error.
func SchemaHash(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return "", fmt.Errorf("namespec: parse for hashing: %w", err)
	}
	obj, ok := v.(*orderedObject)
	if !ok {
		return "", fmt.Errorf("namespec: schema document must be a JSON object")
	}
	obj.delete("schema_hash")
	var buf bytes.Buffer
	if err := encodeCanonical(&buf, obj); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

// orderedObject preserves first-insertion order (irrelevant to the canonical
// output, which always re-sorts) while letting decodeValue detect a repeated
// key before it silently overwrites the first occurrence.
type orderedObject struct {
	keys   []string
	values map[string]any
}

func newOrderedObject() *orderedObject {
	return &orderedObject{values: map[string]any{}}
}

func (o *orderedObject) set(k string, v any) error {
	if _, dup := o.values[k]; dup {
		return fmt.Errorf("namespec: duplicate key %q in schema document", k)
	}
	o.keys = append(o.keys, k)
	o.values[k] = v
	return nil
}

func (o *orderedObject) delete(k string) {
	if _, ok := o.values[k]; !ok {
		return
	}
	delete(o.values, k)
	for i, kk := range o.keys {
		if kk == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := newOrderedObject()
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, fmt.Errorf("namespec: non-string object key")
				}
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				if err := obj.set(key, val); err != nil {
					return nil, err
				}
			}
			if _, err := dec.Token(); err != nil { // closing '}'
				return nil, err
			}
			return obj, nil
		case '[':
			var arr []any
			for dec.More() {
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
			if _, err := dec.Token(); err != nil { // closing ']'
				return nil, err
			}
			return arr, nil
		default:
			return nil, fmt.Errorf("namespec: unexpected delimiter %q", t)
		}
	case json.Number, string, bool, nil:
		return t, nil
	default:
		return nil, fmt.Errorf("namespec: unexpected token %T", t)
	}
}

func encodeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		encodeCanonicalString(buf, t)
	case json.Number:
		return encodeCanonicalNumber(buf, t)
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case *orderedObject:
		keys := append([]string(nil), t.keys...)
		sort.Slice(keys, func(i, j int) bool { return less16(keys[i], keys[j]) })
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			encodeCanonicalString(buf, k)
			buf.WriteByte(':')
			if err := encodeCanonical(buf, t.values[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("namespec: unsupported value type %T", v)
	}
	return nil
}

// less16 orders by UTF-16 code unit value (RFC 8785 §3.2.3), which differs
// from a plain byte or rune comparison only for characters outside the Basic
// Multilingual Plane (surrogate pairs sort by their high surrogate, which is
// numerically below the BMP's upper half). The registry schema's keys are all
// ASCII, so this only matters if that ever changes.
func less16(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

// encodeCanonicalString writes s per RFC 8785 §3.2.2: a plain JSON string with
// only the characters JSON itself requires escaped, and no other escaping.
func encodeCanonicalString(buf *bytes.Buffer, s string) {
	b, _ := json.Marshal(s)
	buf.Write(b)
}

// encodeCanonicalNumber writes a schema document's number in its shortest
// round-trip integer form. Scoped to integers (see the package doc comment on
// canonicalize): the registry schema only ever carries schema_version, and a
// non-integer number there is a malformed schema, not a value to approximate.
func encodeCanonicalNumber(buf *bytes.Buffer, n json.Number) error {
	i, err := n.Int64()
	if err != nil {
		f, ferr := n.Float64()
		if ferr != nil || math.Trunc(f) != f {
			return fmt.Errorf("namespec: schema number %q is not an integer; canonical hashing is scoped to integers", n.String())
		}
		i = int64(f)
	}
	buf.WriteString(strconv.FormatInt(i, 10))
	return nil
}
