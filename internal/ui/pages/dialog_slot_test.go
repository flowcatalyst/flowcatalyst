package pages

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
)

// A dialog is put into its slot by replacing the slot, never by morphing it
// (docs/ui/README.md, "Dialog slot: replace, do not morph"): a morph keeps a
// <dialog> with the same id and data-init, so the second dialog opened on a
// page (create, closed, then edit) came up non-modal. Every page's dialogs go
// through patchDialogSlot; this pins its patch.
func TestPatchDialogSlotReplacesTheSlot(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ui/x", nil)
	req.Header.Set("Datastar-Request", "true")
	dialog := templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<dialog id="x-form" data-init="el.showModal()"></dialog>`)
		return err
	})
	if err := patchDialogSlot(ds.NewSSE(rec, req), "x-dialog-slot", dialog); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{"data: mode replace\n", `data: elements <div id="x-dialog-slot"><dialog id="x-form"`} {
		if !strings.Contains(body, want) {
			t.Errorf("patch lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "mode inner") || strings.Contains(body, "selector #x-dialog-slot") {
		t.Errorf("the slot was morphed, not replaced:\n%s", body)
	}
}
