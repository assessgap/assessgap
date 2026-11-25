// Command packet has moved to "assessgap report". This stub is left only
// until the directory is deleted.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, `packet has moved: use "go run ./cmd/assessgap report -o out.pdf <PIN>"`)
	os.Exit(2)
}
