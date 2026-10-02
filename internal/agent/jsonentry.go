package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// This file edits one entry of a top-level JSON array in a config file
// the user owns: opencode.json "instructions" (SPEC-013
// §Mounts and consent, T8). Re-encoding the file through encoding/json
// would reorder keys and reformat it, so the edit is a byte-range
// splice at offsets json.Decoder reports. Every byte outside the edited
// range is kept. A file encoding/json cannot parse (JSONC comments,
// trailing commas) is refused, never rewritten.

// errJSONNotEditable reports a config sync-agents will not edit: not
// strict JSON, or not the expected shape (a top-level object whose key
// holds an array). The caller prints the line to add and leaves the
// file alone (ChannelManual). Errors returned for it wrap this value
// with the reason.
var errJSONNotEditable = errors.New("config cannot be edited safely")

// jsonObject is the top-level object of a document: the offsets of its
// braces and of each member.
type jsonObject struct {
	open, close int // offsets of '{' and '}'
	members     []jsonMember
}

// jsonMember is one `"key": value` pair. keyStart is the opening quote
// of the key; [valueStart, valueEnd) is the value.
type jsonMember struct {
	key                            string
	keyStart, valueStart, valueEnd int
}

// jsonArray is an array value: the offsets of its brackets and the
// span of each element.
type jsonArray struct {
	open, close int // offsets of '[' and ']'
	elems       []jsonElem
}

// jsonElem is one array element, [start, end), with its raw bytes.
type jsonElem struct {
	start, end int
	raw        json.RawMessage
}

// ensureJSONArrayEntry inserts value into the top-level array at key,
// creating the key when absent and the whole document when src is
// empty (the config does not exist yet). It edits only the byte range
// of that array, or appends one member to the object, so formatting
// and key order elsewhere are untouched. The inserted text follows the
// neighbouring layout: on its own line, with the previous line's
// indentation, when the previous element or member starts a line.
//
// An existing equal string entry makes it a no-op (changed false), so
// repeated syncs converge.
func ensureJSONArrayEntry(src []byte, key, value string) (out []byte, changed bool, err error) {
	entry := jsonString(value)
	if len(bytes.TrimSpace(src)) == 0 {
		return []byte("{\n  " + jsonString(key) + ": [" + entry + "]\n}\n"), true, nil
	}
	obj, err := scanJSONObject(src)
	if err != nil {
		return nil, false, err
	}
	i := obj.member(key)
	if i < 0 {
		return insertJSONMember(src, obj, jsonString(key)+": ["+entry+"]"), true, nil
	}
	arr, err := scanJSONArray(src, obj.members[i])
	if err != nil {
		return nil, false, err
	}
	if arr.index(value) >= 0 {
		return src, false, nil
	}
	if len(arr.elems) == 0 {
		return splice(src, arr.open+1, arr.close, entry), true, nil
	}
	last := arr.elems[len(arr.elems)-1]
	return splice(src, last.end, last.end, separator(src, last.start)+entry), true, nil
}

// removeJSONArrayEntry is ensureJSONArrayEntry's inverse, for clean. It
// removes every string element equal to value, together with one
// separating comma and the whitespace ensureJSONArrayEntry added. When
// that leaves the array empty, the whole member goes, so an
// ensure-then-remove round trip restores a document that had no such
// key byte for byte. A missing document, key, or entry is a no-op.
func removeJSONArrayEntry(src []byte, key, value string) (out []byte, changed bool, err error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return src, false, nil
	}
	for {
		obj, err := scanJSONObject(src)
		if err != nil {
			return nil, false, err
		}
		i := obj.member(key)
		if i < 0 {
			return src, changed, nil
		}
		arr, err := scanJSONArray(src, obj.members[i])
		if err != nil {
			return nil, false, err
		}
		j := arr.index(value)
		if j < 0 {
			return src, changed, nil
		}
		if len(arr.elems) == 1 {
			src = removeJSONMember(src, obj, i)
		} else {
			src = removeJSONElem(src, arr, j)
		}
		changed = true
	}
}

// scanJSONObject parses src as strict JSON whose top-level value is an
// object and records the offsets of its members.
func scanJSONObject(src []byte) (jsonObject, error) {
	if !json.Valid(src) {
		return jsonObject{}, fmt.Errorf("%w: not strict JSON (comments or trailing commas?)", errJSONNotEditable)
	}
	dec := json.NewDecoder(bytes.NewReader(src))
	tok, err := dec.Token()
	if err != nil {
		return jsonObject{}, fmt.Errorf("%w: %v", errJSONNotEditable, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return jsonObject{}, fmt.Errorf("%w: the top-level value is not an object", errJSONNotEditable)
	}
	obj := jsonObject{open: int(dec.InputOffset()) - 1}
	for dec.More() {
		keyStart := skipJSONSpace(src, int(dec.InputOffset()), ",")
		tok, err := dec.Token()
		if err != nil {
			return jsonObject{}, fmt.Errorf("%w: %v", errJSONNotEditable, err)
		}
		key, _ := tok.(string)
		valueStart := skipJSONSpace(src, int(dec.InputOffset()), ":")
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return jsonObject{}, fmt.Errorf("%w: %v", errJSONNotEditable, err)
		}
		obj.members = append(obj.members, jsonMember{
			key: key, keyStart: keyStart, valueStart: valueStart, valueEnd: int(dec.InputOffset()),
		})
	}
	if _, err := dec.Token(); err != nil {
		return jsonObject{}, fmt.Errorf("%w: %v", errJSONNotEditable, err)
	}
	obj.close = int(dec.InputOffset()) - 1
	return obj, nil
}

