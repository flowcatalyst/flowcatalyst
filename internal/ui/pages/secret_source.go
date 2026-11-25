package pages

import (
	"errors"
	"strings"
	"unicode"

	"github.com/flowcatalyst/framework/go/ui/kit"

	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// A secret source is where a secret setting's value comes from: the value
// itself (stored encrypted by the platform), or a reference to a parameter in
// AWS Parameter Store or a secret in AWS Secrets Manager, which the platform
// resolves each time it hands the secret out (docs/external-secret-references.md).
// The control posts <name>Source (the choice), <name> (the value, a password
// input) and, per store, the reference's path without its scheme
// (secretSourceRefName).
const (
	secretSourceValue = "value"
	secretSourcePS    = "aws-ps"
	secretSourceSM    = "aws-sm"
)

// secretSourceOptions are the choices, in the order offered. Only the two AWS
// stores resolve; the platform refuses the others (SECRET_REFERENCE_UNSUPPORTED).
var secretSourceOptions = []kit.Option{
	{Value: secretSourceValue, Label: "Value (stored encrypted)"},
	{Value: secretSourcePS, Label: "AWS Parameter Store"},
	{Value: secretSourceSM, Label: "AWS Secrets Manager"},
}

// secretSourceScheme is the reference scheme of a store, "" for a value.
func secretSourceScheme(source string) string {
	switch source {
	case secretSourcePS:
		return secretref.SchemeParameterStore
	case secretSourceSM:
		return secretref.SchemeSecretsManager
	}
	return ""
}

// secretSourceOf splits a stored reference into its store and path, for the
// form shown again; a value that is not a reference is a value (never shown).
func secretSourceOf(stored string) (source, ref string) {
	switch {
	case strings.HasPrefix(stored, secretref.SchemeParameterStore):
		return secretSourcePS, strings.TrimPrefix(stored, secretref.SchemeParameterStore)
	case strings.HasPrefix(stored, secretref.SchemeSecretsManager):
		return secretSourceSM, strings.TrimPrefix(stored, secretref.SchemeSecretsManager)
	}
	return secretSourceValue, ""
}

// secretSourcePosted is what the control posted: the store chosen (a value
// when the choice is missing or unknown), the reference path as typed (kept
// for the form shown again; a reference is not a secret) and the value to
// store, which is the typed secret or the store's scheme and the path. A path
// pasted with its scheme is taken as it is. stored is "" when nothing was
// entered for the chosen store.
func secretSourcePosted(get func(string) string, name string) (source, ref, stored string) {
	source = get(name + "Source")
	scheme := secretSourceScheme(source)
	if scheme == "" {
		return secretSourceValue, "", get(name)
	}
	ref = strings.TrimSpace(get(secretSourceRefName(name, source)))
	ref = strings.TrimPrefix(ref, scheme)
	if ref == "" {
		return source, "", ""
	}
	return source, ref, scheme + ref
}

// secretSourceRefName is the form name of a store's reference input:
// "valueRefAwsPs" for the value "value" and Parameter Store.
func secretSourceRefName(name, source string) string { return name + "Ref" + signalPart(source) }

// secretSourceMissing is the field error of a source left empty.
func secretSourceMissing(source string) string {
	switch source {
	case secretSourcePS:
		return "Enter the parameter's name."
	case secretSourceSM:
		return "Enter the secret's name or ARN."
	}
	return "Enter the secret."
}

// secretSourceProps draws one control.
type secretSourceProps struct {
	// ID is the value input's id; the choice and the reference get it with
	// "-source" and "-ref".
	ID string
	// Name is the value's form name; the others are Name+"Source" and Name+"Ref".
	Name string
	// Signal is the camel-case base of the control's signals, unique per page.
	Signal string
	// Source and Ref are the store and path shown (a reference already stored,
	// or the one posted with a refused form). A value is never shown.
	Source, Ref string
	// ValuePlaceholder is the password input's placeholder (e.g. "Leave blank
	// to keep the current value").
	ValuePlaceholder string
	Invalid          bool
	DescribedBy      string
}

func (p secretSourceProps) source() string {
	if secretSourceScheme(p.Source) == "" {
		return secretSourceValue
	}
	return p.Source
}

// refFor is the path shown in a store's input: the reference's, in its own
// store's input only.
func (p secretSourceProps) refFor(source string) string {
	if p.source() == source {
		return p.Ref
	}
	return ""
}

// sourceSignal is the signal the choice is bound to.
func (p secretSourceProps) sourceSignal() string { return p.Signal + "Source" }

// initSignals sets the choice's signal to the store shown each time the
// control is drawn: a dialog drawn again for another key would otherwise keep
// the last choice (a bound signal that exists wins over the select's own
// value). The sources are constants, so the expression is safe as written.
func (p secretSourceProps) initSignals() string {
	return "{" + p.sourceSignal() + ": '" + p.source() + "'}"
}

// bindAttr is the choice's data-bind attribute: Datastar takes the kebab-case
// key and binds the camel-case signal.
func (p secretSourceProps) bindAttr() string { return "data-bind:" + kebab(p.sourceSignal()) }

func (p secretSourceProps) showWhen(source string) string {
	return "$" + p.sourceSignal() + " === '" + source + "'"
}

// signalPart is s as part of a camel-case signal name: each run of letters
// and digits capitalised, everything else dropped ("sftp.credential" ->
// "SftpCredential").
func signalPart(s string) string {
	var b strings.Builder
	up := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			up = true
			continue
		}
		if up {
			r = unicode.ToUpper(r)
			up = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// secretReferenceOn moves a refused reference's message from the form to the
// field that posted it, when exactly one field posted a reference (with more,
// the message names the reference and stays on the form).
func (e *formErrors) secretReferenceOn(err error, fields []string) {
	ue, ok := errors.AsType[*usecase.Error](err)
	if !ok || !strings.HasPrefix(ue.Code, "SECRET_REFERENCE_") || len(fields) != 1 || e.Form == "" {
		return
	}
	msg := e.Form
	e.Form = ""
	e.field(fields[0], msg)
}
