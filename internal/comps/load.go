package comps

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ErrNotFound means the PIN has no assessment for the requested year.
var ErrNotFound = errors.New("comps: PIN/year not found")

// homeColumns selects a Home. AV is the certified value when the Assessor has
// certified the year, else the mailed (first-pass) value; the building and
// land split come from the same stage.
const homeColumns = `
	a.pin,
	COALESCE(p.address, ''),
	a.class,
	COALESCE(a.certified, a.mailed),
	CASE WHEN a.certified IS NOT NULL THEN a.certified_bldg ELSE a.mailed_bldg END,
	CASE WHEN a.certified IS NOT NULL THEN a.certified_land ELSE a.mailed_land END,
	r.bldg_sf, r.land_sf, r.yrblt, r.rooms, r.beds, r.fbath, r.hbath,
	r.type_resd, r.ext_wall, r.bsmt, r.bsmt_fin, r.air, r.frpl, r.garage, r.num_cards,
	p.lat, p.lon`

// subjectSQL loads the subject, its location, and whether its AV is certified.
// res_char is LEFT JOINed so out-of-scope parcels still produce a report.
const subjectSQL = `
SELECT` + homeColumns + `,
	a.nbhd,
	COALESCE(p.city, ''),
	COALESCE(p.township_name, ''),
	a.certified IS NOT NULL
FROM assessment a
LEFT JOIN res_char r ON r.pin = a.pin
LEFT JOIN parcel   p ON p.pin = a.pin
WHERE a.pin = ? AND a.year = ?`

// poolSQL loads every in-scope home in a neighborhood for a year. Filtering
// and ranking happen in Go so one query serves every subject in the neighborhood.
var poolSQL = `
SELECT` + homeColumns + `
FROM assessment a
JOIN res_char      r ON r.pin = a.pin
LEFT JOIN parcel   p ON p.pin = a.pin
WHERE a.year = ?
  AND a.nbhd = ?
  AND a.class IN ('` + strings.Join(ScopeClasses(), "','") + `')`

const latestYearSQL = `
SELECT MAX(year) FROM assessment
WHERE pin = ? AND COALESCE(certified, mailed) IS NOT NULL`

const nbhdStatsSQL = `
SELECT pro_se, n_appeals, n_decrease, win_rate, COALESCE(avg_cut_pct, 0), first_year, last_year
FROM nbhd_bor_stats
WHERE nbhd = ?
ORDER BY pro_se DESC`

const salesSQL = `
SELECT sale_date, price, COALESCE(deed_type, '')
FROM sale
WHERE pin = ?
ORDER BY sale_date DESC
LIMIT 5`

type subjectRow struct {
	Home
	Nbhd, City, Township string
	Certified            bool
	HasChars             bool
}

type scanner interface{ Scan(...any) error }

// scanHome reads homeColumns. Missing characteristics come back as zero values;
// hasChars reports whether a res_char row existed.
func scanHome(s scanner, extra ...any) (h Home, hasChars bool, err error) {
	var (
		av, bldgAV, landAV, bldg, land, lat, lon sql.NullFloat64
		yrblt, rooms, beds, fb, hb, frpl, cards  sql.NullInt64
		typ, wall, bsmt, fin, air, garage        sql.NullString
	)
	dest := append([]any{&h.PIN, &h.Address, &h.Class, &av, &bldgAV, &landAV,
		&bldg, &land, &yrblt, &rooms, &beds, &fb, &hb, &typ, &wall, &bsmt, &fin, &air, &frpl, &garage, &cards,
		&lat, &lon}, extra...)
	if err := s.Scan(dest...); err != nil {
		return h, false, err
	}
	h.AV, h.BldgAV, h.LandAV = av.Float64, bldgAV.Float64, landAV.Float64
	h.BldgSF, h.LandSF = bldg.Float64, land.Float64
	h.YearBuilt, h.Rooms, h.Beds = int(yrblt.Int64), int(rooms.Int64), int(beds.Int64)
	h.FullBaths, h.HalfBaths, h.NumCards = int(fb.Int64), int(hb.Int64), int(cards.Int64)
	h.TypeResd, h.ExtWall, h.Basement = typ.String, wall.String, bsmt.String
	h.BsmtFin, h.Air, h.Fireplaces, h.Garage = fin.String, air.String, int(frpl.Int64), garage.String
	if lat.Valid && lon.Valid && lat.Float64 != 0 && lon.Float64 != 0 {
		h.Lat, h.Lon = lat.Float64, lon.Float64
	}
	// A $0 building value happens on vacant or newly split parcels; such a
	// home has no building rate to compare.
	h.HasSplit = bldgAV.Valid && landAV.Valid && bldgAV.Float64 > 0
	if h.BldgSF > 0 && h.HasSplit {
		h.BldgAVPerSF = h.BldgAV / h.BldgSF
	}
	return h, cards.Valid, nil
}

func (e *Engine) latestYear(ctx context.Context, pin string) (int, error) {
	var y sql.NullInt64
	if err := e.db.QueryRowContext(ctx, latestYearSQL, pin).Scan(&y); err != nil {
		return 0, err
	}
	if !y.Valid {
		return 0, ErrNotFound
	}
	return int(y.Int64), nil
}

func (e *Engine) loadSubject(ctx context.Context, pin string, year int) (subjectRow, error) {
	var s subjectRow
	var nbhd sql.NullString
	h, ok, err := scanHome(e.db.QueryRowContext(ctx, subjectSQL, pin, year),
		&nbhd, &s.City, &s.Township, &s.Certified)
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	s.Home, s.HasChars, s.Nbhd = h, ok, nbhd.String
	return s, err
}

// loadPool returns all in-scope homes in nbhd for year, cached per engine.
func (e *Engine) loadPool(ctx context.Context, nbhd string, year int) ([]Home, error) {
	key := poolKey{nbhd, year}
	e.mu.Lock()
	pool, ok := e.pools[key]
	e.mu.Unlock()
	if ok {
		return pool, nil
	}
	rows, err := e.db.QueryContext(ctx, poolSQL, year, nbhd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		h, _, err := scanHome(rows)
		if err != nil {
			return nil, err
		}
		pool = append(pool, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if e.CachePools {
		e.mu.Lock()
		e.pools[key] = pool
		e.mu.Unlock()
	}
	return pool, nil
}

func (e *Engine) loadNbhdStats(ctx context.Context, nbhd string) ([]NbhdStat, error) {
	rows, err := e.db.QueryContext(ctx, nbhdStatsSQL, nbhd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NbhdStat
	for rows.Next() {
		var s NbhdStat
		if err := rows.Scan(&s.ProSe, &s.Appeals, &s.Decreases, &s.WinRate, &s.AvgCutPct, &s.FirstYear, &s.LastYear); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (e *Engine) loadSales(ctx context.Context, pin string) ([]Sale, error) {
	rows, err := e.db.QueryContext(ctx, salesSQL, pin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sale
	for rows.Next() {
		var s Sale
		if err := rows.Scan(&s.Date, &s.Price, &s.DeedType); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
