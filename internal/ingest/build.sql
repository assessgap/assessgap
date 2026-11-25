-- Transforms raw_* tables (loaded by ingest.Load) into the typed tables the engine reads.
-- Run by "assessgap db build" / "db sync". Idempotent: drops and rebuilds.
-- Socrata numbers arrive as text, often with ".0" suffixes, so cast via REAL.

PRAGMA foreign_keys = OFF;

-- ---------------------------------------------------------------- parcels
DROP TABLE IF EXISTS parcel;
CREATE TABLE parcel (
  pin            TEXT PRIMARY KEY,
  class          TEXT,
  township_code  TEXT,
  township_name  TEXT,
  nbhd           TEXT,          -- assessor neighborhood code (5 digits: township + nbhd)
  tax_code       TEXT,
  municipality   TEXT,
  lat            REAL,
  lon            REAL,
  address        TEXT,
  city           TEXT,
  zip            TEXT
) WITHOUT ROWID;

INSERT INTO parcel
SELECT u.pin, u.class, u.township_code, u.township_name, u.nbhd_code, u.tax_code,
       u.tax_municipality_name, CAST(u.lat AS REAL), CAST(u.lon AS REAL),
       a.prop_address_full, a.prop_address_city_name, a.prop_address_zipcode_1
FROM raw_parcel_universe u
LEFT JOIN (
  SELECT pin, prop_address_full, prop_address_city_name, prop_address_zipcode_1,
         ROW_NUMBER() OVER (PARTITION BY pin ORDER BY CAST(year AS REAL) DESC) rn
  FROM raw_addresses
) a ON a.pin = u.pin AND a.rn = 1;

-- ---------------------------------------------------------------- assessed values
DROP TABLE IF EXISTS assessment;
CREATE TABLE assessment (
  pin        TEXT NOT NULL,
  year       INTEGER NOT NULL,
  class      TEXT,
  nbhd       TEXT,
  mailed     REAL,   -- Assessor first-pass total AV
  certified  REAL,   -- Assessor certified total AV (after Assessor appeals)
  board      REAL,   -- Board of Review final total AV
  PRIMARY KEY (pin, year)
) WITHOUT ROWID;

INSERT OR REPLACE INTO assessment
SELECT pin, CAST(CAST(year AS REAL) AS INTEGER), class, nbhd,
       CAST(mailed_tot AS REAL), CAST(certified_tot AS REAL), CAST(board_tot AS REAL)
FROM raw_assessed_values;

-- ---------------------------------------------------------------- residential characteristics
-- One row per PIN for the latest year. Multi-card PINs: sum building SF across cards,
-- take descriptive attributes from card 1, and flag them.
DROP TABLE IF EXISTS res_char;
CREATE TABLE res_char (
  pin         TEXT PRIMARY KEY,
  year        INTEGER,
  class       TEXT,
  bldg_sf     REAL,
  land_sf     REAL,
  yrblt       INTEGER,
  beds        INTEGER,
  fbath       INTEGER,
  hbath       INTEGER,
  rooms       INTEGER,
  apts        TEXT,
  type_resd   TEXT,
  ext_wall    TEXT,
  bsmt        TEXT,
  bsmt_fin    TEXT,
  garage      TEXT,
  air         TEXT,
  frpl        INTEGER,
  cnst_qlty   TEXT,
  repair_cnd  TEXT,
  num_cards   INTEGER
) WITHOUT ROWID;

WITH latest AS (
  SELECT pin, MAX(CAST(year AS REAL)) y FROM raw_res_chars GROUP BY pin
), cards AS (
  SELECT r.* FROM raw_res_chars r JOIN latest l ON l.pin = r.pin AND CAST(r.year AS REAL) = l.y
), agg AS (
  SELECT pin, SUM(CAST(char_bldg_sf AS REAL)) bldg_sf, COUNT(*) n FROM cards GROUP BY pin
), card1 AS (
  SELECT *, ROW_NUMBER() OVER (PARTITION BY pin ORDER BY CAST(card AS REAL)) rn FROM cards
)
INSERT INTO res_char
SELECT c.pin, CAST(CAST(c.year AS REAL) AS INTEGER), c.class, a.bldg_sf,
       CAST(c.char_land_sf AS REAL), CAST(CAST(c.char_yrblt AS REAL) AS INTEGER),
       CAST(CAST(c.char_beds AS REAL) AS INTEGER), CAST(CAST(c.char_fbath AS REAL) AS INTEGER),
       CAST(CAST(c.char_hbath AS REAL) AS INTEGER), CAST(CAST(c.char_rooms AS REAL) AS INTEGER),
       c.char_apts, c.char_type_resd, c.char_ext_wall, c.char_bsmt, c.char_bsmt_fin,
       c.char_gar1_size, c.char_air, CAST(CAST(c.char_frpl AS REAL) AS INTEGER),
       c.char_cnst_qlty, c.char_repair_cnd, a.n
FROM card1 c JOIN agg a ON a.pin = c.pin
WHERE c.rn = 1;

-- ---------------------------------------------------------------- condo characteristics
DROP TABLE IF EXISTS condo_char;
CREATE TABLE condo_char (
  pin          TEXT PRIMARY KEY,
  year         INTEGER,
  pin10        TEXT,      -- building key (first 10 digits)
  unit_sf      REAL,
  bldg_sf      REAL,
  beds         INTEGER,
  fbath        INTEGER,
  yrblt        INTEGER,
  is_parking   INTEGER,
  is_common    INTEGER,
  proration    REAL
) WITHOUT ROWID;

