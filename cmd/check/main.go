// Command check has moved to "assessgap check". This stub is left only until
// the directory is deleted.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, `check has moved: use "go run ./cmd/assessgap check -json <PIN>"`)
	os.Exit(2)
}
