package sdk

import (
	"encoding/json"
	"testing"
)

func TestUnwrapEncodedJSON(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		// The SDK writers' encoding of an object or array: unwrapped once.
		{"encoded object (Java, TS, Laravel, fc-sdk, framework)", `"{\"orderId\":\"ord_1\",\"total\":1500}"`, `{"orderId":"ord_1","total":1500}`},
		{"encoded empty object", `"{}"`, `{}`},
		{"encoded empty array (PHP json_encode([]))", `"[]"`, `[]`},
		{"encoded list", `"[1,{\"a\":true}]"`, `[1,{"a":true}]`},
		{"encoded object with surrounding whitespace", " \n\"  {\\\"a\\\":1} \\n\" ", `{"a":1}`},
		{"encoded nested string members stay strings", `"{\"note\":\"{\\\"x\\\":1}\"}"`, `{"note":"{\"x\":1}"}`},
		{"unicode escapes", `"{\"name\":\"Zo\u00eb ✓\"}"`, `{"name":"Zoë ✓"}`},

		// Values sent as themselves: never touched.
		{"object", `{"a":1}`, `{"a":1}`},
		{"array", `[1,2]`, `[1,2]`},
		{"number", `42`, `42`},
		{"boolean", `true`, `true`},
		{"null", `null`, `null`},

		// Strings meant as strings: kept.
		{"plain string", `"hello"`, `"hello"`},
		{"empty string", `""`, `""`},
		{"numeric text", `"42"`, `"42"`},
		{"boolean text", `"true"`, `"true"`},
		{"null text (Java's null map)", `"null"`, `"null"`},
		{"encoded string (two levels) kept as sent", `"\"{\\\"a\\\":1}\""`, `"\"{\\\"a\\\":1}\""`},
		{"looks like an object, is not JSON", `"{not json"`, `"{not json"`},
		{"truncated array", `"[1,"`, `"[1,"`},
		{"object followed by more text", `"{} trailing"`, `"{} trailing"`},
		{"two values", `"{}{}"`, `"{}{}"`},
		{"brace in the middle", `"total {a}"`, `"total {a}"`},

		// Not valid JSON at all: returned unchanged for the caller to refuse.
		{"empty", ``, ``},
		{"unterminated string", `"abc`, `"abc`},
	}
	for _, c := range cases {
		got := string(UnwrapEncodedJSON(json.RawMessage(c.in)))
		if got != c.want {
			t.Errorf("%s: UnwrapEncodedJSON(%s) = %s, want %s", c.name, c.in, got, c.want)
		}
		// Idempotent: a second pass never changes the result.
		if again := string(UnwrapEncodedJSON(json.RawMessage(got))); again != got {
			t.Errorf("%s: second pass changed %s to %s", c.name, got, again)
		}
	}
}
