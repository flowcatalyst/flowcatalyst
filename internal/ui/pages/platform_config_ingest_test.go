package pages

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
)

// Section ingest holds the per-environment settings of ingestions
// (docs/ingestion-design.md section 13). They are changed on the ingestion, so
// the generic page lists them and offers neither Edit nor Delete.
func cfgWithIngestEntry() *cfgHarness {
	h := newCfgHarness()
	acme := ids.ClientID("clt_acme")
	row := cfgRowFixture("pfc_9", platformconfig.ReservedSection, "vendor-feed", platformconfig.ValuePlain, `{"host":"sftp.vendor.example"}`)
	row.Scope, row.ClientID = platformconfig.ScopeClient, &acme
	h.cfg.rows = append(h.cfg.rows, row)
	return h
}

func TestConfigListShowsIngestEntriesReadOnly(t *testing.T) {
	h := cfgWithIngestEntry()
	body := h.do(t, cfgAdmin(), req{target: "/ui/platform/config", datastar: true})
	mustContain(t, body, `vendor-feed`, `data-testid="cfg-reserved"`, `Edited on the ingestion`,
		`href="/ui/ingestions?application=platform&amp;client=clt_acme"`)
	// The other rows are as ever; this one offers nothing to change.
	mustContain(t, body, `/ui/platform/config/edit?app=platform&amp;property=name&amp;section=branding`)
	mustNotContain(t, body, `section=ingest`)
	assert.Equal(t, 1, strings.Count(body, `Edited on the ingestion`))
}

// A property cannot be created in the reserved section from the generic form:
// the platform refuses it, and the page says so on the section field.
func TestConfigSaveInTheIngestSectionIsRefusedOnItsField(t *testing.T) {
	h := cfgWithIngestEntry()
	h.ops.err = platformconfig.RequireWritableSection("ingest")
	body := h.do(t, cfgAdmin(), req{
		method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, multipart: true,
		form: cfgSaveForm("section", "ingest", "property", "vendor-feed", "plainValue", "{}"),
	})
	mustContain(t, body, `id="cfg-form-section-err"`, `holds ingestion settings, which are changed on the ingestion`)
	mustNotContain(t, body, "The property was saved")
}

// Nor can one be edited or deleted by addressing it directly.
func TestConfigIngestEntriesAreNotEditedOrDeletedHere(t *testing.T) {
	h := cfgWithIngestEntry()
	rec := h.rec(t, cfgAdmin(), req{target: "/ui/platform/config/edit?app=platform&section=ingest&property=vendor-feed&client=clt_acme"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "holds ingestion settings")

	rec = h.rec(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/config/delete?app=platform&section=INGEST&property=vendor-feed&client=clt_acme"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, h.cfg.deleted, "nothing is deleted")
}

// A value the cell cuts off opens a dialog with all of it, indented when it is
// JSON; an ingestion's settings link from the dialog to the ingestions that use
// them. A secret has no such dialog (its value is not read here), nor does a
// value the read service withheld, nor a short one.
func TestConfigValueCellOpensTheWholeValueInADialog(t *testing.T) {
	h := cfgWithIngestEntry()
	long := strings.Repeat("a long plain value ", 12)
	h.cfg.rows = append(h.cfg.rows,
		cfgRowFixture("pfc_10", "branding", "tagline", platformconfig.ValuePlain, long),
		cfgRowFixture("pfc_11", platformconfig.ReservedSection, "hidden-feed", platformconfig.ValuePlain, ""),
	)
	h.cfg.rows[len(h.cfg.rows)-1].Hidden = true
	body := h.do(t, cfgAdmin(), req{target: "/ui/platform/config", datastar: true})

	// The JSON entry: a button in the cell, a dialog with the indented value, and the link to its ingestions.
	mustContain(t, body, `data-testid="cfg-value-open-pfc_9"`, `id="cfg-value-pfc_9"`, `class="fc-code-block"`,
		`<code>{`, `  &#34;host&#34;: &#34;sftp.vendor.example&#34;`, // indented, one key to a line
		`data-testid="cfg-value-ingestions-pfc_9"`, `href="/ui/ingestions?application=platform&amp;client=clt_acme"`)
	// A long plain value shows whole in the dialog although the cell cuts it.
	mustContain(t, body, `data-testid="cfg-value-open-pfc_10"`, `id="cfg-value-pfc_10"`, long)
	assert.NotContains(t, body, `data-testid="cfg-value-ingestions-pfc_10"`, "only an ingestion's settings link to ingestions")

	// No dialog for a short value, a secret or a withheld value.
	mustNotContain(t, body, `id="cfg-value-pfc_1"`, `id="cfg-value-pfc_2"`, `id="cfg-value-pfc_3"`, `id="cfg-value-pfc_11"`, cfgSecretValue)
	mustContain(t, body, `data-testid="cfg-hidden"`)
}
