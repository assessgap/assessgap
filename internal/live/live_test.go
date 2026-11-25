package live

import (
	"context"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/store"
)

func TestBlockWhere(t *testing.T) {
	tests := []struct {
		blocks []string
		want   string
	}{
		{[]string{"1328100"}, "starts_with(pin, '1328100')"},
		{[]string{"1328100", "1328101"}, "starts_with(pin, '1328100') OR starts_with(pin, '1328101')"},
	}
	for _, tt := range tests {
		if got := blockWhere(tt.blocks); got != tt.want {
			t.Errorf("blockWhere(%q) = %q, want %q", tt.blocks, got, tt.want)
		}
	}
}

func TestChunks(t *testing.T) {
	tests := []struct {
		n, size int
		want    []int
	}{
		{0, 3, nil},
		{3, 3, []int{3}},
		{7, 3, []int{3, 3, 1}},
	}
	for _, tt := range tests {
		xs := make([]string, tt.n)
		var got []int
		for _, c := range chunks(xs, tt.size) {
			got = append(got, len(c))
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("chunks(%d, %d) = %v, want %v", tt.n, tt.size, got, tt.want)
		}
	}
}

func TestSQLQuote(t *testing.T) {
	if got := sqlQuote("a'b"); got != "'a''b'" {
		t.Errorf("sqlQuote = %s", got)
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

// TestFetchLive builds a live database for one sample PIN from the real API.
// It skips without network, in -short mode, and in CI (so CI doesn't depend
// on the county's servers). With the sample database built, it also checks
// the live report matches the local one.
func TestFetchLive(t *testing.T) {
	if testing.Short() || os.Getenv("CI") != "" {
		t.Skip("network test")
	}
	conn, err := net.DialTimeout("tcp", ingest.Domain+":443", 3*time.Second)
	if err != nil {
		t.Skipf("no network: %v", err)
	}
	conn.Close()

	const pin = "13281000290000"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	f := &Fetcher{Domain: ingest.Domain, AppToken: os.Getenv("SOCRATA_APP_TOKEN"), Log: t.Logf}
	res, err := f.Fetch(ctx, pin, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Close()
	t.Logf("%+v", res.Stats)
	got, err := comps.New(res.DB).Report(ctx, pin, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Unsupported != "" || got.Stats == nil || got.Stats.NumComps < 8 {
		t.Fatalf("live report: unsupported %q, stats %+v", got.Unsupported, got.Stats)
	}

	const sample = "../../data-sample/cook.db"
	if _, err := os.Stat(sample); err != nil {
		return
	}
	sdb, err := store.OpenReadOnly(sample)
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	want, err := comps.New(sdb).Report(ctx, pin, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The sample covers only PINs starting 1328, so neighborhood homes outside
	// it can make the comparisons differ; the subject must still match.
	if !reflect.DeepEqual(got.Subject, want.Subject) {
		t.Errorf("subject differs:\nlive   %+v\nsample %+v", got.Subject, want.Subject)
	}
}
