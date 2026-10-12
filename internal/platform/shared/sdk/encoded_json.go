package sdk

import (
	"bytes"
	"encoding/json"
)

// UnwrapEncodedJSON undoes one level of the SDK outbox writers' JSON
// encoding of a structured member (F13, owner ruling 2026-10-12: the
// platform unwraps at ingest). The Java, TypeScript, Laravel, fc-sdk and
// framework writers put an event's `data` and an audit entry's
// `operationData` into the batch item as a JSON string holding the
// serialised value (`"data":"{\"a\":1}"`); stored as sent, subscribers got a
// string and a dataOnly delivery body was a quoted string.
//
// The rule: raw is replaced by the text of the string it holds only when
//
//   - raw is a JSON string (anything else, an object, array, number, boolean
//     or null, is returned unchanged, so a writer sending the value itself is
//     never touched), and
//   - that string, less surrounding JSON whitespace, starts with `{` or `[`
//     and is exactly one valid JSON value, i.e. an encoded object or array.
//
// Every other string is a value meant as a string and is kept: "hello", "",
// "42", "true", "null" (what Java writes for a null map), an encoded string
// ("\"x\""), and text that only looks like JSON ("{not json", "[1,"). At
// most one level is removed: the result is an object or array, never a
// string, so nothing is unwrapped twice, and a value encoded twice (its text
// starts with `"`) is kept as sent. The known cost: a writer that means a
// string whose text is a JSON object or array (sends `"data":"{\"a\":1}"`
// deliberately) gets the object. Such a value has to be wrapped in an object
// to arrive as text.
func UnwrapEncodedJSON(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.Trim(raw, jsonSpace)
	if len(trimmed) < 2 || trimmed[0] != '"' {
		return raw
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err != nil {
		return raw
	}
	inner := bytes.Trim([]byte(text), jsonSpace)
	if len(inner) == 0 || (inner[0] != '{' && inner[0] != '[') || !json.Valid(inner) {
		return raw
	}
	return json.RawMessage(inner)
}

// jsonSpace is JSON's insignificant whitespace (RFC 8259 section 2).
const jsonSpace = " \t\n\r"
