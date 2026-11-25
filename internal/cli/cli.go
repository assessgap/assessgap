// Package cli implements the assessgap command: one binary with subcommands
// for checking a home, writing the evidence report, the Board of Review
// calendar, building the county database, and running the website.
//
// Subcommands use the standard flag package. Unlike plain flag, flags may come
// after positional arguments ("assessgap report 13281000270000 -o x.pdf").
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// Env is what a command can touch: its output streams and environment.
type Env struct {
	Stdout, Stderr io.Writer
	Getenv         func(string) string
}

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, env *Env, args []string) error
}

var commands []command

func init() {
	commands = []command{
		{"check", "compare a home with similar homes (text, or -json)", runCheck},
		{"report", "write the evidence report PDF for a home", runReport},
		{"township", "Board of Review filing window for one township, or all", runTownship},
		{"db", "build or refresh the county database (sync, fetch, load, build, validate, finalize)", runDB},
		{"serve", "run the website", runServe},
		{"backtest", "compare the method with actual Board of Review outcomes (full database)", runBacktest},
		{"sample", "coverage and comparison mix on a random sample of homes (full database)", runSample},
	}
}

// exitError carries a specific exit status (for example 1 when an address is
// ambiguous) with a message that has already been printed, or none.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

// errUsage means the arguments were wrong; the command has printed usage.
var errUsage = &exitError{code: 2}

// Main runs the command line and returns the process exit status.
func Main(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, &Env{Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv}, args)
}

// Run dispatches args (without the program name) to a subcommand.
func Run(ctx context.Context, env *Env, args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help" {
		usage(env.Stderr)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	for _, c := range commands {
		if c.name != args[0] {
			continue
		}
		err := c.run(ctx, env, args[1:])
		var ee *exitError
		switch {
		case err == nil:
			return 0
		case errors.Is(err, flag.ErrHelp):
			return 0
		case errors.As(err, &ee):
			if ee.msg != "" {
				fmt.Fprintln(env.Stderr, "assessgap "+c.name+": "+ee.msg)
			}
			return ee.code
		default:
			fmt.Fprintln(env.Stderr, "assessgap "+c.name+": "+err.Error())
			return 1
		}
	}
	fmt.Fprintf(env.Stderr, "assessgap: unknown command %q\n\n", args[0])
	usage(env.Stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "assessgap compares Cook County homes' assessed values with similar homes, using public county data.")
	fmt.Fprintln(w, "\nUsage: assessgap <command> [flags] [args]\n\nCommands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-9s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w, "\nRun \"assessgap <command> -h\" for a command's flags.")
}

// newFlags returns a FlagSet whose usage line is "assessgap <name> <synopsis>".
func newFlags(env *Env, name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(env.Stderr, "usage: assessgap %s %s\n", name, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parseArgs parses flags that may appear before, between or after positional
// arguments, and returns the positional ones in order. "--" ends flag parsing.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, errUsage
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if used := len(args) - len(rest); used > 0 && args[used-1] == "--" {
			return append(pos, rest...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

// joinArgs turns positional args into one query ("5555 W Belmont Ave").
func joinArgs(pos []string) string { return strings.TrimSpace(strings.Join(pos, " ")) }
