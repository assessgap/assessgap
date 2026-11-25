package views

import (
	"context"
	"encoding/json"
)

// Share is the link-preview (Open Graph, Twitter card) and canonical-URL
// information for one request. Package web puts it in the render context, so
// Layout's signature stays the same for every page.
type Share struct {
	Origin    string // "https://assessgap.com"
	URL       string // og:url, absolute
	Canonical string // <link rel="canonical">; "" for pages that aren't indexed
	Image     string // og:image, absolute
	// Private pages (reports) get a generic preview: previews are cached by
	// third parties, so a shared report link must never carry an address or PIN.
	Private bool
}

type shareKey struct{}

// WithShare returns ctx carrying sh for Layout.
func WithShare(ctx context.Context, sh Share) context.Context {
	return context.WithValue(ctx, shareKey{}, sh)
}

func shareFrom(ctx context.Context) Share {
	sh, _ := ctx.Value(shareKey{}).(Share)
	return sh
}

const (
	shareTitle = "AssessGap: free Cook County property tax appeal check"
	shareDesc  = "Free, open-source check that compares a Cook County home's assessed value with similar homes nearby, using public county data."
	shareAlt   = "AssessGap: Free, open-source Cook County property tax appeal check"
)

// previewText is the title and description a link preview shows.
func (sh Share) previewText(title, desc string) (string, string) {
	if sh.Private {
		return shareTitle, shareDesc
	}
	if desc == "" {
		desc = shareDesc
	}
	return title, desc
}

// breadcrumbJSON is schema.org BreadcrumbList JSON-LD for a township page.
// json.Marshal escapes <, > and &, so the result is safe inside <script>.
func breadcrumbJSON(ctx context.Context, name, slug string) string {
	o := shareFrom(ctx).Origin
	type item struct {
		Type     string `json:"@type"`
		Position int    `json:"position"`
		Name     string `json:"name"`
		Item     string `json:"item"`
	}
	b, err := json.Marshal(struct {
		Context string `json:"@context"`
		Type    string `json:"@type"`
		Items   []item `json:"itemListElement"`
	}{"https://schema.org", "BreadcrumbList", []item{
		{"ListItem", 1, "AssessGap", o + "/"},
		{"ListItem", 2, "Townships", o + "/townships"},
		{"ListItem", 3, name + " Township", o + "/townships/" + slug},
	}})
	if err != nil {
		return ""
	}
	return `<script type="application/ld+json">` + string(b) + `</script>`
}
