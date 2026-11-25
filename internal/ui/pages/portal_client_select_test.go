package pages

import (
	"testing"

	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/stretchr/testify/require"
)

func TestClientSearchSelect(t *testing.T) {
	opts := []kit.Option{{Value: "clt_z", Label: "zeta"}, {Value: "clt_a", Label: "Acme"}, {Value: "clt_b", Label: "beta"}}
	p := clientSearchSelect("fw-client", "fwClientSearch", fieldWorkersPath, opts, "clt_b")

	// Listed by name, case ignored; the caller's slice is left alone.
	var labels []string
	for _, o := range p.Options {
		labels = append(labels, o.Label)
	}
	require.Equal(t, []string{"Acme", "beta", "zeta"}, labels)
	require.Equal(t, "clt_z", opts[0].Value)

	require.Equal(t, "beta", p.Summary())
	require.Equal(t, "/ui/field-workers?client=clt_a", p.Href("clt_a"))
	require.Equal(t, "/ui/field-workers?client=a%26b", p.Href("a&b"))

	p.Selected = ""
	require.Equal(t, "Select a client", p.Summary())
}
