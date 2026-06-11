package auth

import "testing"

type scopedItem struct {
	Name     string
	ClientID *string
}

func TestFilterClientScoped(t *testing.T) {
	t.Parallel()

	cl1, cl2 := "client-1", "client-2"
	items := []scopedItem{
		{Name: "platform", ClientID: nil},
		{Name: "mine", ClientID: &cl1},
		{Name: "theirs", ClientID: &cl2},
	}
	byClientID := func(i *scopedItem) *string { return i.ClientID }

	names := func(in []scopedItem) []string {
		out := make([]string, 0, len(in))
		for _, i := range in {
			out = append(out, i.Name)
		}
		return out
	}

	cases := []struct {
		name string
		ac   *AuthContext
		want []string
	}{
		{"anchor sees all", &AuthContext{Scope: ScopeAnchor}, []string{"platform", "mine", "theirs"}},
		{"client sees platform plus own", &AuthContext{Scope: ScopeClient, Clients: []string{cl1}}, []string{"platform", "mine"}},
		{"no clients sees platform only", &AuthContext{Scope: ScopeClient}, []string{"platform"}},
		{"nil context sees platform only", nil, []string{"platform"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := names(FilterClientScoped(tc.ac, items, byClientID))
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}

	if got := FilterClientScoped(nil, nil, byClientID); got == nil || len(got) != 0 {
		t.Errorf("FilterClientScoped(nil items) = %v, want non-nil empty slice", got)
	}
}
