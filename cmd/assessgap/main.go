// Command assessgap compares Cook County homes' assessed values with similar
// homes, writes the free evidence report, shows the Board of Review calendar,
// builds the county database and runs the website. Run it with no arguments
// for the list of commands.
package main

import (
	"os"

	"github.com/assessgap/assessgap/internal/cli"
)

func main() { os.Exit(cli.Main(os.Args[1:])) }
