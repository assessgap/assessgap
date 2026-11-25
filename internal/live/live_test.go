package live

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/store"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

// TestDatasetWhere checks the block filter is joined inside the dataset's own
// year filter: without the parentheses, the OR list would escape the year limit.
func TestDatasetWhere(t *testing.T) {
	d, err := dataset("bor_appeals", blockWhere([]string{"1328100", "1328101"}))
	if err != nil {
		t.Fatal(err)
	}
	want := "(tax_year >= '2021') AND (starts_with(pin, '1328100') OR starts_with(pin, '1328101'))"
	if d.Where != want {
		t.Errorf("Where = %q\nwant    %q", d.Where, want)
	}
	if _, err := dataset("nope", ""); err == nil {
		t.Error("unknown dataset: no error")
	}
}

// TestChunks guards the PIN-block batching: a lost remainder would silently
// drop homes from the comparison pool, and an empty chunk would become an
// empty filter that downloads a whole dataset.
func TestChunks(t *testing.T) {
	tests := []struct {
		name string
		xs   []string
		want [][]string
	}{
		{"none", nil, nil},
		{"remainder", []string{"a", "b", "c", "d", "e", "f", "g"}, [][]string{{"a", "b", "c"}, {"d", "e", "f"}, {"g"}}},
	}
	for _, tt := range tests {
		if got := chunks(tt.xs, 3); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: chunks = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestColumnsCoverBuild checks that every column build.sql reads from a raw
// table is in the live column list for that dataset, so a live build sees
// the same inputs as a full one.
func TestColumnsCoverBuild(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, d := range ingest.Datasets {
		if err := emptyRaw(db, d.Name); err != nil {
			t.Fatal(err)
		}
	}
	if err := ingest.Build(context.Background(), db, t.Logf); err != nil {
		t.Fatalf("build.sql on empty live tables: %v", err)
	}
}

// TestLiveMatchesDatabase runs live mode against a fake county API that
// answers its queries from the fixture's raw tables, and requires the live
// report to equal the report from the fixture database, field for field.
// It exercises every round of Fetch (year and neighborhood, PIN blocks,
// merging parts, empty datasets) without the network.
func TestLiveMatchesDatabase(t *testing.T) {
	db := fixture.Open(t)
	api := &fakeAPI{db: db}
	f := &Fetcher{Domain: "county.test", HTTP: &http.Client{Transport: api}}
	e := comps.New(db)
	pins := []string{
		"13331030130000", // Jefferson, mailed
		"24294030030000", // Worth, certified, recorded sales
		"16171280200000", // Oak Park, few comps after widening
		"13331120411001", // condo (unsupported)
	}
	for _, pin := range pins {
		t.Run(pin, func(t *testing.T) {
			res, err := f.Fetch(context.Background(), pin, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Close()
			got, err := comps.New(res.DB).Report(context.Background(), pin, 0)
			if err != nil {
				t.Fatal(err)
			}
			want, err := e.Report(context.Background(), pin, 0)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("live report differs from the database's:\nlive %+v\ndb   %+v", got, want)
			}
			if res.Stats.Calls < 4 {
				t.Errorf("%d API calls; want every round to run", res.Stats.Calls)
			}
		})
	}
	if _, err := f.Fetch(context.Background(), "99999999999999", 0); err == nil {
		t.Error("unknown PIN: no error")
	}
}

// fakeAPI serves Socrata CSV queries from raw_<dataset> tables. It
// understands the SoQL live mode sends: =, >=, AND, OR, parentheses and
// starts_with(pin, '...').
type fakeAPI struct{ db *sql.DB }

var startsWith = regexp.MustCompile(`starts_with\(pin, '([0-9]+)'\)`)

func (a *fakeAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/resource/"), ".csv")
	name := ""
	for _, d := range ingest.Datasets {
		if d.ID == id {
			name = d.Name
		}
	}
	if name == "" {
		return nil, fmt.Errorf("fake API: unknown dataset %q", id)
	}
	q := r.URL.Query()
	cols := strings.Split(q.Get("$select"), ", ")
	where := startsWith.ReplaceAllString(q.Get("$where"), "pin LIKE '$1%'")
	if where == "" {
		where = "1"
	}
	limit, _ := strconv.Atoi(q.Get("$limit"))
	offset, _ := strconv.Atoi(q.Get("$offset"))
	rows, err := a.db.Query(fmt.Sprintf(`SELECT "%s" FROM raw_%s WHERE %s ORDER BY rowid LIMIT %d OFFSET %d`,
		strings.Join(cols, `", "`), name, where, limit, offset))
	if err != nil {
		return nil, fmt.Errorf("fake API: %w", err)
	}
	defer rows.Close()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(cols); err != nil {
		return nil, err
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	rec := make([]string, len(cols))
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range vals {
			rec[i] = v.String
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	w.Flush()
	return &http.Response{StatusCode: 200, Body: io.NopCloser(&buf), Header: http.Header{}, Request: r}, nil
}
