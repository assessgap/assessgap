// Command prcheck fails when a pull request description leaves a required
// section of .github/pull_request_template.md empty or unchanged: "Numbers
// impact" and "Risk and rollback".
//
// It reads the description from $PR_BODY. Like tools/prloc, it uses only the
// standard library so the workflow can run the default branch's copy without
// checking out the pull request.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// required are the sections that must be filled in.
var required = []string{"Numbers impact", "Risk and rollback"}

var (
	comment = regexp.MustCompile(`(?s)<!--.*?-->`)
	heading = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
)

// placeholders are answers that say nothing.
var placeholders = map[string]bool{"": true, "-": true, "n/a": true, "na": true, "tbd": true, "todo": true, "...": true, "…": true}

// sections maps each "## " heading to its text, with HTML comments removed.
func sections(body string) map[string]string {
	body = comment.ReplaceAllString(strings.ReplaceAll(body, "\r\n", "\n"), "")
	out := map[string]string{}
	locs := heading.FindAllStringSubmatchIndex(body, -1)
	for i, l := range locs {
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		out[body[l[2]:l[3]]] = strings.TrimSpace(body[l[1]:end])
	}
	return out
}

// problems lists required sections that are missing, empty or placeholders.
func problems(body string) []string {
	got := sections(body)
	var out []string
	for _, name := range required {
		text, ok := got[name]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%q is missing; use .github/pull_request_template.md", name))
		case placeholders[strings.ToLower(strings.Trim(text, " *_.`"))]:
			out = append(out, fmt.Sprintf("%q is empty; fill it in (see the comment in the template)", name))
		}
	}
	return out
}

func main() {
	ps := problems(os.Getenv("PR_BODY"))
	for _, p := range ps {
		fmt.Printf("::error title=PR description::%s\n", p)
	}
	if len(ps) > 0 {
		os.Exit(1)
	}
}
