package cli

import (
	"context"
	"fmt"
	"runtime/debug"
)

// Set by release builds:
//
//	-ldflags "-X github.com/assessgap/assessgap/internal/cli.version=1.2.3 -X ...cli.commit=abc1234 -X ...cli.date=2026-11-25T00:00:00Z"
//
// Other builds fall back to what the Go toolchain records: the module version
// for "go install ...@v1.2.3", and the VCS revision and time for builds from a
// git checkout.
var version, commit, date string

func runVersion(_ context.Context, env *Env, args []string) error {
	fs := newFlags(env, "version", "")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, versionLine())
	return nil
}

func versionLine() string {
	v, c, d := version, commit, date
	if bi, ok := debug.ReadBuildInfo(); ok && commit == "" {
		if v == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v = bi.Main.Version
		}
		dirty := false
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				c = s.Value[:min(12, len(s.Value))]
			case "vcs.time":
				d = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if dirty && c != "" {
			c += "-dirty"
		}
	}
	if v == "" {
		v = "dev"
	}
	out := "assessgap " + v
	if c != "" {
		out += " (commit " + c
		if d != "" {
			out += ", built " + d
		}
		out += ")"
	}
	return out
}
