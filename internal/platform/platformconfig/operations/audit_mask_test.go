package operations

import (
	"reflect"
	"testing"
)

func TestSetPropertyMasksTheValueUnlessExplicitlyPlain(t *testing.T) {
	plain, secret := "PLAIN", "SECRET"
	if got := (SetPropertyCommand{ValueType: &plain}).AuditMaskedFields(); got != nil {
		t.Fatalf("PLAIN must keep the value, got mask %v", got)
	}
	for name, vt := range map[string]*string{"SECRET": &secret, "nil (keep current type)": nil} {
		if got := (SetPropertyCommand{ValueType: vt}).AuditMaskedFields(); !reflect.DeepEqual(got, []string{"value"}) {
			t.Fatalf("%s must mask value, got %v", name, got)
		}
	}
}
