package principal

import (
	"slices"
	"testing"
)

func TestParseEmailList(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		wantEmails  []string
		wantInvalid []string
	}{
		{"empty", "  \n\t ", nil, nil},
		{"commas", "a@x.com,b@x.com, c@x.com", []string{"a@x.com", "b@x.com", "c@x.com"}, nil},
		{"semicolons", "a@x.com;b@x.com; c@x.com", []string{"a@x.com", "b@x.com", "c@x.com"}, nil},
		{"newlines tabs spaces", "a@x.com\nb@x.com\r\nc@x.com\td@x.com e@x.com", []string{"a@x.com", "b@x.com", "c@x.com", "d@x.com", "e@x.com"}, nil},
		{"mixed separators and empties", ",;a@x.com,,;\n\nb@x.com;", []string{"a@x.com", "b@x.com"}, nil},
		{"lowercases", "Ann@X.COM", []string{"ann@x.com"}, nil},
		{"dedupes keeping first order", "b@x.com a@x.com B@x.com a@x.com", []string{"b@x.com", "a@x.com"}, nil},
		{"name in angle brackets", "Ann Smith <Ann@x.com>", []string{"ann@x.com"}, nil},
		{"quoted name with comma", `"Name, Jr" <a@b.com>`, []string{"a@b.com"}, nil},
		{"list of angle entries", `Ann Smith <a@x.com>, "Bob, Jr" <b@x.com>; Cy <c@x.com>`, []string{"a@x.com", "b@x.com", "c@x.com"}, nil},
		{"outlook last-first names", `Smith, Ann <ann@x.com>; Jones, Bob <bob@x.com>`, []string{"ann@x.com", "bob@x.com"}, nil},
		{"plain address before a named one", `a@x.com, Smith, Ann <ann@x.com>`, []string{"a@x.com", "ann@x.com"}, nil},
		{"plain then named", "a@x.com b@x.com Cy Dee <c@x.com>", []string{"a@x.com", "b@x.com", "c@x.com"}, nil},
		{"quoted entries", `"a@x.com", 'b@x.com'`, []string{"a@x.com", "b@x.com"}, nil},
		{"angle without name", "<a@x.com>", []string{"a@x.com"}, nil},
		{"invalid no at", "a@x.com, nope, B@@x.com", []string{"a@x.com"}, []string{"nope", "B@@x.com"}},
		{"invalid deduped keeps case", "Foo foo Foo", nil, []string{"Foo", "foo"}},
		{"invalid bare at", "@ a@ @b", nil, []string{"@", "a@", "@b"}},
		{"invalid angle entry", "Ann <not-an-email>", nil, []string{"not-an-email"}},
		{"order across valid and invalid", "z@x.com junk a@x.com", []string{"z@x.com", "a@x.com"}, []string{"junk"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotE, gotI := ParseEmailList(tc.in)
			if gotE == nil || gotI == nil {
				t.Fatalf("results must be non-nil slices: %v %v", gotE, gotI)
			}
			if !slices.Equal(gotE, nz(tc.wantEmails)) {
				t.Errorf("emails = %q, want %q", gotE, tc.wantEmails)
			}
			if !slices.Equal(gotI, nz(tc.wantInvalid)) {
				t.Errorf("invalid = %q, want %q", gotI, tc.wantInvalid)
			}
		})
	}
}

func nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
