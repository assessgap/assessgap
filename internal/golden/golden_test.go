package golden_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/assessgap/assessgap/internal/cli"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/golden"
	"github.com/assessgap/assessgap/internal/packet"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

const regenerate = "Regenerate deliberately: go test ./internal/golden ./internal/web -run Golden -update"

// TestGoldenCheck runs "assessgap check -json" on the fixture for every case
// and compares the report, field by field, with testdata/golden/check.
func TestGoldenCheck(t *testing.T) {
	db := fixture.Path(t)
	for _, c := range golden.Cases {
		t.Run(c.Name, func(t *testing.T) {
			var out, errb bytes.Buffer
			env := &cli.Env{Stdout: &out, Stderr: &errb, Getenv: func(string) string { return "" }}
			args := append([]string{"check", "-json", "-db", db}, c.Args...)
			code := cli.Run(context.Background(), env, args)

			got := []string{"exit = " + strconv.Itoa(code)}
			if s := strings.TrimSpace(errb.String()); s != "" {
				got = append(got, "stderr = "+strconv.Quote(s))
			}
			if out.Len() > 0 {
				fields, err := golden.Flatten(out.Bytes())
				if err != nil {
					t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
				}
				got = append(got, fields...)
			}
			header := "assessgap check -json " + strings.Join(c.Args, " ") + "\n" + regenerate
			golden.CheckFields(t, "check/"+c.Name+".txt", c.Name+" ("+c.Args[0]+")", header, got)
		})
	}
}

// TestGoldenNoOrphans fails when a golden file no longer has a case, so
// deleting a case can't silently drop its coverage.
func TestGoldenNoOrphans(t *testing.T) {
	want := map[string]bool{}
	for _, c := range golden.Cases {
		want["check/"+c.Name+".txt"] = true
	}
	for _, c := range golden.PacketPINs {
		want["pdf/"+c.Name+".txt"] = true
	}
	for _, dir := range []string{"check", "pdf"} {
		files, err := os.ReadDir(filepath.Join(golden.Dir, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if name := dir + "/" + f.Name(); !want[name] {
				t.Errorf("testdata/golden/%s has no case; delete it with git rm or restore its case", name)
			}
		}
	}
}

// generated is the preparation date printed in the packet. It also decides
// which filing-window wording the checklist uses, so it is fixed.
var generated = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

// TestGoldenPacket compares the evidence report's text with testdata/golden/pdf.
func TestGoldenPacket(t *testing.T) {
	e := comps.New(fixture.Open(t))
	for _, c := range golden.PacketPINs {
		t.Run(c.Name, func(t *testing.T) {
			r, err := e.Report(context.Background(), c.Args[0], 0)
			if err != nil {
				t.Fatal(err)
			}
			txt, err := packet.Text(r, generated)
			if err != nil {
				t.Fatal(err)
			}
			header := "Evidence report text for PIN " + c.Args[0] + ", prepared " + generated.Format("2006-01-02") + "\n" + regenerate
			golden.CheckLines(t, "pdf/"+c.Name+".txt", c.Name+" ("+c.Args[0]+")", header, strings.Split(txt, "\n"))
		})
	}
}
