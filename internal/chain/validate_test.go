package chain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		act, by string
		wantAct string
		wantBy  string
		wantErr string
	}{
		{"plain sentence", "I carried groceries for a neighbour", "Olena", "I carried groceries for a neighbour", "Olena", ""},
		{"whitespace collapses", "  I   carried\n\tgroceries  ", " Olena ", "I carried groceries", "Olena", ""},
		{"control and invisible characters go", "I carried\x00 gro​ceries today", "", "I carried groceries today", "", ""},
		{"non-latin counts in characters", strings.Repeat("я", MaxActLength), "", strings.Repeat("я", MaxActLength), "", ""},
		{"too short", "Hi there", "", "", "", MsgTooShort},
		{"too long", strings.Repeat("a", MaxActLength+1), "", "", "", MsgTooLong},
		{"name too long", "I carried groceries", strings.Repeat("n", MaxByLength+1), "", "", MsgNameTooLong},
		{"http link", "Donate at http://example.com today", "", "", "", MsgNoLinks},
		{"https link in any case", "See HTTPS://example.com now", "", "", "", MsgNoLinks},
		{"www link", "kindness at www.example.com", "", "", "", MsgNoLinks},
		{"link in the name", "I carried groceries for a neighbour", "www.spam.example", "", "", MsgNoLinks},
		{"blocked word", "I told him to fuck off nicely", "", "", "", MsgBlockedWord},
		{"blocked word in the name", "I carried groceries for a neighbour", "shit", "", "", MsgBlockedWord},
		{"whole words only", "Scunthorpe was lovely and the shiitake soup helped", "", "Scunthorpe was lovely and the shiitake soup helped", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			act, by, err := Validate(tc.act, tc.by)
			if tc.wantErr != "" {
				var verr *ValidationError
				if !errors.As(err, &verr) || verr.Message != tc.wantErr {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if act != tc.wantAct || by != tc.wantBy {
				t.Errorf("got %q / %q, want %q / %q", act, by, tc.wantAct, tc.wantBy)
			}
		})
	}
}
