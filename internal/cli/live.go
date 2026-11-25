package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/live"
	"github.com/assessgap/assessgap/internal/search"
)

func newFetcher(env *Env) *live.Fetcher {
	return &live.Fetcher{Domain: ingest.Domain, AppToken: env.Getenv("SOCRATA_APP_TOKEN")}
}

// useLiveByDefault reports whether a check should go to the API because no
// database was asked for and none exists at the default path. Only PINs can
// be checked live; an address still needs a local database (and gets the
// "how to build one" error).
func useLiveByDefault(env *Env, dbFlag, path, q string) bool {
	if dbFlag != "" || env.Getenv("ASSESSGAP_DB") != "" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return false
	}
	_, isPIN := search.ParsePIN(q)
	return isPIN
}

// lookupLive builds the report for a PIN from the open data API.
func lookupLive(ctx context.Context, env *Env, q string, year int) (*comps.Report, error) {
	pin, ok := search.ParsePIN(q)
	if !ok {
		return nil, &exitError{2, "live mode needs a 14-digit PIN; searching by address needs a local database (assessgap db sync)"}
	}
	res, err := newFetcher(env).Fetch(ctx, pin, year)
	if errors.Is(err, live.ErrNotFound) {
		return nil, fmt.Errorf("PIN %s has no assessed value in the county's open data", search.FormatPIN(pin))
	}
	if err != nil {
		return nil, fmt.Errorf("fetching from the county's open data API: %w", err)
	}
	defer res.Close()
	fmt.Fprintf(env.Stderr, "Fetched live from the county's open data API (%d requests, %.1f MB, %s). Use -db for a local database.\n",
		res.Stats.Calls, float64(res.Stats.Bytes)/1e6, res.Stats.Duration.Round(100*time.Millisecond))
	return comps.New(res.DB).Report(ctx, pin, year)
}
