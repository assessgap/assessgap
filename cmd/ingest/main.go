// Command ingest has moved to "assessgap db sync|fetch|load". This stub is
// left only until the directory is deleted.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, `ingest has moved: use "go run ./cmd/assessgap db sync" (or db fetch / db load)`)
	os.Exit(2)
}
