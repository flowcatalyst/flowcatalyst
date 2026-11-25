package pages

import (
	"cmp"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/framework/go/ui/kit"
)

// sortedOpts returns a copy of opts ordered by label, ignoring case: a
// search select shows its options in the order given, so a record list is
// sorted where the control is built.
func sortedOpts(opts []kit.Option) []kit.Option {
	out := slices.Clone(opts)
	slices.SortStableFunc(out, func(a, b kit.Option) int {
		return cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
	})
	return out
}

// filterSearch is a record filter in a list's Filters popover: a form-mode
// search select that draws its own label, with a leading "none" radio (the
// placeholder) that clears the filter. change is the handler on every radio
// (a live filter's fetch, or the auto-submit of a plain form).
func filterSearch(name, label, all, signal string, opts []kit.Option, selected string, change string) kit.SearchSelectProps {
	return kit.SearchSelectProps{
		ID: "filter-" + name, Name: name, Label: label, Placeholder: all, Signal: signal,
		Options: sortedOpts(opts), Selected: selected,
		OptionAttrs: templ.Attributes{"data-on:change": change},
	}
}

// fieldSearch is a record chooser inside a kit.Field: embedded, form mode,
// its label the control's hidden accessible name. The caller sets Required,
// Disabled, Invalid, DescribedBy and Placeholder.
func fieldSearch(id, name, label, signal string, opts []kit.Option, selected string) kit.SearchSelectProps {
	return kit.SearchSelectProps{
		ID: id, Name: name, Label: label, Signal: signal, Embedded: true,
		Options: sortedOpts(opts), Selected: selected,
	}
}

// recordField fills in the state a form field carries: the placeholder (the
// "none" choice of an optional field, the prompt of a required one), and its
// required, disabled and invalid state with the id of its help or error text.
func recordField(p kit.SearchSelectProps, placeholder string, required, disabled, invalid bool, describedBy string) kit.SearchSelectProps {
	p.Placeholder, p.Required, p.Disabled, p.Invalid, p.DescribedBy = placeholder, required, disabled, invalid, describedBy
	return p
}

// fieldAppSelect is the application chooser of a worker dialog.
func fieldAppSelect(id, signal string, opts []kit.Option, selected string, invalid bool, describedBy string) kit.SearchSelectProps {
	return recordField(fieldSearch(id, "app", "Application", signal, opts, selected), "", true, false, invalid, describedBy)
}

// edmProviderSelect is the identity provider chooser of the email domain form;
// the form's flags follow the chosen provider.
func edmProviderSelect(f edmForm, key string) kit.SearchSelectProps {
	p := recordField(fieldSearch(f.id(key), key, "Identity Provider", "edmFormIdpQ", f.Providers, f.ProviderID),
		"Select an identity provider", true, false, f.fieldError(key) != "", f.describedBy(key))
	p.OptionAttrs = templ.Attributes{"data-bind:edm-form-idp": true}
	return p
}

// ocPortalClientSelect is the portal owner client chooser of the OAuth client
// form: its choice drives $ocFormPortal and re-fetches the portal app field.
func ocPortalClientSelect(f ocForm, key string) kit.SearchSelectProps {
	p := recordField(fieldSearch(f.id(key), key, "Portal owner client", "ocFormPortalQ", f.ClientOptions, f.PortalClient),
		"Not a portal client", false, false, f.fieldError(key) != "", f.describedBy(key))
	p.OptionAttrs = templ.Attributes{"data-bind:oc-form-portal": true, "data-on:change": ocPortalChange}
	return p
}

// permAppSelect is the application chooser of the permission form: its choice
// drives the preview ($permApp).
func permAppSelect(f permsCreateForm, key string) kit.SearchSelectProps {
	p := recordField(fieldSearch(f.id(key), key, "Application", "permFormAppQ", f.Apps, f.Application),
		"Select application", true, false, f.fieldError(key) != "", f.describedBy(key))
	p.OptionAttrs = templ.Attributes{"data-bind:perm-app": true}
	return p
}
