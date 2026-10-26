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
