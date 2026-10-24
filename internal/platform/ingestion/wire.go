package ingestion

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// strictDecode decodes one JSON value into v and rejects unknown fields and
// trailing data. Every definition goes through it: a misspelt setting must
// fail loudly instead of being ignored.
func strictDecode(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

// marshalTagged writes v (a struct that marshals to an object) with "kind"
// first, so a union member reads {"kind":"csv","header":true}.
func marshalTagged(kind string, v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	head := `{"kind":` + strconv.Quote(kind)
	if string(body) == "{}" {
		return []byte(head + "}"), nil
	}
	return []byte(head + "," + string(body[1:])), nil
}

// variant decodes a union member: the raw object without its "kind", strictly,
// into the concrete type C, returned as the union interface U.
func variant[U, C any](raw []byte) (U, error) {
	var c C
	if err := strictDecode(raw, &c); err != nil {
		var zero U
		return zero, err
	}
	return any(c).(U), nil
}

// decodeTagged reads a union ({"kind": ..., fields}) by its kind. what names
// the union in errors ("source", "split").
func decodeTagged[U any](data []byte, what string, members map[string]func([]byte) (U, error)) (U, error) {
	var zero U
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return zero, fmt.Errorf("%s must be an object with a kind", what)
	}
	var kind string
	if err := json.Unmarshal(fields["kind"], &kind); err != nil || kind == "" {
		return zero, fmt.Errorf("%s needs a kind", what)
	}
	decode, ok := members[kind]
	if !ok {
		return zero, fmt.Errorf("%s kind %q is not known", what, kind)
	}
	delete(fields, "kind")
	rest, err := json.Marshal(fields)
	if err != nil {
		return zero, err
	}
	u, err := decode(rest)
	if err != nil {
		return zero, fmt.Errorf("%s %s: %w", what, kind, err)
	}
	return u, nil
}

// SecretRef names a secret; a definition never holds a value. The runtime
// resolves the name against the ingestion's own client.
type SecretRef struct {
	Name string `json:"name"`
}

// Duration is an ISO 8601 duration on the wire ("PT10S", "PT5M", "PT1H"), as
// definition files write them.
type Duration time.Duration

var durationPattern = regexp.MustCompile(`(?i)^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)(?:\.(\d{1,9}))?S)?)?$`)

// ParseDuration reads an ISO 8601 duration of days, hours, minutes and
// seconds. A negative one is refused.
func ParseDuration(s string) (time.Duration, error) {
	m := durationPattern.FindStringSubmatch(s)
	if m == nil || s == "P" || strings.HasSuffix(strings.ToUpper(s), "T") {
		return 0, fmt.Errorf("%q is not a duration such as PT30S", s)
	}
	var d time.Duration
	units := []time.Duration{24 * time.Hour, time.Hour, time.Minute, time.Second}
	for i, unit := range units {
		if m[i+1] == "" {
			continue
		}
		n, err := strconv.ParseInt(m[i+1], 10, 64)
		if err != nil || n > int64(time.Duration(1<<62)/unit) {
			return 0, fmt.Errorf("%q is too long", s)
		}
		d += time.Duration(n) * unit
	}
	if m[5] != "" {
		frac := (m[5] + "000000000")[:9]
		n, _ := strconv.ParseInt(frac, 10, 64)
		d += time.Duration(n)
	}
	return d, nil
}

// String formats as Java's Duration does: hours, not days ("PT24H").
func (d Duration) String() string {
	t := time.Duration(d)
	if t == 0 {
		return "PT0S"
	}
	var b strings.Builder
	b.WriteString("PT")
	if h := t / time.Hour; h > 0 {
		b.WriteString(strconv.FormatInt(int64(h), 10) + "H")
	}
	t %= time.Hour
	if m := t / time.Minute; m > 0 {
		b.WriteString(strconv.FormatInt(int64(m), 10) + "M")
	}
	t %= time.Minute
	if t > 0 {
		secs := strconv.FormatFloat(t.Seconds(), 'f', -1, 64)
		b.WriteString(secs + "S")
	}
	return b.String()
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return errors.New("a duration is a string such as PT30S")
	}
	parsed, err := ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}
