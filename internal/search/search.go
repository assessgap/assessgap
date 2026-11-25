// Package search finds Cook County parcels by PIN or street address. The
// website's search box and the assessgap CLI both use it.
package search

import (
	"context"
	"database/sql"
	"strings"
	"unicode"
)

// Match is one search result.
type Match struct {
	PIN     string
	Address string
	City    string
	Class   string
}

// Kind is a short, human label for the parcel's class.
func (m Match) Kind() string { return ClassKind(m.Class) }

// MaxMatches caps how many results Find returns.
const MaxMatches = 8

// Cook County addresses are stored upper-case with USPS abbreviations
// ("5555 W BELMONT AVE UNIT 2"). Map what people type onto that form.
var abbrev = map[string]string{
	"NORTH": "N", "SOUTH": "S", "EAST": "E", "WEST": "W",
	"STREET": "ST", "AVENUE": "AVE", "AV": "AVE", "ROAD": "RD", "DRIVE": "DR",
	"BOULEVARD": "BLVD", "LANE": "LN", "COURT": "CT", "PLACE": "PL",
	"PARKWAY": "PKWY", "TERRACE": "TER", "CIRCLE": "CIR", "HIGHWAY": "HWY",
	"APT": "UNIT", "APARTMENT": "UNIT", "STE": "UNIT", "SUITE": "UNIT",
}

var directions = map[string]bool{"N": true, "S": true, "E": true, "W": true}

// NormalizeQuery turns free text into an address prefix. The last token may be
// partial (the user is still typing), so it is only abbreviated when it is a
// complete word we know.
func NormalizeQuery(q string) string {
	q = strings.ToUpper(q)
	q = strings.ReplaceAll(q, "#", " UNIT ")
	if i := strings.Index(q, ","); i >= 0 { // drop ", CHICAGO IL 60641"
		q = q[:i]
	}
	q = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '/' {
			return r
		}
		return ' '
	}, q)
	toks := strings.Fields(q)
	for i, t := range toks {
		if a, ok := abbrev[t]; ok {
			toks[i] = a
		}
	}
	return strings.Join(toks, " ")
}

// ParsePIN recognizes a PIN typed with or without dashes or spaces. A 10-digit
// PIN is the parcel without its unit suffix, so it maps to "...0000".
func ParsePIN(q string) (string, bool) {
	var digits strings.Builder
	for _, r := range strings.TrimSpace(q) {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '-' || r == ' ':
		default:
			return "", false
		}
	}
	switch d := digits.String(); len(d) {
	case 14:
		return d, true
	case 10:
		return d + "0000", true
	}
	return "", false
}

// FormatPIN renders a 14-digit PIN as 13-28-100-027-0000.
func FormatPIN(p string) string {
	if len(p) != 14 {
		return p
	}
	return p[0:2] + "-" + p[2:4] + "-" + p[4:7] + "-" + p[7:10] + "-" + p[10:14]
}

const prefixSQL = `
SELECT pin, address, COALESCE(city, ''), COALESCE(class, '')
FROM parcel
WHERE address >= ? AND address < ?
ORDER BY address, pin
LIMIT ?`

const pinSQL = `
SELECT pin, COALESCE(address, ''), COALESCE(city, ''), COALESCE(class, '')
FROM parcel WHERE pin = ?`

// Find looks parcels up by PIN or address prefix. If an address without a
// street direction finds nothing ("5555 BELMONT"), it retries with each one.
func Find(ctx context.Context, db *sql.DB, q string) ([]Match, error) {
	if pin, ok := ParsePIN(q); ok {
		return query(ctx, db, pinSQL, pin)
	}
	n := NormalizeQuery(q)
	if len(n) < 3 {
		return nil, nil
	}
	ms, err := prefix(ctx, db, n)
	if err != nil || len(ms) > 0 {
		return ms, err
	}
	toks := strings.SplitN(n, " ", 2)
	if len(toks) < 2 || directions[strings.SplitN(toks[1], " ", 2)[0]] {
		return nil, nil
	}
	for _, d := range []string{"N", "S", "E", "W"} {
		more, err := prefix(ctx, db, toks[0]+" "+d+" "+toks[1])
		if err != nil {
			return nil, err
		}
		ms = append(ms, more...)
	}
	if len(ms) > MaxMatches {
		ms = ms[:MaxMatches]
	}
	return ms, nil
}

// Exact returns the one match whose address is exactly the normalized query
// (so "5555 W Belmont Ave" picks the house over "... UNIT 2"), if there is one.
func Exact(q string, ms []Match) (Match, bool) {
	n := NormalizeQuery(q)
	var found []Match
	for _, m := range ms {
		if m.Address == n {
			found = append(found, m)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return Match{}, false
}

func prefix(ctx context.Context, db *sql.DB, p string) ([]Match, error) {
	// Addresses are ASCII, so DEL (0x7f) sorts after every character we store.
	return query(ctx, db, prefixSQL, p, p+"\x7f", MaxMatches)
}

func query(ctx context.Context, db *sql.DB, q string, args ...any) ([]Match, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.PIN, &m.Address, &m.City, &m.Class); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ClassKind gives a plain-English label for an Assessor class.
func ClassKind(c string) string {
	switch {
	case c == "299":
		return "Condo"
	case c == "211" || c == "212":
		return "Multi-family"
	case c == "210" || c == "295":
		return "Townhome"
	case len(c) == 3 && c[0] == '2' && c != "200" && c != "201" && c != "241":
		return "Home"
	case c == "":
		return ""
	default:
		return "Other"
	}
}
