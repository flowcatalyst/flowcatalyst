package appupdate

import (
	"errors"
	"strings"
)

// The protocol's signature headers are Expo SFV dictionaries (RFC 8941
// structured-field dictionaries): expo-expect-signature on the request
// (`sig, keyid="main", alg="rsa-v1_5-sha256"`) and expo-signature on a part.

// SFItem is one bare item of a dictionary member. Only the kinds the
// signature headers use are kept apart; inner lists are refused.
type SFItem struct {
	// Kind is "string", "token", "boolean" or "integer".
	Kind string
	Str  string
	Bool bool
	Int  int64
}

// errSFV is every parse failure: the caller answers one fixed error.
var errSFV = errors.New("not a structured-field dictionary")

// maxSFVHeader bounds what is parsed.
const maxSFVHeader = 1024

// ParseSFVDictionary parses an RFC 8941 dictionary of bare items (parameters
// are parsed and dropped). Duplicate keys keep the last value, as the RFC says.
func ParseSFVDictionary(s string) (map[string]SFItem, error) {
	if len(s) > maxSFVHeader {
		return nil, errSFV
	}
	p := sfParser{s: s}
	out := map[string]SFItem{}
	p.skipSP()
	if p.done() {
		return out, nil
	}
	for {
		key, err := p.key()
		if err != nil {
			return nil, err
		}
		item := SFItem{Kind: "boolean", Bool: true}
		if p.peek() == '=' {
			p.i++
			if item, err = p.bareItem(); err != nil {
				return nil, err
			}
		}
		if err := p.params(); err != nil {
			return nil, err
		}
		out[key] = item
		p.skipOWS()
		if p.done() {
			return out, nil
		}
		if p.peek() != ',' {
			return nil, errSFV
		}
		p.i++
		p.skipOWS()
		if p.done() {
			return nil, errSFV // trailing comma
		}
	}
}

type sfParser struct {
	s string
	i int
}

func (p *sfParser) done() bool { return p.i >= len(p.s) }

func (p *sfParser) peek() byte {
	if p.done() {
		return 0
	}
	return p.s[p.i]
}

func (p *sfParser) skipSP() {
	for !p.done() && p.s[p.i] == ' ' {
		p.i++
	}
}

func (p *sfParser) skipOWS() {
	for !p.done() && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

func isLcalpha(c byte) bool { return c >= 'a' && c <= 'z' }
func isDigit(c byte) bool   { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool   { return isLcalpha(c) || (c >= 'A' && c <= 'Z') }

func (p *sfParser) key() (string, error) {
	c := p.peek()
	if !isLcalpha(c) && c != '*' {
		return "", errSFV
	}
	start := p.i
	for !p.done() {
		c := p.s[p.i]
		if !isLcalpha(c) && !isDigit(c) && c != '_' && c != '-' && c != '.' && c != '*' {
			break
		}
		p.i++
	}
	return p.s[start:p.i], nil
}

func (p *sfParser) params() error {
	for p.peek() == ';' {
		p.i++
		p.skipSP()
		if _, err := p.key(); err != nil {
			return err
		}
		if p.peek() == '=' {
			p.i++
			if _, err := p.bareItem(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *sfParser) bareItem() (SFItem, error) {
	c := p.peek()
	switch {
	case c == '"':
		return p.str()
	case c == '?':
		p.i++
		switch p.peek() {
		case '1':
			p.i++
			return SFItem{Kind: "boolean", Bool: true}, nil
		case '0':
			p.i++
			return SFItem{Kind: "boolean", Bool: false}, nil
		}
		return SFItem{}, errSFV
	case c == '-' || isDigit(c):
		return p.integer()
	case isAlpha(c) || c == '*':
		start := p.i
		for !p.done() && isTokenChar(p.s[p.i]) {
			p.i++
		}
		return SFItem{Kind: "token", Str: p.s[start:p.i]}, nil
	}
	return SFItem{}, errSFV
}

func isTokenChar(c byte) bool {
	return isAlpha(c) || isDigit(c) || strings.IndexByte("!#$%&'*+-.^_`|~:/", c) >= 0
}

func (p *sfParser) str() (SFItem, error) {
	p.i++ // opening quote
	var b strings.Builder
	for !p.done() {
		c := p.s[p.i]
		p.i++
		switch {
		case c == '\\':
			if p.done() {
				return SFItem{}, errSFV
			}
			n := p.s[p.i]
			if n != '"' && n != '\\' {
				return SFItem{}, errSFV
			}
			b.WriteByte(n)
			p.i++
		case c == '"':
			return SFItem{Kind: "string", Str: b.String()}, nil
		case c < 0x20 || c > 0x7e:
			return SFItem{}, errSFV
		default:
			b.WriteByte(c)
		}
	}
	return SFItem{}, errSFV
}

func (p *sfParser) integer() (SFItem, error) {
	neg := false
	if p.peek() == '-' {
		neg = true
		p.i++
	}
	if !isDigit(p.peek()) {
		return SFItem{}, errSFV
	}
	var n int64
	digits := 0
	for !p.done() && isDigit(p.s[p.i]) {
		n = n*10 + int64(p.s[p.i]-'0')
		p.i++
		digits++
		if digits > 15 {
			return SFItem{}, errSFV
		}
	}
	if p.peek() == '.' {
		return SFItem{}, errSFV // decimals are not used by these headers
	}
	if neg {
		n = -n
	}
	return SFItem{Kind: "integer", Int: n}, nil
}

// SFString serialises s as an RFC 8941 string item. s must be printable
// ASCII (key ids and base64 are); the caller guarantees it.
func SFString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
