// Command prloc counts a pull request's added and deleted lines for test code
// versus production code, and writes the table into the PR description
// between marker comments, replacing the previous table.
//
// Generated files (templ output, app.css, vendored JS, goldens, the fixture,
// go.sum) get their own row and count toward neither side.
//
// It uses only the standard library and imports nothing from this module, so
// the workflow can fetch this one file from the default branch and run it
// without checking out the pull request's code.
//
//	prloc -pr 12                                  # update PR 12 (needs GITHUB_TOKEN, GITHUB_REPOSITORY)
//	git diff --numstat main...HEAD | prloc -numstat   # print the table for a local branch
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	startMarker = "<!-- prloc:start -->"
	endMarker   = "<!-- prloc:end -->"
)

// Kinds of file, in table order.
const (
	kindTest       = "Test"
	kindProduction = "Production"
	kindGenerated  = "Generated (excluded)"
	kindOther      = "Docs and config (excluded)"
)

var kinds = []string{kindTest, kindProduction, kindGenerated, kindOther}

// File is one changed file.
type File struct {
	Name      string `json:"filename"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// classify says which row a path counts toward.
func classify(path string) string {
	base := path[strings.LastIndex(path, "/")+1:]
	switch {
	case strings.HasSuffix(path, "_templ.go"),
		path == "internal/web/static/app.css",
		strings.HasSuffix(path, ".min.js"),
		strings.HasPrefix(path, "testdata/golden/"),
		strings.HasPrefix(path, "testdata/fixture/"),
		path == "go.sum":
		return kindGenerated
	case strings.HasSuffix(path, "_test.go"),
		strings.HasPrefix(path, "internal/fixture/"), // test-only packages
		strings.HasPrefix(path, "internal/golden/"),
		strings.HasPrefix(path, "testdata/"):
		return kindTest
	case strings.HasSuffix(path, ".md"), strings.HasPrefix(path, ".github/"),
		base == "go.mod", base == "LICENSE", base == ".gitignore", base == ".golangci.yml":
		return kindOther
	}
	return kindProduction
}

type row struct{ files, added, deleted int }

// table renders the markdown table, with marker comments around it.
func table(files []File) string {
	rows := map[string]*row{}
	for _, k := range kinds {
		rows[k] = &row{}
	}
	for _, f := range files {
		r := rows[classify(f.Name)]
		r.files++
		r.added += f.Additions
		r.deleted += f.Deletions
	}
	var b strings.Builder
	b.WriteString(startMarker + "\n")
	b.WriteString("### Lines changed: test vs production\n\n")
	b.WriteString("| | Files | Added | Deleted | Net |\n|---|--:|--:|--:|--:|\n")
	for _, k := range kinds {
		r := rows[k]
		fmt.Fprintf(&b, "| %s | %d | +%d | −%d | %s |\n", k, r.files, r.added, r.deleted, signed(r.added-r.deleted))
	}
	t, p := rows[kindTest], rows[kindProduction]
	if p.added > 0 {
		fmt.Fprintf(&b, "\nTest lines added per production line added: %.2f.\n", float64(t.added)/float64(p.added))
	}
	b.WriteString("\n_Updated automatically on every push. Generated: `*_templ.go`, `app.css`, vendored `*.min.js`, `testdata/golden/`, `testdata/fixture/`, `go.sum`._\n")
	b.WriteString(endMarker)
	return b.String()
}

func signed(n int) string {
	if n < 0 {
		return "−" + strconv.Itoa(-n)
	}
	return "+" + strconv.Itoa(n)
}

// splice replaces the marked section of body with section, or appends it.
func splice(body, section string) string {
	i := strings.Index(body, startMarker)
	j := strings.Index(body, endMarker)
	if i >= 0 && j > i {
		return body[:i] + section + body[j+len(endMarker):]
	}
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return section + "\n"
	}
	return body + "\n\n" + section + "\n"
}

// parseNumstat reads "git diff --numstat" output. Binary files ("-") count as 0.
func parseNumstat(r io.Reader) ([]File, error) {
	var out []File
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.SplitN(sc.Text(), "\t", 3)
		if len(fields) != 3 {
			continue
		}
		add, _ := strconv.Atoi(fields[0])
		del, _ := strconv.Atoi(fields[1])
		name := fields[2]
		if strings.Contains(name, " => ") { // renames: count the new path
			name = renamed(name)
		}
		out = append(out, File{Name: name, Additions: add, Deletions: del})
	}
	return out, sc.Err()
}

// renamed turns "a/{b => c}/d" or "b => c" into the new path.
func renamed(s string) string {
	if i, j := strings.Index(s, "{"), strings.Index(s, "}"); i >= 0 && j > i {
		inner := s[i+1 : j]
		_, to, _ := strings.Cut(inner, " => ")
		return strings.ReplaceAll(s[:i]+to+s[j+1:], "//", "/")
	}
	_, to, _ := strings.Cut(s, " => ")
	return to
}

type github struct {
	repo, token string
	client      *http.Client
	api         string
}

func (g *github) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, g.api+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return &apiError{resp.StatusCode, string(msg)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("GitHub API: HTTP %d: %s", e.status, e.msg) }

func (g *github) files(pr int) ([]File, error) {
	var all []File
	for page := 1; ; page++ {
		var batch []File
		if err := g.do("GET", fmt.Sprintf("/repos/%s/pulls/%d/files?per_page=100&page=%d", g.repo, pr, page), nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			return all, nil
		}
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("prloc", flag.ContinueOnError)
	pr := fs.Int("pr", 0, "pull request number to update")
	numstat := fs.Bool("numstat", false, "read `git diff --numstat` from stdin and print the table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *numstat {
		files, err := parseNumstat(stdin)
		if err != nil {
			return err
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
		_, err = fmt.Fprintln(stdout, table(files))
		return err
	}
	if *pr == 0 {
		return errors.New("give -pr N or -numstat")
	}
	g := &github{repo: getenv("GITHUB_REPOSITORY"), token: getenv("GITHUB_TOKEN"),
		client: &http.Client{Timeout: 30 * time.Second}, api: "https://api.github.com"}
	if g.repo == "" || g.token == "" {
		return errors.New("GITHUB_REPOSITORY and GITHUB_TOKEN must be set")
	}
	files, err := g.files(*pr)
	if err != nil {
		return err
	}
	section := table(files)
	var cur struct {
		Body string `json:"body"`
	}
	if err := g.do("GET", fmt.Sprintf("/repos/%s/pulls/%d", g.repo, *pr), nil, &cur); err != nil {
		return err
	}
	next := splice(cur.Body, section)
	if next == cur.Body {
		return nil
	}
	err = g.do("PATCH", fmt.Sprintf("/repos/%s/pulls/%d", g.repo, *pr), map[string]string{"body": next}, nil)
	var ae *apiError
	if errors.As(err, &ae) && ae.status == http.StatusForbidden {
		// Pull requests from forks get a read-only token: show the table in the job summary instead.
		if path := getenv("GITHUB_STEP_SUMMARY"); path != "" {
			return os.WriteFile(path, []byte(section+"\n"), 0o644)
		}
	}
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "prloc:", err)
		os.Exit(1)
	}
}