INSERT INTO condo_char
SELECT pin, y, pin10, unit_sf, bldg_sf, beds, fbath, yrblt, is_parking, is_common, proration FROM (
  SELECT pin, CAST(CAST(year AS REAL) AS INTEGER) y, pin10,
         CAST(char_unit_sf AS REAL) unit_sf, CAST(char_building_sf AS REAL) bldg_sf,
         CAST(CAST(char_bedrooms AS REAL) AS INTEGER) beds, CAST(CAST(char_full_baths AS REAL) AS INTEGER) fbath,
         CAST(CAST(char_yrblt AS REAL) AS INTEGER) yrblt,
         (is_parking_space = 'true') is_parking, (is_common_area = 'true') is_common,
         CAST(tieback_proration_rate AS REAL) proration,
         ROW_NUMBER() OVER (PARTITION BY pin ORDER BY CAST(year AS REAL) DESC, CAST(card AS REAL)) rn
  FROM raw_condo_chars
) WHERE rn = 1;

-- ---------------------------------------------------------------- arm's-length sales
DROP TABLE IF EXISTS sale;
CREATE TABLE sale (
  pin        TEXT NOT NULL,
  sale_date  TEXT NOT NULL,   -- YYYY-MM-DD
  price      REAL NOT NULL,
  doc_no     TEXT,
  deed_type  TEXT,
  class      TEXT,
  nbhd       TEXT
);

INSERT INTO sale
SELECT pin, substr(sale_date, 1, 10), CAST(sale_price AS REAL), doc_no, deed_type, class, nbhd
FROM raw_sales
WHERE COALESCE(is_multisale, 'false') = 'false'
  AND COALESCE(sale_filter_same_sale_within_365, 'false') = 'false'
  AND COALESCE(sale_filter_less_than_10k, 'false') = 'false'
  AND COALESCE(sale_filter_deed_type, 'false') = 'false'
  AND CAST(sale_price AS REAL) > 0;

CREATE INDEX sale_pin ON sale(pin);
CREATE INDEX sale_nbhd ON sale(nbhd, class, sale_date);

-- ---------------------------------------------------------------- Board of Review appeal outcomes
DROP TABLE IF EXISTS bor_appeal;
CREATE TABLE bor_appeal (
  pin         TEXT NOT NULL,
  tax_year    INTEGER NOT NULL,
  class       TEXT,        -- 3-digit Assessor class (source mixes '203' and '20300')
  majorclass  TEXT,
  appeal_type TEXT,
  result      TEXT,        -- Decrease / No Change / Increase
  pro_se      INTEGER,     -- 1 if filed by the taxpayer
  av_before   REAL,
  av_after    REAL,
  reason      TEXT
);

INSERT INTO bor_appeal
SELECT pin, CAST(CAST(tax_year AS REAL) AS INTEGER), substr(class, 1, 3), majorclass, appealtypedescription, result,
       COALESCE(attorneycode = 'Individual/Pro Se', 0),  -- 10 rows (TY2024) have no attorneycode
       CAST(assessor_totalvalue AS REAL), CAST(bor_totalvalue AS REAL),
       COALESCE(changereasondescription, nochangereasondescription)
FROM raw_bor_appeals;

CREATE INDEX bor_appeal_pin ON bor_appeal(pin, tax_year);

-- ---------------------------------------------------------------- Assessor appeal outcomes
DROP TABLE IF EXISTS ccao_appeal;
CREATE TABLE ccao_appeal (
  pin        TEXT NOT NULL,
  year       INTEGER NOT NULL,
  class      TEXT,
  status     TEXT,
  change     TEXT,
  agent      TEXT,
  av_before  REAL,
  av_after   REAL,
  reason     TEXT
);

INSERT INTO ccao_appeal
SELECT pin, CAST(CAST(year AS REAL) AS INTEGER), class, status, change, agent_name,
       CAST(mailed_tot AS REAL), CAST(certified_tot AS REAL), reason_desc1
FROM raw_ccao_appeals;

CREATE INDEX ccao_appeal_pin ON ccao_appeal(pin, year);

-- ---------------------------------------------------------------- neighborhood appeal stats
-- Residential (non-condo) BOR outcomes by assessor neighborhood, last 3 tax years.
-- This is the "how did appeals like yours turn out" number.
DROP TABLE IF EXISTS nbhd_bor_stats;
CREATE TABLE nbhd_bor_stats (
  nbhd            TEXT NOT NULL,
  pro_se          INTEGER NOT NULL,
  n_appeals       INTEGER,
  n_decrease      INTEGER,
  win_rate        REAL,
  avg_cut_pct     REAL,     -- among decreases, (before-after)/before
  first_year      INTEGER,
  last_year       INTEGER,
  PRIMARY KEY (nbhd, pro_se)
) WITHOUT ROWID;

INSERT INTO nbhd_bor_stats
SELECT p.nbhd, b.pro_se, COUNT(*),
       SUM(b.result = 'Decrease'),
       1.0 * SUM(b.result = 'Decrease') / COUNT(*),
       AVG(CASE WHEN b.result = 'Decrease' AND b.av_before > 0
                THEN (b.av_before - b.av_after) / b.av_before END),
       MIN(b.tax_year), MAX(b.tax_year)
FROM bor_appeal b
JOIN parcel p ON p.pin = b.pin
WHERE b.majorclass = 'Residential'
  AND b.class NOT IN ('299', '399')
  AND b.tax_year >= (SELECT MAX(tax_year) - 2 FROM bor_appeal)
GROUP BY p.nbhd, b.pro_se;

-- ---------------------------------------------------------------- comp-search indexes
CREATE INDEX IF NOT EXISTS parcel_nbhd ON parcel(nbhd, class);
CREATE INDEX IF NOT EXISTS parcel_address ON parcel(address);  -- prefix search on the site
CREATE INDEX IF NOT EXISTS assessment_year ON assessment(year, nbhd, class);

ANALYZE;