// member returns the index of the first member named key, or -1.
func (o jsonObject) member(key string) int {
	for i, m := range o.members {
		if m.key == key {
			return i
		}
	}
	return -1
}

// scanJSONArray records the element offsets of m's value, which must be
// an array.
func scanJSONArray(src []byte, m jsonMember) (jsonArray, error) {
	val := src[m.valueStart:m.valueEnd]
	dec := json.NewDecoder(bytes.NewReader(val))
	tok, err := dec.Token()
	if err != nil {
		return jsonArray{}, fmt.Errorf("%w: %v", errJSONNotEditable, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return jsonArray{}, fmt.Errorf("%w: %q is not an array", errJSONNotEditable, m.key)
	}
	arr := jsonArray{open: m.valueStart}
	for dec.More() {
		start := m.valueStart + skipJSONSpace(val, int(dec.InputOffset()), ",")
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return jsonArray{}, fmt.Errorf("%w: %v", errJSONNotEditable, err)
		}
		arr.elems = append(arr.elems, jsonElem{start: start, end: m.valueStart + int(dec.InputOffset()), raw: raw})
	}
	if _, err := dec.Token(); err != nil {
		return jsonArray{}, fmt.Errorf("%w: %v", errJSONNotEditable, err)
	}
	arr.close = m.valueStart + int(dec.InputOffset()) - 1
	return arr, nil
}

// index returns the position of the first string element equal to
// value, or -1. Non-string elements never match.
func (a jsonArray) index(value string) int {
	for i, e := range a.elems {
		var s string
		if json.Unmarshal(e.raw, &s) == nil && s == value {
			return i
		}
	}
	return -1
}

// insertJSONMember appends member after the object's last member, or
// fills an empty object.
func insertJSONMember(src []byte, obj jsonObject, member string) []byte {
	if len(obj.members) == 0 {
		return splice(src, obj.open+1, obj.close, "\n  "+member+"\n")
	}
	last := obj.members[len(obj.members)-1]
	return splice(src, last.valueEnd, last.valueEnd, separator(src, last.keyStart)+member)
}

// removeJSONMember removes member i with one adjacent comma: the one
// after it when another member follows, else the one before it. An
// only member leaves an empty object.
func removeJSONMember(src []byte, obj jsonObject, i int) []byte {
	m := obj.members
	switch {
	case i < len(m)-1:
		return splice(src, m[i].keyStart, m[i+1].keyStart, "")
	case i > 0:
		return splice(src, m[i-1].valueEnd, m[i].valueEnd, "")
	default:
		return splice(src, obj.open+1, obj.close, "")
	}
}

// removeJSONElem removes element i (never the only one) with one
// adjacent comma, as removeJSONMember does for members.
func removeJSONElem(src []byte, arr jsonArray, i int) []byte {
	e := arr.elems
	if i < len(e)-1 {
		return splice(src, e[i].start, e[i+1].start, "")
	}
	return splice(src, e[i-1].end, e[i].end, "")
}

// separator is what goes between the item starting at prevStart and a
// new item after it: a comma, then a newline and the same indentation
// when the previous item starts its own line, else a space.
func separator(src []byte, prevStart int) string {
	i := prevStart
	for i > 0 && (src[i-1] == ' ' || src[i-1] == '\t') {
		i--
	}
	if i > 0 && src[i-1] == '\n' {
		return ",\n" + string(src[i:prevStart])
	}
	return ", "
}

// skipJSONSpace returns the first offset at or after i that is neither
// JSON whitespace nor one of the punctuation bytes in extra.
func skipJSONSpace(src []byte, i int, extra string) int {
	for i < len(src) {
		c := src[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' && !bytes.ContainsRune([]byte(extra), rune(c)) {
			break
		}
		i++
	}
	return i
}

// splice returns src with [from, to) replaced by text, in a new slice.
func splice(src []byte, from, to int, text string) []byte {
	out := make([]byte, 0, len(src)-(to-from)+len(text))
	out = append(out, src[:from]...)
	out = append(out, text...)
	return append(out, src[to:]...)
}

// jsonString encodes s as a JSON string literal without HTML escaping,
// so a path stays readable in the user's config.
func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // encoding a string cannot fail
	return string(bytes.TrimRight(buf.Bytes(), "\n"))
}
