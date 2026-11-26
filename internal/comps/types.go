package comps

// Disclaimer goes on every report. We compare assessed values only.
const Disclaimer = "This compares Cook County assessed values (AV), and the building's AV per square foot, " +
	"among similar homes, using public Assessor data. It is not an appraisal and does not " +
	"estimate what any property would sell for."

// LandNote explains, in every report surface, why only the building is compared.
const LandNote = "We compare the building part of your assessment, per square foot, because the Assessor values land separately at a neighborhood rate."

// Verdicts.
const (
	Strong   = "strong"
	Possible = "possible"
	Weak     = "weak"
)

// Home is one residential parcel with its assessed value for a tax year.
//
// AV is the total; BldgAV and LandAV are the Assessor's split of it into the
// building (improvement) and the land. BldgAVPerSF, building AV ÷ building
// square feet, is the rate homes are compared on.
type Home struct {
	PIN         string  `json:"pin"`
	Address     string  `json:"address,omitempty"`
	Class       string  `json:"class"`
	AV          float64 `json:"av"`
	BldgAV      float64 `json:"bldg_av"`
	LandAV      float64 `json:"land_av"`
	BldgAVPerSF float64 `json:"bldg_av_per_sf"`
	BldgSF      float64 `json:"bldg_sf"`
	LandSF      float64 `json:"land_sf"`
	YearBuilt   int     `json:"year_built"`
	Rooms       int     `json:"rooms"`
	Beds        int     `json:"beds"`
	FullBaths   int     `json:"full_baths"`
	HalfBaths   int     `json:"half_baths"`
	TypeResd    string  `json:"type_resd"`
	ExtWall     string  `json:"ext_wall"`
	Basement    string  `json:"basement"`
	BsmtFin     string  `json:"basement_finish"`
	Air         string  `json:"central_air"`
	Fireplaces  int     `json:"fireplaces"` // -1 = not on record
	Garage      string  `json:"garage"`
	Lat         float64 `json:"lat,omitempty"`
	Lon         float64 `json:"lon,omitempty"`
	NumCards    int     `json:"-"`
	HasSplit    bool    `json:"-"` // building and land AV on record, building > 0
}

// LandAVPerSF is land AV ÷ land square feet, or 0 when either is missing.
func (h Home) LandAVPerSF() float64 {
	if h.LandSF <= 0 || !h.HasSplit {
		return 0
	}
	return h.LandAV / h.LandSF
}

// Baths counts a half bath as 0.5.
func (h Home) Baths() float64 { return float64(h.FullBaths) + 0.5*float64(h.HalfBaths) }

// Comp is a candidate comparable with its similarity distance from the
// subject (lower = more similar) and how far away it is.
type Comp struct {
	Home
	Distance float64 `json:"distance"`
	// Miles is the straight-line distance between parcel centers; -1 when
	// either home has no location on record.
	Miles float64 `json:"miles"`
	// Differences lists the material differences from the subject, as facts
	// ("0.3 mi away · built 12 years later"). Set on evidence comps only.
	Differences string `json:"differences,omitempty"`
}

// Search records the criteria that produced the comp set.
type Search struct {
	Level       int     `json:"level"` // 0 = strictest; higher = widened
	SameClass   bool    `json:"same_class"`
	SFTolerance float64 `json:"sf_tolerance"`  // ±fraction of subject building sqft
	AgeTol      int     `json:"age_tolerance"` // ±years built; 0 = not applied
	Description string  `json:"description"`
	Candidates  int     `json:"candidates"` // homes passing the filters, before top-20 cut
}

// Stats summarizes the subject against its comps. Rates are building AV per
// building square foot.
type Stats struct {
	NumComps             int     `json:"num_comps"`
	SubjectBldgAVPerSF   float64 `json:"subject_bldg_av_per_sf"`
	MedianBldgAVPerSF    float64 `json:"median_comp_bldg_av_per_sf"`
	PctAboveMedian       float64 `json:"pct_above_median"`   // e.g. 0.12 = 12% above
	SubjectPercentile    float64 `json:"subject_percentile"` // 0–100 among comps
	CompsLower           int     `json:"comps_lower"`        // comps with a building assessed lower per sq ft (strictly)
	UniformityAV         float64 `json:"uniformity_av"`      // median comp rate × subject sf + subject land AV
	PotentialAVReduction float64 `json:"potential_av_reduction"`
	// Land, checked separately: the subject's land AV per land sq ft and the
	// comps' median. 0 when the land sq ft or land AV isn't on record.
	SubjectLandAVPerSF float64 `json:"subject_land_av_per_sf"`
	MedianLandAVPerSF  float64 `json:"median_comp_land_av_per_sf"`
}

// NbhdStat is the neighborhood's recent BOR appeal record for one filer type.
type NbhdStat struct {
	ProSe     bool    `json:"pro_se"`
	Appeals   int     `json:"appeals"`
	Decreases int     `json:"decreases"`
	WinRate   float64 `json:"win_rate"`
	AvgCutPct float64 `json:"avg_cut_pct"` // among decreases
	FirstYear int     `json:"first_year"`
	LastYear  int     `json:"last_year"`
}

// Sale is a recorded arm's-length sale of the subject (informational only).
type Sale struct {
	Date     string  `json:"date"`
	Price    float64 `json:"price"`
	DeedType string  `json:"deed_type,omitempty"`
}

// Flag is a characteristic that looks unusual and that the owner should check.
type Flag struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Report is the full comparison for one PIN and tax year.
type Report struct {
	PIN         string `json:"pin"`
	Year        int    `json:"year"`
	AVStage     string `json:"av_stage,omitempty"` // "certified" or "mailed"
	Address     string `json:"address,omitempty"`
	City        string `json:"city,omitempty"`
	Township    string `json:"township,omitempty"`
	Nbhd        string `json:"nbhd,omitempty"`
	Class       string `json:"class,omitempty"`
	Unsupported string `json:"unsupported,omitempty"`

	Subject     *Home      `json:"subject,omitempty"`
	Search      *Search    `json:"search,omitempty"`
	Stats       *Stats     `json:"stats,omitempty"`
	Verdict     string     `json:"verdict,omitempty"`
	Reasons     []string   `json:"reasons,omitempty"`
	Evidence    []Comp     `json:"evidence_comps,omitempty"`
	Comps       []Comp     `json:"comps,omitempty"`
	Flags       []Flag     `json:"please_verify,omitempty"`
	NbhdAppeals []NbhdStat `json:"neighborhood_appeals,omitempty"`
	Sales       []Sale     `json:"recent_sales,omitempty"`

	Disclaimer string `json:"disclaimer"`
}
