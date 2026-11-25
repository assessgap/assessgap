// Command server has moved to "assessgap serve". This stub is left only
// until the directory is deleted.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, `server has moved: use "go run ./cmd/assessgap serve"`)
	os.Exit(2)
}
