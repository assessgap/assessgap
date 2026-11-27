package live

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fuzzAPI answers the first request (the subject's assessed values) with a
// fuzzed CSV body and every later one with just a header, recording each
// query's $where so the test can check what county-supplied text reached it.
type fuzzAPI struct {
	first []byte
	mu    sync.Mutex
	calls int
	where []string
}

func (a *fuzzAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	q := r.URL.Query()
	a.mu.Lock()
	a.calls++
	n := a.calls
	a.where = append(a.where, q.Get("$where"))
	a.mu.Unlock()
	body := []byte(strings.ReplaceAll(q.Get("$select"), " ", "") + "\n")
	if n == 1 {
		body = a.first
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

var (
	soqlLiteral = regexp.MustCompile(`'(?:[^']|'')*'`)
	nbhdLiteral = regexp.MustCompile(`nbhd(?:_code)? = '([^']*)'`)
	blockLit    = regexp.MustCompile(`starts_with\(pin, '([^']*)'\)`)
	fiveDigits  = regexp.MustCompile(`^[0-9]{5}$`)
	sevenDigits = regexp.MustCompile(`^[0-9]{7}$`)
)

// FuzzFetchCountyRows feeds live mode arbitrary county CSV for the subject's
// assessed values. Fetch must not panic or hang, and nothing the county sends
// may reach a later query except as a quoted literal of the expected shape:
// a 5-digit neighborhood and 7-digit PIN blocks.
func FuzzFetchCountyRows(f *testing.F) {
	const hdr = "pin,year,class,nbhd,mailed_tot,certified_tot,board_tot,mailed_bldg,mailed_land,certified_bldg,certified_land,board_bldg,board_land\n"
	for _, s := range []string{
		hdr + "13331030130000,2026,203,71280,30000,,,25000,5000,,,,\n",
		hdr + "13331030130000,2026.0,203,71280,30000,31000,,,,,,,\n",
		hdr + "13331030130000,2026,203,71280' OR '1'='1,30000,,,,,,,,\n",
		hdr + "13331030130000,2026,203,7128,30000,,,,,,,,\n",
		hdr + "13331030130000,abc,203,71280,30000,,,,,,,,\n",
		hdr + "13331030130000,2026,203,71280,30000,,,,,,,,\n1333'),--,2026,203,71280,1,,,,,,,,\n",
		hdr + "\"13331030130000\",\"2026\",\"203\",\"71280\",\"3e4\"\n",
		hdr,
		"",
		"pin\n13331030130000\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		api := &fuzzAPI{first: body}
		fe := &Fetcher{Domain: "county.test", HTTP: &http.Client{Transport: api}}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := fe.Fetch(ctx, "13331030130000", 0)
		if err == nil {
			res.Close()
		}
		api.mu.Lock()
		defer api.mu.Unlock()
		for _, w := range api.where[1:] {
			if strings.Contains(soqlLiteral.ReplaceAllString(w, ""), "'") {
				t.Fatalf("unbalanced quote in $where %q", w)
			}
			for _, m := range nbhdLiteral.FindAllStringSubmatch(w, -1) {
				if !fiveDigits.MatchString(m[1]) {
					t.Fatalf("neighborhood %q reached $where %q", m[1], w)
				}
			}
			for _, m := range blockLit.FindAllStringSubmatch(w, -1) {
				if !sevenDigits.MatchString(m[1]) {
					t.Fatalf("PIN block %q reached $where %q", m[1], w)
				}
			}
		}
	})
}

// FuzzSQLQuote checks a quoted value is one SoQL string literal that reads
// back as the value.
func FuzzSQLQuote(f *testing.F) {
	for _, s := range []string{"", "'", "''", "71280", "a'b", `\'`, "' OR '1'='1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		q := sqlQuote(s)
		if m := soqlLiteral.FindString(q); m != q {
			t.Fatalf("sqlQuote(%q) = %q is not one literal (matched %q)", s, q, m)
		}
		if back := strings.ReplaceAll(q[1:len(q)-1], "''", "'"); back != s {
			t.Fatalf("sqlQuote(%q) = %q reads back as %q", s, q, back)
		}
	})
}
