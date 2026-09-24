package usecase

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type auditVector struct {
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Masked   []string        `json:"masked"`
	Expected json.RawMessage `json:"expected"`
}

// Every case of the shared vectors (a copy of the Java platform's
// docs/spec/audit-redaction-vectors.json — the TS, Laravel and Java
// implementations run the same file).
func TestRedactAuditJSONAgreesWithEverySharedVector(t *testing.T) {
	raw, err := os.ReadFile("testdata/audit-redaction-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []auditVector
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			got, err := RedactAuditJSON(v.Input, v.Masked)
			if err != nil {
				t.Fatal(err)
			}
			var gotAny, wantAny any
			if err := json.Unmarshal(got, &gotAny); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(v.Expected, &wantAny); err != nil {
				t.Fatal(err)
			}
			g, _ := json.Marshal(gotAny)
			w, _ := json.Marshal(wantAny)
			if string(g) != string(w) {
				t.Fatalf("got  %s\nwant %s", g, w)
			}
		})
	}
}

type maskedCmd struct {
	Value     string `json:"value"`
	ValueType string `json:"valueType"`
	Password  string `json:"password"`
}

func (c maskedCmd) AuditMaskedFields() []string { return []string{"value"} }

func TestRedactedAuditCommandAppliesTheCommandsOwnMask(t *testing.T) {
	got, err := RedactedAuditCommand(maskedCmd{Value: "sk_live_1", ValueType: "SECRET", Password: "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, "sk_live_1") || strings.Contains(s, "hunter2") {
		t.Fatalf("secret survived: %s", s)
	}
	if !strings.Contains(s, `"valueType":"SECRET"`) {
		t.Fatalf("non-secret field lost: %s", s)
	}
}

func TestLargeNumbersSurviveExactly(t *testing.T) {
	got, err := RedactAuditJSON([]byte(`{"count":12345678901234567890,"token":"x"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "12345678901234567890") {
		t.Fatalf("number rounded: %s", got)
	}
}
