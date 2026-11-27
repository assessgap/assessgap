package views

import (
	"context"
	"encoding/json"
	"strings"
)

// navLink is one of the header's site links.
type navLink struct{ Href, Label, Section string }

var navLinks = []navLink{
	{"/#check", "Check my home", "check"},
	{"/townships", "Townships", "townships"},
	{"/how-it-works", "How it works", "how"},
}

// navCurrent is the header link's aria-current on path: "page" on the page
// itself, "true" elsewhere in its section (a report, a township), else "".
func navCurrent(section, path string) string {
	switch section {
	case "check":
		if path == "/" {
			return "page"
		}
		if strings.HasPrefix(path, "/check/") || path == "/search" {
			return "true"
		}
	case "townships":
		if path == "/townships" {
			return "page"
		}
		if strings.HasPrefix(path, "/townships/") {
			return "true"
		}
	case "how":
		if path == "/how-it-works" {
			return "page"
		}
	}
	return ""
}

// Crumb is one breadcrumb. The last one is the current page.
type Crumb struct{ Name, Path string }

// breadcrumbJSON is schema.org BreadcrumbList JSON-LD with the same names
// and order as the visible breadcrumbs. json.Marshal escapes <, > and &, so
// the result is safe inside <script>.
func breadcrumbJSON(ctx context.Context, cs []Crumb) string {
	o := shareFrom(ctx).Origin
	type item struct {
		Type     string `json:"@type"`
		Position int    `json:"position"`
		Name     string `json:"name"`
		Item     string `json:"item"`
	}
	items := make([]item, len(cs))
	for i, c := range cs {
		items[i] = item{"ListItem", i + 1, c.Name, o + c.Path}
	}
	b, err := json.Marshal(struct {
		Context string `json:"@context"`
		Type    string `json:"@type"`
		Items   []item `json:"itemListElement"`
	}{"https://schema.org", "BreadcrumbList", items})
	if err != nil {
		return ""
	}
	return `<script type="application/ld+json">` + string(b) + `</script>`
}

// TownshipCrumbs are the breadcrumbs of a township's page.
func TownshipCrumbs(name, slug string) []Crumb {
	return []Crumb{{"Home", "/"}, {"Townships", "/townships"}, {name, "/townships/" + slug}}
}
