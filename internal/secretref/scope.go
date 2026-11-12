package secretref

import (
	"log/slog"
	"strings"
)

// Owner is the row a secret belongs to: the coordinates a scoped allow-list
// entry is matched against. Application is the application's code. Client is
// the client's id (the "clt_..." value, not its identifier: every caller holds
// the id, where the identifier would be a lookup on a path that runs every few
// seconds). An empty field is "not supplied": an entry that names it never
// matches.
type Owner struct {
	Application string
	Client      string
}

const (
	placeholderApp    = "{app}"
	placeholderClient = "{client}"
)

func hasPlaceholder(entry string) bool {
	return strings.Contains(entry, placeholderApp) || strings.Contains(entry, placeholderClient)
}

// nextPlaceholder is the first placeholder in s: its start, its text and the
// owner's value for it; ok is false when there is none.
func nextPlaceholder(s string, owner Owner) (at int, ph, value string, ok bool) {
	a, c := strings.Index(s, placeholderApp), strings.Index(s, placeholderClient)
	switch {
	case a < 0 && c < 0:
		return 0, "", "", false
	case c < 0 || (a >= 0 && a < c):
		return a, placeholderApp, owner.Application, true
	default:
		return c, placeholderClient, owner.Client, true
	}
}

// matchEntry reports whether ref is admitted by one allow-list entry.
//
// An entry without a placeholder is a raw text prefix, as it always was. An
// entry with {app} and/or {client} matches when each placeholder, which must
// stand as a whole path segment of the entry (after a "/", before a "/" or the
// end), meets exactly one whole segment of ref (up to the next "/" or "#")
// equal to the owner's value. A placeholder the owner did not supply never
// matches; a segment that is "." or ".." never matches. anyOwner (the
// pre-check before the row is known) lets a placeholder meet any non-empty
// segment.
func matchEntry(entry, ref string, owner Owner, anyOwner bool) bool {
	if !hasPlaceholder(entry) {
		return strings.HasPrefix(ref, entry)
	}
	rest := entry
	for {
		at, ph, value, ok := nextPlaceholder(rest, owner)
		if !ok {
			return strings.HasPrefix(ref, rest)
		}
		lit := rest[:at]
		after := rest[at+len(ph):]
		if !strings.HasSuffix(lit, "/") || (after != "" && after[0] != '/') || !strings.HasPrefix(ref, lit) {
			return false
		}
		ref = ref[len(lit):]
		end := strings.IndexAny(ref, "/#")
		if end < 0 {
			end = len(ref)
		}
		seg := ref[:end]
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
		if !anyOwner && (value == "" || strings.Contains(value, "/") || seg != value) {
			return false
		}
		ref, rest = ref[end:], after
	}
}

// WarnLooseEntries logs one WARN per allow-list entry that admits more than
// its author likely meant: a plain entry that does not end in "/" (it also
// admits sibling names: "aws-sm://prod/data" admits "aws-sm://prod/data-admin/x"),
// and a placeholder that is not a whole path segment (it never matches). It
// does not change how anything matches.
func WarnLooseEntries(allow []string) {
	for _, e := range allow {
		switch {
		case !hasPlaceholder(e) && !strings.HasSuffix(e, "/"):
			slog.Warn("FC_SECRET_REFERENCE_ALLOW entry has no placeholder and does not end in '/': it also admits sibling names (an entry 'aws-sm://prod/data' admits 'aws-sm://prod/data-admin/x'); end it with '/' or scope it with {app}/{client}",
				"entry", e)
		case hasPlaceholder(e) && !wellFormedPlaceholders(e):
			slog.Warn("FC_SECRET_REFERENCE_ALLOW entry has a {app}/{client} placeholder that is not a whole path segment: it never matches; write it between slashes, e.g. 'aws-sm://prod/{app}/'",
				"entry", e)
		}
	}
}

func wellFormedPlaceholders(entry string) bool {
	rest := entry
	for {
		at, ph, _, ok := nextPlaceholder(rest, Owner{})
		if !ok {
			return true
		}
		after := rest[at+len(ph):]
		if !strings.HasSuffix(rest[:at], "/") || (after != "" && after[0] != '/') {
			return false
		}
		rest = after
	}
}
