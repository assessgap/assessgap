# Data inventory

What the county's open data offers for comparing homes, to inform configurable comparison rules (homeowners choosing which characteristics to match on, and how strictly). Checked **2026-11-24**.

- Part 1 lists every column of every dataset we ingest: its meaning (the portal's own column description, shortened), type, how often it is filled and how many distinct values it has for supported homes, and whether the engine uses it today.
- Part 2 lists other Cook County datasets that could add per-parcel attributes. We don't ingest them yet.
- Part 3 proposes a comparison config. It is not implemented.

Everything here compares **assessed values** and the characteristics the Assessor records. Nothing here estimates what a home would sell for.

## How the numbers were made

- **Meaning and type** come from the Socrata metadata API (`/api/views/<id>.json`), shortened to about 160 characters. For residential characteristics, the most common values and their shares are appended.
- **API rows** is `count(*)` on the dataset with no filter, and **last updated** is the portal's `rowsUpdatedAt`.
- **Filled** and **Distinct** are computed read-only on a full local build (see the README). They cover rows for **supported home classes** (202–210, 234, 278, 295) in the **latest year** of each dataset:
  - tax year 2026 for characteristics, assessed values, addresses and Assessor appeals;
  - tax year 2025 for Board of Review appeals;
  - 2026 so far for sales;
  - class 299, 2026, for condo characteristics, since condos aren't supported.

  The parcel universe has one row per parcel, so it is filtered by class only.
- "Filled" means not empty. It does not mean accurate: the Assessor fills some fields with defaults (for example, a missing full-bath count is set to 1).
- **Used today**:
  - **E**: the comparison engine uses it (choosing the pool, the similarity distance, the statistics, the data checks, or neighborhood appeal outcomes).
  - **R**: the report or the address search shows or uses it, but it doesn't affect the comparison.
  - **B**: built into a typed table but unused.
  - **–**: downloaded but not loaded.

## Summary

- **The engine uses 11 characteristics today**: building sq ft, year built, land sq ft, full and half baths, residence type, exterior wall, basement, number of buildings (cards), class and neighborhood. Bedrooms and rooms are used only in the data checks and the display.
- **Well filled and with real variety** (good candidates to offer as user choices): building sq ft, land sq ft, year built, bedrooms, rooms, full and half baths, residence type (5 values), exterior wall (4), basement type (4), basement finish (3), garage size (8), central air (2), attic type and finish (3 each), fireplaces, porch (3), garage attached (2).
- **Filled but almost constant** (99%+ one value; not useful for matching, and they could mislead): construction quality, repair condition, CDU (the portal itself says "not well maintained"), site desirability, use, apartments, commercial units.
- **Too sparse**: renovation (1.2% filled, and only ever "Yes").
- **Location**: today's only location rule is "same Assessor neighborhood". The parcel universe adds latitude/longitude, census tract and block group, municipality, school districts, ward and community area, flood and noise indicators, subdivision ID and a walkability score. Parcel proximity (Part 2) adds the distance to the 3 nearest neighbors, transit, major roads, railroads, parks and water. Any of these could support "within N feet" or "same subdivision" rules.
- **Assessed values**: the 2026 certified value is present for 47.6% of supported homes (the Assessor certifies township by township), and the Board of Review value for none yet. That is why reports say whether they use mailed or certified values.

## Part 1: datasets we ingest

### res_chars: Assessor - Residential Single and Multi-Family Improvement Characteristics (`x54s-btds`)

API rows: 30,656,402. Rows last updated 2026-11-15. Fill rates over 934,627 rows (2026).

| Column | Type | Meaning (from the portal) | Filled | Distinct | Used today |
|---|---|---|--:|--:|---|
| `pin` | text | Parcel Identification Number (PIN) | 100.0% | 931911 | E |
| `year` | number | Tax year | 100.0% | 1 | B (latest year kept) |
| `card` | number | Card number. Each card is an improvement/building on the parcel | 100.0% | 36 | E (multi-building exclusion) |
| `class` | text | Property class. Values: 203 (37%), 202 (12%), 234 (10%), 278 (9%), 205 (9%), 204 (6%), 295 (6%), 206 (5%) | 100.0% | 12 | B |
| `township_code` | text | Township code | 100.0% | 38 | – |
| `tieback_key_pin` | text | Tieback key PIN. Prorated properties (whose value is split across multiple PINs) have a "main" or key PIN | 4.7% | 22104 | – |
| `tieback_proration_rate` | number | Tieback proration rate. Prorated properties (whose value is split across multiple PINs) pay taxes on the proportion of value on their PIN. In other words,… | 100.0% | 108 | – |
| `card_proration_rate` | number | Card proration rate. Prorated parcels (whose value is split across multiple cards) pay taxes on the proportion of value on their card. In other words, assessed… | 100.0% | 103 | – |
| `cdu` | text | Condition, Desirability, and Utility code. Not well maintained. Values: AV (97%), GD (1%), VG (0%), FR (0%), PR (0%), EX (0%), VP (0%), UN (0%) | 98.8% | 8 | – |
| `pin_is_multicard` | checkbox | Multicard PIN. Indicates whether the parcel contains more than one building (ADU, coach house, etc.) Values: false (99%), true (1%) | 100.0% | 2 | – |
| `pin_num_cards` | number | Number of cards on this parcel. Each card is an improvement/building. Values: 1 (99%), 2 (1%), 3–36 (under 0.5%) | 100.0% | 12 | – |
| `pin_is_multiland` | checkbox | Multiland PIN. Indicates whether parcel has more than one landline. Values: false (100%), true (0%) | 100.0% | 2 | – |
| `pin_num_landlines` | number | Number of landlines on a parcel. The sum of all landline square footage should be equal to the total square footage of the parcel… Values: 1 (100%), 2–4 (0%) | 100.0% | 4 | – |
| `char_yrblt` | number | Year built | 100.0% | 188 | E (filter, distance, check) |
| `char_bldg_sf` | number | Building square feet. Square footage of the building, as measured from the exterior | 100.0% | 7478 | E (filter, distance, AV/sq ft) |
| `char_land_sf` | number | Land square feet. Square footage of the land (not just the building) of the property. Note that a single PIN can have multiple landlines, meaning it can be… | 100.0% | 31119 | E (distance) |
| `char_beds` | number | Number of bedrooms. Values: 3 (52%), 4 (25%), 2 (16%), 5 (5%), 6 (1%), other (under 0.5%) | 100.0% | 16 | R (check, display) |
| `char_rooms` | number | Rooms. Number of total rooms in the building (excluding baths). Not to be confused with bedrooms | 100.0% | 36 | R (check) |
| `char_fbath` | number | Full baths. Defined as having a bath or shower. If this value is missing, the default value is set to 1. Values: 1 (57%), 2 (34%), 3 (7%), 4 (2%) | 100.0% | 16 | E (distance, check) |
| `char_hbath` | number | Half baths. Defined as bathrooms without a shower or bathtub. Values: 0 (51%), 1 (47%), 2 (2%) | 99.7% | 13 | E (distance) |
| `char_frpl` | number | Fireplaces. Counted as the number of flues one can see from the outside of the building. Values: 0 (67%), 1 (28%), 2 (4%), 3 (1%) | 99.2% | 12 | B |
| `char_type_resd` | text | Type of residence. Values: 1 Story (44%), 2 Story (33%), 1.5 Story (11%), Split Level (10%), 3 Story + (2%) | 100.0% | 5 | E (distance) |
| `char_cnst_qlty` | text | Construction quality. Values: Average (99%), Deluxe (1%), Poor (0%) | 100.0% | 3 | B |
| `char_apts` | text | Apartments. Number of apartments for class 211 and 212 properties. Values: None (99%), Two–Six (under 0.5%) | 99.3% | 6 | B |
| `char_attic_fnsh` | text | Attic finish. Values: None (85%), Living Area (14%), Partial (0%) | 99.4% | 3 | – |
| `char_gar1_att` | text | Garage 1 attached. Values: No (65%), Yes (35%) | 99.8% | 2 | – |
| `char_gar1_area` | text | Is Garage 1 physically included within the building area? If yes, the garage area is subtracted from the building square feet calculation by the field agent. Values: No (89%), Yes (10%) | 99.8% | 2 | – |
| `char_gar1_size` | text | Garage 1 size. Values: 2 cars (58%), 1 cars (14%), 0 cars (12%), 1.5 cars (6%), 2.5 cars (5%), 3 cars (4%), 4 cars (0%), 3.5 cars (0%) | 99.9% | 8 | B |
| `char_gar1_cnst` | text | Garage 1 exterior wall material. Values: Frame (60%), Masonry (22%), Frame + Masonry (6%), Stucco (0%) | 87.8% | 4 | – |
| `char_attic_type` | text | Attic type. Values: None (67%), Full (18%), Partial (15%) | 100.0% | 3 | – |
| `char_bsmt` | text | Basement type. Values: Full (58%), Partial (24%), Slab (12%), Crawl (6%) | 99.9% | 4 | E (distance) |
| `char_ext_wall` | text | Exterior wall material. Values: Masonry (40%), Frame (31%), Frame + Masonry (27%), Stucco (2%) | 100.0% | 4 | E (distance) |
| `char_heat` | text | Central heating. Values: Warm Air Furnace (89%), Hot Water Steam (10%), Electric Heater (0%), None (0%) | 100.0% | 4 | – |
| `char_repair_cnd` | text | Repair condition. Values: Average (99%), Above Average (1%), Below Average (0%) | 100.0% | 3 | B |
| `char_bsmt_fin` | text | Basement finish. Values: Unfinished (65%), Formal Rec Room (35%), Apartment (0%) | 99.6% | 3 | B |
| `char_roof_cnst` | text | Roof material. Values: Shingle + Asphalt (95%), Tar + Gravel (3%), Shake (1%), Tile (1%), Other (1%), Slate (0%) | 100.0% | 6 | – |
| `char_use` | text | Single vs. multi-family use. Values: Single-Family (100%), Multi-Family (0%) | 100.0% | 2 | – |
| `char_site` | text | Site desirability. Values: Not Relevant To Value (99%), Beneficial To Value (1%), Detracts From Value (0%) | 100.0% | 3 | – |
| `char_ncu` | text | Number of commercial units on the parcel (the vast majority are for properties with class 212). Values: 0 (99%) | 98.9% | 6 | – |
| `char_renovation` | text | Renovation. Values: Yes (1%) | 1.2% | 1 | – |
| `char_porch` | text | Porch. Values: None (85%), Frame Enclosed (14%), Masonry Enclosed (1%) | 99.8% | 3 | – |
| `char_air` | text | Central air conditioning. Values: Central A/C (57%), No Central A/C (43%) | 100.0% | 2 | B |
| `char_tp_plan` | text | Design plan. Values: Stock Plan (69%), None (29%), Architect (2%) | 100.0% | 3 | – |
| `row_id` | text | Unique row key for API | 100.0% | 934627 | – |

Notes: "B" for `num_cards` means the engine counts the cards itself (one row per card) rather than reading `pin_num_cards`. Multi-card homes, about 1%, are excluded.

### assessed_values: Assessor - Assessed Values (`uzyt-m557`)

API rows: 50,671,458. Rows last updated 2026-11-15. Fill rates over 929,070 rows (2026).

| Column | Type | Meaning (from the portal) | Filled | Distinct | Used today |
|---|---|---|--:|--:|---|
| `pin` | text | Parcel Identification Number (PIN) | 100.0% | 929070 | E |
| `year` | number | Tax year | 100.0% | 1 | E |
| `class` | text | Property class | 100.0% | 12 | E (same class / group) |
| `township_code` | text | Township code | 100.0% | 38 | – |
| `township_name` | text | Township name | 100.0% | 38 | – |
| `nbhd` | text | Assessor neighborhood code, first two digits are township, last three are neighborhood | 100.0% | 847 | E (pool) |
| `mailed_bldg` | number | Assessor mailed building value | 100.0% | 103460 | – |
| `mailed_land` | number | Assessor mailed land value | 100.0% | 37699 | – |
| `mailed_tot` | number | Assessor mailed total value | 100.0% | 83385 | E (AV if not certified) |
| `mailed_hie` | number | Assessor mailed home improvement exemption value | 100.0% | 2247 | – |
| `certified_bldg` | number | Assessor certified building value | 47.6% | 94239 | – |
| `certified_land` | number | Assessor certified land value | 47.6% | 34942 | – |
| `certified_tot` | number | Assessor certified total value | 47.6% | 66381 | E (AV) |
| `certified_hie` | number | Assessor certified home improvement exemption value | 47.6% | 1677 | – |
| `board_bldg` | number | Board of Review certified building value | 0.0% | 0 | – |
| `board_land` | number | Board of Review certified land value | 0.0% | 0 | – |
| `board_tot` | number | Board of Review certified total value | 0.0% | 0 | B |
| `board_hie` | number | Board of Review certified home improvement exemption value | 0.0% | 0 | – |
| `row_id` | text | Unique row key for API | 100.0% | 929070 | – |

The building/land split (`*_bldg`, `*_land`) is filled and unused. A "building AV per sq ft" comparison, which leaves land out, would be a natural option.

### parcel_universe: Assessor - Parcel Universe (Current) (`pabr-t5kh`)

API rows: 1,864,469. Rows last updated 2026-11-15. Fill rates over 929,070 supported-class parcels.

| Column | Type | Meaning (from the portal) | Filled | Distinct | Used today |
|---|---|---|--:|--:|---|
| `pin` | text | Parcel Identification Number (PIN) | 100.0% | 929070 | E |
| `pin10` | text | Parcel Identification Number (10-digit) | 100.0% | 929066 | – |
| `year` | number | Tax year | 100.0% | 1 | – |
| `class` | text | Property class | 100.0% | 12 | R (search label) |
| `triad_name` | text | Triad name. Reassessment of property in Cook County is done within a triennial cycle, meaning it occurs every three years… | 100.0% | 3 | – |
| `triad_code` | text | Triad code. Reassessment of property in Cook County is done within a triennial cycle, meaning it occurs every three years… | 100.0% | 3 | – |
| `township_name` | text | Township name | 100.0% | 38 | R |
| `township_code` | text | Township code | 100.0% | 38 | B |
| `nbhd_code` | text | Assessor neighborhood code, first two digits are township, last three are neighborhood | 100.0% | 847 | E (appeal stats by neighborhood) |
| `tax_code` | text | Tax district code, as seen on individual property tax bills (Not currently up-to-date) | 100.0% | 1534 | B |
| `zip_code` | text | Property zip code | 99.7% | 182 | – |
| `lon` | number | Parcel centroid longitude | 99.9% | 928204 | B |
| `lat` | number | Parcel centroid latitude | 99.9% | 928228 | B |
| `x_3435` | number | Parcel centroid X coordinate (CRS 3435) | 99.9% | 928239 | – |
| `y_3435` | number | Parcel centroid Y coordinate (CRS 3435) | 99.9% | 928297 | – |
| `census_block_group_geoid` | text | Census block group GEOID | 99.9% | 3830 | – |
| `census_block_geoid` | text | Census block GEOID | 99.9% | 60003 | – |
| `census_congressional_district_geoid` | text | Census congressional district GEOID | 0.0% | 0 | – |
| `census_congressional_district_num` | text | Census congressional district number | 0.0% | 0 | – |
| `census_county_subdivision_geoid` | text | Census county subdivision GEOID | 99.9% | 31 | – |
| `census_place_geoid` | text | Census place GEOID | 97.5% | 129 | – |
| `census_puma_geoid` | text | Census PUMA GEOID | 0.0% | 0 | – |
| `census_school_district_elementary_geoid` | text | Census school district (elementary) GEOID | 63.5% | 116 | – |
| `census_school_district_secondary_geoid` | text | Census school district (secondary) GEOID | 63.5% | 28 | – |
| `census_school_district_unified_geoid` | text | Census school district (unified) GEOID | 36.4% | 6 | – |
| `census_state_representative_geoid` | text | Census state representative GEOID | 99.9% | 59 | – |
| `census_state_representative_num` | text | Census state representative number | 99.9% | 59 | – |
| `census_state_senate_geoid` | text | Census state senate GEOID | 99.9% | 32 | – |
| `census_state_senate_num` | text | Census state senate number | 99.9% | 32 | – |
| `census_tract_geoid` | text | Census tract GEOID | 99.9% | 1303 | – |
| `census_zcta_geoid` | text | Census ZCTA GEOID | 99.9% | 171 | – |
| `census_data_year` | number | Census data year | 99.9% | 1 | – |
| `census_acs5_congressional_district_geoid` | text | Census ACS5 congressional district GEOID | 99.9% | 11 | – |
| `census_acs5_congressional_district_num` | text | Census ACS5 congressional district number | 99.9% | 11 | – |
| `census_acs5_county_subdivision_geoid` | text | Census ACS5 county subdivision GEOID | 99.9% | 31 | – |
| `census_acs5_place_geoid` | text | Census ACS5 place GEOID | 97.5% | 129 | – |
| `census_acs5_puma_geoid` | text | Census ACS5 PUMA GEOID | 99.9% | 36 | – |
| `census_acs5_school_district_elementary_geoid` | text | Census ACS5 school district (elementary) GEOID | 63.5% | 116 | – |
| `census_acs5_school_district_secondary_geoid` | text | Census ACS5 school district (secondary) GEOID | 63.5% | 28 | – |
| `census_acs5_school_district_unified_geoid` | text | Census ACS5 school district (unified) GEOID | 36.4% | 6 | – |
| `census_acs5_state_representative_geoid` | text | Census ACS5 state representative GEOID | 99.9% | 59 | – |
| `census_acs5_state_representative_num` | text | Census ACS5 state representative number | 99.9% | 59 | – |
| `census_acs5_state_senate_geoid` | text | Census ACS5 state senate GEOID | 99.9% | 32 | – |
| `census_acs5_state_senate_num` | text | Census ACS5 state senate number | 99.9% | 32 | – |
| `census_acs5_tract_geoid` | text | Census ACS5 tract GEOID | 99.9% | 1303 | – |
| `census_acs5_data_year` | number | Census ACS5 data year | 99.9% | 1 | – |
| `cook_board_of_review_district_num` | text | Board of Review district number | 99.9% | 3 | – |
| `cook_board_of_review_district_data_year` | number | Board of Review district data year | 99.9% | 1 | – |
| `cook_commissioner_district_num` | text | Commissioner district number | 99.9% | 17 | – |
| `cook_commissioner_district_data_year` | number | Commissioner district data year | 99.9% | 1 | – |
| `cook_judicial_district_num` | text | Judicial district number | 99.9% | 15 | – |
| `cook_judicial_district_data_year` | number | Judicial district data year | 99.9% | 1 | – |
| `cook_municipality_num` | text | Municipality number per Cook County spatial records | 97.5% | 130 | – |
| `cook_municipality_name` | text | Municipality name per Cook County spatial records | 99.9% | 131 | – |
| `cook_municipality_data_year` | number | Municipality data year per Cook County spatial records | 99.9% | 1 | – |
| `ward_num` | text | Ward number | 33.7% | 50 | – |
| `ward_chicago_data_year` | number | Chicago ward data year | 32.5% | 1 | – |
| `ward_evanston_data_year` | number | Evanston ward data year | 1.2% | 1 | – |
| `chicago_community_area_num` | text | Chicago community area number | 32.5% | 76 | – |
| `chicago_community_area_name` | text | Chicago community area name | 32.5% | 76 | – |
| `chicago_community_area_data_year` | number | Chicago community area data year | 32.5% | 1 | – |
| `chicago_industrial_corridor_num` | text | Chicago industrial corridor number | 0.1% | 18 | – |
| `chicago_industrial_corridor_name` | text | Chicago industrial corridor name | 0.1% | 18 | – |
| `chicago_industrial_corridor_data_year` | number | Chicago industrial corridor data year | 0.1% | 1 | – |
| `chicago_police_district_num` | text | Chicago police district number | 33.2% | 23 | – |
| `chicago_police_district_data_year` | number | Chicago police district data year | 33.2% | 1 | – |
| `econ_coordinated_care_area_num` | text | Coordinated Care Area number | 66.2% | 139 | – |
| `econ_coordinated_care_area_data_year` | number | Coordinated Care Area data year | 66.2% | 1 | – |
| `econ_enterprise_zone_num` | text | Enterprise Zone number | 4.6% | 6 | – |
| `econ_enterprise_zone_data_year` | number | Enterprise Zone data year | 4.6% | 1 | – |
| `econ_industrial_growth_zone_num` | text | Industrial Growth Zone number | 0.5% | 6 | – |
| `econ_industrial_growth_zone_data_year` | number | Industrial Growth Zone data year | 0.5% | 1 | – |
| `econ_qualified_opportunity_zone_num` | text | Qualified Opportunity Zone number | 9.2% | 178 | – |
| `econ_qualified_opportunity_zone_data_year` | number | Qualified Opportunity Zone data year | 9.2% | 1 | – |
| `econ_central_business_district_num` | text | Chicago central business district number | 0.0% | 1 | – |
| `econ_central_business_district_data_year` | number | Chicago central business district data year | 0.0% | 1 | – |
| `env_flood_fema_sfha` | checkbox | FEMA Special Flood Hazard Area (SFHA) indicator | 99.9% | 2 | – |
| `env_flood_fema_data_year` | number | FEMA Special Flood Hazard Area (SFHA) data year | 99.9% | 1 | – |
| `env_flood_fs_factor` | number | First Street Flood Factor | 99.4% | 10 | – |
| `env_flood_fs_risk_direction` | number | First Street flood risk direction | 99.4% | 3 | – |
| `env_flood_fs_data_year` | number | First Street data year | 99.4% | 1 | – |
| `env_ohare_noise_contour_no_buffer_bool` | checkbox | O'Hare noise contour indicator (no buffer). Indicates whether or not a parcel's centroid is within O'Hare's 65 DNL noise contour | 99.9% | 2 | – |
| `env_ohare_noise_contour_half_mile_buffer_bool` | checkbox | O'Hare noise contour indicator (1/2 mile buffer) | 99.9% | 2 | – |
| `env_ohare_noise_contour_data_year` | number | O'Hare noise contour data year | 99.9% | 1 | – |
| `env_airport_noise_dnl` | number | Airport continuous noise surface estimated DNL | 99.6% | 239110 | – |
| `env_airport_noise_data_year` | text | Airport continuous noise surface estimated data year | 99.9% | 1 | – |
| `school_elementary_district_geoid` | text | School district (elementary) GEOID, derived from Cook County and City of Chicago shapefiles. Chicago Public Schools are associated with attendance areas… | 96.0% | 469 | – |
| `school_elementary_district_name` | text | School district (elementary) name, derived from Cook County and City of Chicago shapefiles… | 96.0% | 469 | – |
| `school_secondary_district_geoid` | text | School district (secondary) GEOID, derived from Cook County and City of Chicago shapefiles… | 96.0% | 73 | – |
| `school_secondary_district_name` | text | School district (secondary) name, derived from Cook County and City of Chicago shapefiles… | 96.0% | 73 | – |
| `school_unified_district_geoid` | text | School district (unified) GEOID… | 3.9% | 5 | – |
| `school_unified_district_name` | text | School district (unified) name… | 3.9% | 5 | – |
| `school_school_year` | text | School year | 99.9% | 1 | – |
| `school_data_year` | number | School data year | 99.9% | 1 | – |
| `tax_municipality_num` | text | Municipality number per Cook County tax records | 97.5% | 130 | – |
| `tax_municipality_name` | text | Municipality name per Cook County tax records | 97.5% | 130 | B |
| `tax_school_elementary_district_num` | text | School district (elementary) number, derived from tax district… | 63.5% | 116 | – |
| `tax_school_elementary_district_name` | text | School district (elementary) name, derived from tax district… | 63.5% | 116 | – |
| `tax_school_secondary_district_num` | text | School district (secondary) number, derived from tax district… | 63.5% | 28 | – |
| `tax_school_secondary_district_name` | text | School district (secondary) name, derived from tax district… | 63.5% | 28 | – |
| `tax_school_unified_district_num` | text | School district (unified) number, derived from tax district… | 36.4% | 6 | – |
| `tax_school_unified_district_name` | text | School district (unified) name, derived from tax district… | 36.4% | 6 | – |
| `tax_community_college_district_num` | text | Community college district number | 99.9% | 11 | – |
| `tax_community_college_district_name` | text | Community college district name | 99.9% | 11 | – |
| `tax_fire_protection_district_num` | text | Fire protection district number | 9.5% | 33 | – |
| `tax_fire_protection_district_name` | text | Fire protection district name | 9.5% | 33 | – |
| `tax_library_district_num` | text | Library district number | 39.1% | 70 | – |
| `tax_library_district_name` | text | Library district name | 39.1% | 70 | – |
| `tax_park_district_num` | text | Park district number | 85.4% | 93 | – |
| `tax_park_district_name` | text | Park district name | 85.4% | 93 | – |
| `tax_sanitation_district_num` | text | Sanitation district number | 4.2% | 16 | – |
| `tax_sanitation_district_name` | text | Sanitation district name | 4.2% | 16 | – |
| `tax_special_service_area_num` | text | Special Service Area number | 1.6% | 180 | – |
| `tax_special_service_area_name` | text | Special Service Area name | 1.6% | 180 | – |
| `tax_tif_district_num` | text | TIF district number | 0.0% | 0 | – |
| `tax_tif_district_name` | text | TIF district name | 0.0% | 0 | – |
| `tax_data_year` | number | Data year for municipality, school, community college, fire, library, park, sanitary, special service area, and tax increment financing tax districts | 99.9% | 1 | – |
| `access_cmap_walk_id` | text | CMAP walkability grid ID. From CMAP's ON TO 2050 spatial data files | 99.9% | 3051 | – |
| `access_cmap_walk_nta_score` | number | CMAP walkability score (no transit) | 99.9% | 310 | – |
| `access_cmap_walk_total_score` | number | CMAP walkability total score | 99.9% | 325 | – |
| `access_cmap_walk_data_year` | number | CMAP walkability data year | 99.9% | 1 | – |
| `misc_subdivision_id` | text | Subdivision ID | 97.8% | 36833 | – |
| `misc_subdivision_data_year` | number | Subdivision data year | 97.8% | 1 | – |
| `row_id` | text | Unique row key for API | 100.0% | 929070 | – |

### addresses: Assessor - Parcel Addresses (`3723-97qp`)

API rows: 50,672,323. Rows last updated 2026-11-15. Fill rates over 929,070 rows (2026). We load only the property address. The mailing and owner name and address columns are downloaded but never loaded or shown.

| Column | Type | Meaning (from the portal) | Filled | Distinct | Used today |
|---|---|---|--:|--:|---|
| `pin` | text | Parcel Identification Number (PIN) | 100.0% | 929070 | E |
| `pin10` | text | Parcel Identification Number (10-digit) | 100.0% | 929066 | – |
| `year` | number | Tax year | 100.0% | 1 | B (latest row kept) |
| `prop_address_full` | text | Property street address | 100.0% | 902639 | R (search, report) |
| `prop_address_city_name` | text | Property city | 100.0% | 172 | R |
| `prop_address_state` | text | Property state | 99.5% | 1 | – |
| `prop_address_zipcode_1` | text | Property zip code | 99.7% | 182 | B |
| `mail_address_name` | text | Taxpayer mailing name | 99.9% | 812880 | – |
| `mail_address_full` | text | Taxpayer mailing street address | 99.9% | 868219 | – |
| `mail_address_city_name` | text | Taxpayer mailing city | 99.9% | 2825 | – |
| `mail_address_state` | text | Taxpayer mailing state | 99.9% | 58 | – |
| `mail_address_zipcode_1` | text | Taxpayer mailing zip code | 99.6% | 3970 | – |
| `owner_address_name` | text | Owner mailing name | 100.0% | 802285 | – |
| `owner_address_full` | text | Owner mailing street address | 100.0% | 862982 | – |
| `owner_address_city_name` | text | Owner mailing city | 100.0% | 2832 | – |
| `owner_address_state` | text | Owner mailing state | 100.0% | 55 | – |
| `owner_address_zipcode_1` | text | Owner mailing zip code | 99.6% | 4056 | – |
| `row_id` | text | Unique row key for API | 100.0% | 929070 | – |

### sales: Assessor - Parcel Sales (`wvhk-k5uv`)

API rows: 2,691,587. Rows last updated 2026-11-15. Fill rates over 15,168 rows (2026 so far). The report lists the subject home's own recorded sales for reference only. Sales play no part in the comparison.

| Column | Type | Meaning (from the portal) | Filled | Distinct | Used today |
|---|---|---|--:|--:|---|
| `pin` | text | Parcel Identification Number (PIN) | 100.0% | 14873 | R |
| `year` | number | Year | 100.0% | 1 | – |
| `township_code` | text | Township code | 100.0% | 38 | – |
| `nbhd` | text | Assessor neighborhood code, first two digits are township, last three are neighborhood | 100.0% | 816 | B |
| `class` | text | Property class | 100.0% | 12 | B |
| `sale_date` | calendar_date | Sale date (recorded, not executed) | 100.0% | 251 | R (subject's sales) |
| `is_mydec_date` | checkbox | Indicates whether the sale date has been overwritten with a more precise value from IDOR (Illinois Department of Revenue)… | 100.0% | 1 | – |
| `sale_price` | number | Sale price | 100.0% | 2766 | R (subject's sales) |
| `doc_no` | text | Sale document number. Corresponds to Clerk's document number | 100.0% | 14849 | B |
| `deed_type` | text | Sale deed type | 100.0% | 5 | R |
| `mydec_deed_type` | text | Deed type from MyDec, more granular than CCAO deed type | 93.1% | 17 | – |
| `seller_name` | text | Sale seller name | 96.7% | 13346 | – |
| `is_multisale` | checkbox | Indicates whether a parcel was sold individually or as part of a larger group of PINs | 100.0% | 2 | R (filter) |
| `num_parcels_sale` | number | The number of parcels that were part of the sale | 100.0% | 7 | – |
| `buyer_name` | text | Sale buyer name | 98.4% | 14193 | – |
| `sale_type` | text | Sale type | 0.0% | 0 | – |
| `sale_filter_same_sale_within_365` | checkbox | Remove sale with the same value (for the same PIN) within 365 days | 100.0% | 2 | R (filter) |
| `sale_filter_less_than_10k` | checkbox | Indicator for whether sale is less than $10K | 100.0% | 2 | R (filter) |
| `sale_filter_deed_type` | checkbox | Indicator for quit claim, executor, beneficiary and missing deed types | 100.0% | 2 | R (filter) |
| `row_id` | text | Unique row key for API | 100.0% | 15168 | – |

### bor_appeals: Board of Review Appeal Decision History (`7pny-nedm`)

API rows: 6,933,428. Rows last updated 2026-06-09. Fill rates over 219,853 rows (tax year 2025). Names in this dataset (appellants, attorneys) are never loaded or shown. We use only whether an appeal was self-filed.

| Column | Type | Meaning (from the portal) | Filled | Distinct | Used today |
|---|---|---|--:|--:|---|
| `pin` | text | 14-digit Parcel Identification Number | 100.0% | 219853 | E |
| `township_code` | number | Township code | 100.0% | 38 | – |
| `tax_year` | number | Tax year | 100.0% | 1 | E (last 3 years) |
| `appealtrk` | number | First part of the Complaint number | 100.0% | 206350 | – |
| `appealseq` | number | Second part of the Complaint number | 100.0% | 265 | – |
| `taxcode` | number | tax code | 100.0% | 1190 | – |
| `vol` | number | volume number (legacy administrative designation to group properties geographically) | 100.0% | 600 | – |
| `class` | number | property class | 100.0% | 12 | E (excludes condos) |
| `appealtype` | text | Appeal type code | 100.0% | 1 | – |
| `appealtypedescription` | text | Appeal type description | 100.0% | 1 | B |
| `assessor_landvalue` | number | Assessor certified land value | 100.0% | 30245 | – |
| `assessor_improvementvalue` | number | Assessor certified improvement value | 100.0% | 79181 | – |
| `assessor_totalvalue` | number | Assessor certified total value | 100.0% | 51552 | E (average cut) |
| `bor_landvalue` | number | Board of Review certified land value | 100.0% | 30248 | – |
| `bor_improvementvalue` | number | Board of Review certified improvement value | 100.0% | 80494 | – |
| `bor_totalvalue` | number | Board of Review certified total value | 100.0% | 77594 | E (average cut) |
| `result` | text | Board of Review appeal result | 100.0% | 3 | E (reduction rate) |
| `changereason` | text | Reason for Change. This is only populated starting with 2015 | 100.0% | 6 | – |
| `changereasondescription` | text | Reason for Change. This is only populated starting with 2015 | 53.3% | 5 | B |
| `nochangereason` | text | Reason for No Change. This is only populated starting with 2015 | 100.0% | 9 | – |
| `nochangereasondescription` | text | Reason for No Change. This is only populated starting with 2015 | 45.2% | 7 | B |
| `majorclass` | text | Property major class description | 100.0% | 1 | E (residential only) |
| `appellant` | text | Appellant name | 100.0% | 192065 | – |
| `appellant_address` | text | Appellant street address | 100.0% | 191671 | – |
| `appellant_city` | text | Appellant city | 100.0% | 1104 | – |
| `appellant_state` | text | Appellant state | 100.0% | 54 | – |
| `appellant_zip` | number | Appellant zip code | 100.0% | 1061 | – |
| `attorneycode` | text | Attorney code; whether an attorney was used. Values are Individual/Pro Se, Exempt, or Attorney | 100.0% | 2 | E (self-filed vs attorney) |
| `attny` | number | Attorney ID. This can be used to join to the Attorney list dataset | 68.9% | 312 | – |
| `attorney_firstname` | text | Attorney first name | 65.3% | 234 | – |
| `attorney_lastname` | text | Attorney last name | 65.3% | 266 | – |
| `attorney_firmname` | text | Attorney firm name | 65.3% | 249 | – |
| `appealid` | text | Appeal ID. This matches the "Complaint" value on the Board of Review Decision Search | 100.0% | 219853 | – |
| `pin10` | text | Parcel Identification Number (10-digit) | 100.0% | 219853 | – |
| `long` | number | Parcel centroid longitude | 100.0% | 219744 | – |
| `lat` | number | Parcel centroid latitude | 100.0% | 219744 | – |
| `xcoord_crs_3435` | number | Parcel centroid X coordinate (CRS 3435) | 100.0% | 219547 | – |
| `ycoord_crs_3435` | number | Parcel centroid Y coordinate (CRS 3435) | 100.0% | 219646 | – |
| `pin_dash` | text | 14-digit Parcel Identification Number with dashes | 100.0% | 219853 | – |
| `centroid_geom` | point | Parcel Centroid geometry | 100.0% | 219744 | – |
| `rowid` | text | Row ID. This is a combination of tax year, PIN, AppealTrk, and AppealSeq | 100.0% | 219853 | – |

The change and no-change reasons (filled about half the time) could separate uniformity reductions from other reasons, which is one of the backtest's caveats.

### ccao_appeals: Assessor - Appeals (`y282-6ig3`)

API rows: 2,193,381. Rows last updated 2026-11-23. Fill rates over 145,765 rows (2026).

| Column | Type | Meaning (from the portal) | Filled | Distinct | Used today |
|---|---|---|--:|--:|---|
| `pin` | text | Parcel Identification Number (PIN) | 100.0% | 145645 | B |
| `year` | number | Tax year | 100.0% | 1 | B |
| `class` | text | Property class | 100.0% | 12 | B |
| `township_code` | text | Township code | 100.0% | 38 | – |
| `case_no` | text | Case number | 100.0% | 141592 | – |
| `hearing_type` | text | Appeal hearing type - Current year appeal or Current appeal & certificate of error | 100.0% | 2 | – |
| `subkey` | number | Unique number to allow multiple hearings per level | 100.0% | 1 | – |
| `appeal_type` | text | Appeal type | 100.0% | 4 | – |
| `status` | text | Appeal status - Open, Closed, Pending, or Closed Pending Certificate of Error. Only available for 2021 and later | 100.0% | 3 | B |
| `mailed_bldg` | number | Assessor mailed building value | 100.0% | 63939 | – |
| `mailed_land` | number | Assessor mailed land value | 100.0% | 25896 | – |
| `mailed_tot` | number | Assessor mailed total value | 100.0% | 36203 | B |
| `certified_bldg` | number | Assessor certified building value | 67.1% | 54118 | – |
| `certified_land` | number | Assessor certified land value | 67.1% | 22830 | – |
| `certified_tot` | number | Assessor certified total value | 67.1% | 35212 | B |
| `change` | text | Change: result of appeal | 77.0% | 2 | B |
| `reason_code1` | text | Reason code for change or no change | 77.0% | 51 | – |
| `reason_desc1` | text | Reason description for change or no change | 77.0% | 48 | B |
| `reason_code2` | text | Reason code for change or no change | 3.9% | 40 | – |
| `reason_desc2` | text | Reason description for change or no change | 3.9% | 38 | – |
| `reason_code3` | text | Reason code for change or no change | 0.1% | 15 | – |
| `reason_desc3` | text | Reason description for change or no change | 0.1% | 14 | – |
| `agent_code` | text | Agent code assigned by assessor to anonymize appeal for review. Only available for 2021 and later | 63.1% | 286 | – |
| `agent_name` | text | Agent name, typically taxpayer or attorney filing appeal. Only available for 2021 and later | 63.1% | 275 | B |
| `row_id` | text | Unique row key for API | 100.0% | 145765 | – |

This is the data for Assessor-stage appeals, which aren't covered yet.

### condo_chars: Assessor - Residential Condominium Unit Characteristics (`3r7i-mrz4`)

API rows: 11,436,289. Rows last updated 2026-11-15. Fill rates over 457,239 class-299 rows (2026). Condos aren't supported (see `docs/methodology.md`), and unit square footage is only 32% filled.

| Column | Type | Meaning (from the portal) | Filled | Distinct | Used today |
|---|---|---|--:|--:|---|
| `pin` | text | Parcel Identification Number (PIN). The last 4 digits identify the unit within the building | 100.0% | 457239 | B |
| `pin10` | text | Parcel Identification Number (10-digit). Identifies the condominium building | 100.0% | 18705 | B |
| `card` | number | Card number. Generally not applicable to condominiums | 100.0% | 4 | B |
| `year` | number | Tax year | 100.0% | 1 | B |
| `class` | text | Property class | 100.0% | 1 | – |
| `township_code` | text | Township code | 100.0% | 38 | – |
| `tieback_key_pin` | text | Tieback key PIN. Prorated properties (whose value is split across multiple PINs) have a "main" or key PIN | 100.0% | 54155 | – |
| `tieback_proration_rate` | number | Tieback proration rate. Prorated properties (whose value is split across multiple PINs) pay taxes on the proportion of value on their PIN… | 100.0% | 47009 | B |
| `card_proration_rate` | number | Card proration rate. Prorated parcels (whose value is split across multiple cards) pay taxes on the proportion of value on their card… | 100.0% | 47000 | – |
| `char_yrblt` | number | Year built | 100.0% | 161 | B |
| `char_building_sf` | number | Condominium building total square feet. Sourced from listings and other sources | 47.1% | 8201 | B |
| `char_unit_sf` | number | Condominium unit square feet. Sourced from listings and other sources | 32.4% | 2697 | B |
| `char_bedrooms` | number | Condominium unit bedrooms. Sourced from listings and other sources | 43.7% | 12 | B |
| `char_full_baths` | number | Full baths. Defined as having a bath or shower. If this value is missing, the default value is set to 1 | 23.2% | 8 | B |
| `char_half_baths` | number | Half baths. Defined as bathrooms without a shower or bathtub | 13.9% | 5 | – |
| `char_building_non_units` | number | Total condominium building non-livable parcel count. Includes parking, storage areas, and common areas | 100.0% | 241 | – |
| `char_building_pins` | number | Total condominium building parcel count. Includes both livable and non-livable parcels | 100.0% | 466 | – |
| `char_land_sf` | number | Land square feet. Same for all units in a building | 100.0% | 9713 | – |
| `cdu` | text | Condition, Desirability, and Utility code. Not well maintained | 19.4% | 8 | – |
| `pin_is_multiland` | checkbox | Multiland PIN indicator. Generally not applicable to condominiums | 100.0% | 2 | – |
| `pin_num_landlines` | number | Number of landlines on PIN. Generally not applicable to condominiums | 100.0% | 3 | – |
| `bldg_is_mixed_use` | checkbox | Condominium building contains non-residential units | 100.0% | 2 | – |
| `is_parking_space` | checkbox | Parking/garage space or storage unit indicator | 100.0% | 2 | B |
| `is_common_area` | checkbox | Building common area indicator | 100.0% | 2 | B |
| `row_id` | text | Unique row key for API | 100.0% | 457239 | – |

## Part 2: other datasets that could add per-parcel attributes

From the Socrata discovery API (`api.us.socrata.com/api/catalog/v1?domains=datacatalog.cookcountyil.gov`), which lists 523 datasets on the county portal. 89 of them have a `pin`, `pin10` or `pin14` column. Most of those are archived Assessor tables, single-year parcel shapefiles (2000–2010, 2019–2021), Recorder extracts from 2011–2015, and old Treasurer tax-sale lists. The current, maintained datasets we don't ingest yet:

| Dataset | ID | Rows | Updated (cadence) | Join key | Most useful columns |
|---|---|--:|---|---|---|
| Assessor - Parcel Proximity | `ydue-e5u3` | 36,675,279 | 2026-03-03 (data annual, published monthly) | `pin10` + `year` | `nearest_neighbor_{1,2,3}_pin10` / `_dist_ft`; `num_pin_in_half_mile`; distances to the nearest CTA/Metra stop, major/arterial/highway road (with daily traffic), railroad, park, water, Lake Michigan; `airport_dnl_total`; `nearest_new_construction_*`, `nearest_vacant_land_*` |
| Assessor - Permits | `6yjf-dfxs` | 759,224 | 2026-11-15 (bi-weekly) | `pin` | `date_issued`, `status`, `assessable`, `amount`, `work_description`, `improvement_code_1..4`. Useful to flag homes with recent assessable work, which may make a characteristic out of date. |
| Assessor - Property Tax-Exempt Parcels | `vgzx-68gb` | 465,858 | 2026-11-15 (bi-weekly) | `pin` + `year` | `class`, `lat`/`lon`. Lets us exclude exempt parcels from pools and counts. |
| Assessor - Parcel Universe (Historical) | `nj4t-kc8j` | 50,672,310 | 2026-11-15 (bi-weekly) | `pin` + `year` | The same columns as the current parcel universe (Part 1) for every year since 1999. Needed if comparisons should use each year's neighborhood and class rather than the current ones (one of the backtest caveats). |
| Assessor - Neighborhood Boundaries | `pcdw-pxtg` | 860 | 2022-04-29 (annual on paper; last 2022) | `town_nbhd` = our `nbhd` | `multipolygon`, `triad_code`. For maps and an "adjacent neighborhoods" rule. |
| Cook County Address Points | `78yw-iddh` | 1,376,187 | 2024-04-08 (as needed) | `pin` | Unit-level address points (`unit`, `floor`, `lat`/`long`). Could improve address search for units; not needed for comparisons. |

Datasets we already ingest have more columns available than we load. The biggest is the parcel universe: census geographies, school, park and library districts, ward, community area, FEMA flood zone, airport noise, walkability, subdivision. See its table in Part 1.

Not useful here: **Assessor - Commercial Valuation Data** (`csik-bsws`) is commercial only, and its columns are market-value estimates, which we don't use. The Recorder and Treasurer extracts stop in 2011–2016. The Clerk's tax-rate tables stop in 2010, so a dollar estimate of tax savings still needs another source (see `docs/methodology.md`).

## Part 3: proposal for a configurable comparison

Today's rules (see `docs/methodology.md`) are a fixed pipeline: a pool, hard filters that widen in four steps, a weighted distance, and the 20 nearest. A config can express the same thing as a list of criteria, so the default config reproduces today's results exactly. That can be tested: the backtest and `assessgap live-eval` must give identical output with the default config.

```yaml
# default.yaml: today's rules
pool:
  same: [nbhd, year]                  # Assessor neighborhood and tax year
  require: {num_cards: 1, bldg_sf: ">0", av: ">0"}
  classes: [202, 203, 204, 205, 206, 207, 208, 209, 210, 234, 278, 295]
keep: 20                              # nearest homes kept
min_comps: 8                          # widen until at least this many
criteria:                             # each: field, rule, tolerance per widening step, weight
  - {field: class,     match: exact,      widen: [exact, group, group, group], weight: 0.25}
  - {field: bldg_sf,   match: within_pct, steps: [25, 25, 35, 50],             weight: 4,   scale: subject}
  - {field: yrblt,     match: within,     steps: [15, 15, 25, 40],             weight: 1,   scale: 15}
  - {field: baths,     match: distance,                                        weight: 0.5}
  - {field: land_sf,   match: distance,   cap: 1,                              weight: 0.5, scale: subject}
  - {field: type_resd, match: same,                                            weight: 0.5}
  - {field: ext_wall,  match: same,                                            weight: 0.5}
  - {field: bsmt,      match: same,                                            weight: 0.5}
```

Rules, each implemented once and shared by every field:

- `exact` / `same`: a hard filter (`exact`) or a mismatch penalty of `weight` (`same`).
- `within` / `within_pct`: a hard filter at the current widening step's tolerance, plus `weight·|Δ|/scale` in the distance.
- `distance`: no filter, only the distance term (optionally capped).
- Later, location rules over Part 2 data: `within_ft` (from parcel centroids, or `nearest_neighbor_*` for next-door comparisons), `same_area` on a geography column (subdivision, census tract, school district).

What users could change: turn a criterion on or off, tighten or loosen its tolerance, and choose its weight from a few presets (ignore, low, normal, high) rather than raw numbers. Bounds keep results meaningful: the pool always stays within one Assessor neighborhood and tax year, because that is what the Board's own uniformity guidance compares ("same neighborhood and within the same classification code", see `docs/bor-filing-facts.md`). Building sq ft also stays required, because AV per sq ft depends on it. The report would print the config used, alongside today's "Compared on" line.

Fields reliable enough to offer, from Part 1:

| Offer | Fields | Why |
|---|---|---|
| Yes | `bldg_sf`, `yrblt`, `land_sf`, `beds`, `rooms`, `fbath`, `hbath`, `type_resd`, `ext_wall`, `bsmt`, `bsmt_fin`, `gar1_size`, `air`, `frpl`, `attic_type`, `attic_fnsh`, `porch`, `gar1_att`; `class` (exact or group) | ≥99% filled with real variety |
| Yes, location (needs Part 2 or more parcel-universe columns) | distance between centroids (`lat`/`lon`, 99.9%), `misc_subdivision_id` (97.8%), `census_tract_geoid` / `census_block_group_geoid` (99.9%), `school_elementary_district_name` (96%) | well filled; some pools get small, so widening must handle them |
| No | `cnst_qlty`, `repair_cnd`, `cdu`, `site`, `use`, `apts`, `ncu` | ≥99% one value; matching on them does nothing, or excludes the rare home it shouldn't |
| No | `renovation` | 1.2% filled |
| Later | permits (recent assessable work), exempt status | better as data checks or pool exclusions than as similarity criteria |

Caveats: characteristics are the Assessor's records, and some fields get defaults when missing. That is why the report's "worth double-checking" section exists, and a stricter config can't make wrong records right. Also, more criteria shrink the pool: in a 300-home random sample (`assessgap sample -n 300`, townships without posted dates), 97% of homes found 8 or more similar homes at the strictest step today, and every added hard filter lowers that.
