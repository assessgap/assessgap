package socrata

import (
	"encoding/csv"
	"io"
)

func newCSV(r io.Reader) *csv.Reader {
	cr := csv.NewReader(r)
	cr.ReuseRecord = true
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	return cr
}

// NewCSVReader returns a lenient CSV reader suited to Socrata exports.
func NewCSVReader(r io.Reader) *csv.Reader { return newCSV(r) }
