// Package comps compares a home's assessed value per square foot with similar
// homes in its Assessor neighborhood and judges whether a uniformity appeal has
// a case. It works only with assessed values (AV); it never estimates market value.
package comps

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// Engine builds reports from a database produced by ingest.Build ("assessgap db sync").
type Engine struct {
	db *sql.DB

	// CachePools keeps each neighborhood's candidate pool in memory after the
	// first lookup. Turn it on for batch runs; leave it off for a long-lived
	// server where the database is refreshed underneath.
	CachePools bool

	mu    sync.Mutex
	pools map[poolKey][]Home
}

type poolKey struct {
	nbhd string
	year int
}

// New returns an Engine reading from db.
func New(db *sql.DB) *Engine {
	return &Engine{db: db, pools: map[poolKey][]Home{}}
}

// Report compares pin with similar homes for a tax year. year 0 means the
// latest year in which the PIN has an assessed value.
//
// Out-of-scope parcels get a Report with Unsupported set and a nil error.
// A PIN with no assessment for the year returns ErrNotFound.
func (e *Engine) Report(ctx context.Context, pin string, year int) (*Report, error) {
	if year == 0 {
		y, err := e.latestYear(ctx, pin)
		if err != nil {
			return nil, err
		}
		year = y
	}
	sub, err := e.loadSubject(ctx, pin, year)
	if err != nil {
		return nil, fmt.Errorf("load subject %s/%d: %w", pin, year, err)
	}
	r := &Report{
		PIN: pin, Year: year, Address: sub.Address, City: sub.City,
		Township: sub.Township, Nbhd: sub.Nbhd, Class: sub.Class,
		AVStage: "mailed", Disclaimer: Disclaimer,
	}
	if sub.Certified {
		r.AVStage = "certified"
	}
	if r.Unsupported = unsupported(sub, year); r.Unsupported != "" {
		return r, nil
	}
	subject := sub.Home
	r.Subject = &subject

	pool, err := e.loadPool(ctx, sub.Nbhd, year)
	if err != nil {
		return nil, fmt.Errorf("load pool %s/%d: %w", sub.Nbhd, year, err)
	}
	comps, search := selectComps(subject, pool)
	st := computeStats(subject, comps)
	r.Comps, r.Search, r.Stats = comps, &search, &st
	r.Evidence = evidenceComps(subject, comps)
	r.Verdict, r.Reasons = decide(r)
	r.Flags = append(characteristicFlags(subject, pool), landFlag(subject, st)...)

	if r.NbhdAppeals, err = e.loadNbhdStats(ctx, sub.Nbhd); err != nil {
		return nil, fmt.Errorf("nbhd stats: %w", err)
	}
	if r.Sales, err = e.loadSales(ctx, pin); err != nil {
		return nil, fmt.Errorf("sales: %w", err)
	}
	return r, nil
}

// unsupported explains why a subject can't be compared, or returns "".
func unsupported(s subjectRow, year int) string {
	switch {
	case !InScope(s.Class):
		return unsupportedReason(s.Class)
	case !s.HasChars:
		return "No building characteristics on record for this PIN."
	case s.NumCards > 1:
		return "This parcel has more than one building on record; multi-building parcels aren't supported yet."
	case s.BldgSF <= 0:
		return "No building square footage on record for this PIN. Please verify your home's details with the Assessor."
	case s.AV <= 0:
		return fmt.Sprintf("No assessed value on record for tax year %d.", year)
	case !s.HasSplit:
		return fmt.Sprintf("The Assessor's tax year %d record for this PIN doesn't split the assessed value into "+
			"building and land, so the building can't be compared with similar homes.", year)
	case s.Nbhd == "":
		return "No Assessor neighborhood on record for this PIN."
	}
	return ""
}
