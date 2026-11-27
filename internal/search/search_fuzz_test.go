package search

import (
	"strings"
	"testing"
	"unicode"
)

// FuzzParsePIN checks the PIN parser on arbitrary input. A PIN it accepts is
// looked up directly, so it must be exactly 14 digits made only of the
// digits typed (a wrong digit shows someone else's home), and the formatted
// form must parse back to the same PIN.
func FuzzParsePIN(f *testing.F) {
	for _, s := range []string{
		"13281000270000", "13-28-100-027-0000", " 13 28 100 027 0000 ", "1328100027",
		"13-28-100-027", "123", "", "13281000270000X", "١٣٢٨١٠٠٠٢٧٠٠٠٠", "13281000270000\n",
		"--------------", "132810002700001",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, q string) {
		pin, ok := ParsePIN(q)
		if !ok {
			if pin != "" {
				t.Fatalf("ParsePIN(%q) = %q, false; want empty", q, pin)
			}
			return
		}
		if len(pin) != 14 || strings.Trim(pin, "0123456789") != "" {
			t.Fatalf("ParsePIN(%q) = %q, not 14 ASCII digits", q, pin)
		}
		typed := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, q)
		if pin != typed && pin != typed+"0000" {
			t.Fatalf("ParsePIN(%q) = %q, not the digits typed (%q)", q, pin, typed)
		}
		if again, ok := ParsePIN(FormatPIN(pin)); !ok || again != pin {
			t.Fatalf("ParsePIN(FormatPIN(%q)) = %q, %v", pin, again, ok)
		}
	})
}

// FuzzNormalizeQuery checks the address normalizer. Its output becomes an
// SQL range prefix and is compared with stored addresses, so it must be
// stable (normalizing twice changes nothing; otherwise Exact, which
// normalizes again, could miss the home Find just matched) and hold only
// letters, digits, single spaces, '-' and '/'.
func FuzzNormalizeQuery(f *testing.F) {
	for _, s := range []string{
		"3134 N Luna Ave", "5555 west belmont avenue #2, Chicago IL 60641", "5555 BELMONT",
		"", "   ", "#", "apt 3", "ß", "ǅ", "ﬀ street", "123 N. Main St.", "1/2 Elm", "\x00\xff",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, q string) {
		n := NormalizeQuery(q)
		if again := NormalizeQuery(n); again != n {
			t.Fatalf("NormalizeQuery not stable for %q: %q, then %q", q, n, again)
		}
		if n != strings.Join(strings.Fields(n), " ") {
			t.Fatalf("NormalizeQuery(%q) = %q: stray spaces", q, n)
		}
		for _, r := range n {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' && r != '-' && r != '/' {
				t.Fatalf("NormalizeQuery(%q) = %q: holds %q", q, n, r)
			}
		}
	})
}
