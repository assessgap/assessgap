package cli

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/format"
)

func runTownship(ctx context.Context, env *Env, args []string) error {
	fs := newFlags(env, "township", "[-on YYYY-MM-DD] [name]")
	on := fs.String("on", "", "report status as of this date (default today)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	now := time.Now()
	if *on != "" {
		if now, err = time.Parse("2006-01-02", *on); err != nil {
			return &exitError{2, "-on: want YYYY-MM-DD"}
		}
		now = now.Add(12 * time.Hour) // midday, so the date is the same in Chicago
	}
	if name := joinArgs(pos); name != "" {
		t, ok := bor.LookupTownship(name)
		if !ok {
			return &exitError{1, fmt.Sprintf("no Cook County township named %q. Run \"assessgap township\" for the list.", name)}
		}
		fmt.Fprintf(env.Stdout, "%s Township, Board of Review: %s\n", t, windowText(t, now))
		return nil
	}
	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Township\tStatus\tOpens\tCloses\tEvidence due")
	for _, t := range bor.Townships {
		st := bor.StatusOf(t, now)
		if st.Group == nil {
			fmt.Fprintf(tw, "%s\t%s\t\t\t\n", t, stateName(st))
			continue
		}
		g := st.Group
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t, stateName(st), day(g.Open), day(g.Close), day(g.EvidenceDue))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "\nBoard of Review %d session, as posted on %s (checked %s). \"Not posted\" townships can pre-register at %s\n",
		bor.SessionYear, bor.DatesURL, bor.ScheduleAsOf, bor.PortalURL)
	return nil
}

func stateName(st bor.Status) string {
	switch st.State {
	case bor.Open:
		return "open (" + format.DaysLeft(st.DaysLeft) + ")"
	case bor.Upcoming:
		return "upcoming"
	case bor.Closed:
		return "closed"
	}
	return "not posted"
}
