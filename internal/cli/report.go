package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/assessgap/assessgap/internal/packet"
)

func runReport(ctx context.Context, env *Env, args []string) error {
	fs := newFlags(env, "report", "[-db path | -live] [-year N] -o out.pdf <address-or-PIN>")
	db := fs.String("db", "", "county database (default $ASSESSGAP_DB, else "+DefaultDB+")")
	year := fs.Int("year", 0, "tax year (default: latest year with an assessed value)")
	out := fs.String("o", "", "output PDF path (required)")
	live := fs.Bool("live", false, "fetch this home's data from the county's open data API instead of a local database (PIN only)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	q := joinArgs(pos)
	if q == "" || *out == "" {
		fs.Usage()
		return errUsage
	}
	r, err := lookup(ctx, env, *db, *live, q, *year)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = packet.Render(&buf, r, time.Now())
	if errors.Is(err, packet.ErrUnsupported) {
		return fmt.Errorf("no evidence report for this property: %s", r.Unsupported)
	}
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, *out)
	return nil
}
