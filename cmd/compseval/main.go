// Command compseval has moved to "assessgap backtest" and "assessgap sample".
// This stub is left only until the directory is deleted.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, `compseval has moved: use "go run ./cmd/assessgap backtest" or "... sample"`)
	os.Exit(2)
}
