package principal

import (
	"regexp"
	"strings"
)

// angleEntry matches the address part of an Outlook/Gmail "Name <addr>" entry.
var angleEntry = regexp.MustCompile(`<([^<>]*)>`)

// ParseEmailList turns pasted text into a clean address list. It accepts
// commas, semicolons and any whitespace as separators, and the "Ann Smith
// <ann@x.com>" form (the display name is dropped, even when quoted and
// containing commas). Addresses are trimmed, unquoted, lower-cased and
// de-duplicated in first-seen order; entries that are not a single
// local@domain go to invalid (trimmed, original case, de-duplicated).
func ParseEmailList(text string) (emails, invalid []string) {
	emails, invalid = []string{}, []string{}
	seenEmail := map[string]struct{}{}
	seenInvalid := map[string]struct{}{}

	add := func(raw string) {
		entry := strings.Trim(strings.TrimSpace(raw), `"'`)
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return
		}
		if strings.Count(entry, "@") != 1 || strings.HasPrefix(entry, "@") || strings.HasSuffix(entry, "@") {
			if _, ok := seenInvalid[entry]; !ok {
				seenInvalid[entry] = struct{}{}
				invalid = append(invalid, entry)
			}
			return
		}
		entry = strings.ToLower(entry)
		if _, ok := seenEmail[entry]; !ok {
			seenEmail[entry] = struct{}{}
			emails = append(emails, entry)
		}
	}
	addPlain := func(s string) {
		for _, f := range strings.FieldsFunc(s, isEmailSeparator) {
			add(f)
		}
	}

	pos := 0
	for _, m := range angleEntry.FindAllStringSubmatchIndex(text, -1) {
		addPlain(stripDisplayName(text[pos:m[0]]))
		add(text[m[2]:m[3]])
		pos = m[1]
	}
	addPlain(text[pos:])
	return emails, invalid
}

func isEmailSeparator(r rune) bool {
	switch r {
	case ',', ';', ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return r > 0x7f && strings.ContainsRune("\u0085   ", r)
}

// stripDisplayName removes the display name that sits directly before an
// "<addr>": a quoted string (which may contain commas), else every word since
// the last semicolon/newline that is not itself an address. Commas do not end
// the name: Outlook writes "Smith, Ann <ann@x.com>; ...", so "Smith" is part of
// it. (A stray non-address word there is dropped rather than reported.)
func stripDisplayName(pre string) string {
	trimmed := strings.TrimRight(pre, " \t\r\n")
	if strings.HasSuffix(trimmed, `"`) {
		if k := strings.LastIndex(trimmed[:len(trimmed)-1], `"`); k >= 0 {
			return trimmed[:k]
		}
	}
	cut := strings.LastIndexAny(trimmed, ";\n")
	head, tail := trimmed[:cut+1], trimmed[cut+1:]
	var addrs []string
	for _, w := range strings.FieldsFunc(tail, isEmailSeparator) {
		if strings.Contains(w, "@") {
			addrs = append(addrs, w)
		}
	}
	return head + " " + strings.Join(addrs, " ")
}
