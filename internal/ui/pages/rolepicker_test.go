package pages

import (
	"context"
	"strings"
	"testing"
)

func renderRolePicker(t *testing.T, p RolePickerProps) string {
	t.Helper()
	var b strings.Builder
	if err := RolePicker(p).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestRolePickerRendersBothPanes(t *testing.T) {
	body := renderRolePicker(t, RolePickerProps{
		ID: "rp", Name: "roles", Signal: "rpPicked", FilterSignal: "rpQ", ItemIDPrefix: "rp-",
		Items: []RolePickItem{
			{Value: "orders:viewer", Key: "r0", Name: "Orders Viewer", Selected: true},
			{Value: "orders:editor", Key: "r1", Name: "Orders Editor"},
			{Value: "platform:admin", Key: "r2", Name: "Admin", Selected: true, Locked: true},
			{Value: "platform:root", Key: "r3", Name: "Root", Locked: true},
		},
		Hidden: 1,
	})
	mustContain(t, body,
		"Available Roles", "Selected Roles (", `placeholder="Filter roles..."`,
		// Display name over the code, in both panes.
		"Orders Viewer", "orders:viewer", "Orders Editor", "orders:editor",
		// The positional signal: unlocked roles only, '' for the unpicked.
		`{rpPicked: [&#34;orders:viewer&#34;, &#34;&#34;], rpQ: &#39;&#39;}`,
		// The count includes the locked pick; the render-time count is 2.
		`$rpPicked.filter(Boolean).length + 1`,
		// Locked: hint, no checkbox; the held one is posted back, the other is not.
		rolePickerLockedHint, `<input type="hidden" name="roles" value="platform:admin">`,
		// A remove button per unlocked role; the lock icon for the locked one.
		`aria-label="Remove Orders Viewer"`, `document.getElementById(&#39;rp-r0&#39;).click()`,
		// One hidden role: singular.
		"1 role hidden because their application isn&#39;t enabled for this user", "No roles found", "No roles selected",
		// Keyboard: a real checkbox, Enter toggles.
		`type="checkbox"`, "evt.key === &#39;Enter&#39;",
	)
	mustNotContain(t, body, `name="roles" value="platform:root"`, `id="rp-r2"`, `id="rp-r3"`, "1 roles hidden")
	if strings.Count(body, `name="roles"`) != 3 {
		t.Errorf("posted fields: want 2 checkboxes + 1 locked hidden:\n%s", body)
	}
}

func TestRolePickerEscapesRoleText(t *testing.T) {
	body := renderRolePicker(t, RolePickerProps{
		ID: "rp", Name: "roles", Signal: "rpPicked", FilterSignal: "rpQ", ItemIDPrefix: "rp-",
		Items: []RolePickItem{{Value: `a:"</script><b>'`, Key: "r0", Name: "<img src=x onerror=1>", Selected: true}},
	})
	mustNotContain(t, body, "<img", "</script><b>")
}

func TestRolePickerEmptyAndPlural(t *testing.T) {
	body := renderRolePicker(t, RolePickerProps{ID: "rp", Name: "roles", Signal: "rpPicked", FilterSignal: "rpQ", ItemIDPrefix: "rp-", Hidden: 3})
	mustContain(t, body, "3 roles hidden", "Selected Roles (")
	// "No roles found" must not start hidden for an empty list.
	mustNotContain(t, body, `data-texts="[]" style="display: none"`)
}
