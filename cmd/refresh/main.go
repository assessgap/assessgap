// Command refresh has moved to "assessgap db validate|finalize". This stub is
// left only until the directory is deleted.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, `refresh has moved: use "go run ./cmd/assessgap db validate" or "db finalize"`)
	os.Exit(2)
}
