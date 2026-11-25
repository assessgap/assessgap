// Command build has moved to "assessgap db build"; the SQL is now
// internal/ingest/build.sql. This stub is left only until the directory is
// deleted.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, `build has moved: use "go run ./cmd/assessgap db build -db PATH"`)
	os.Exit(2)
}
