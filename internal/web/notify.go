package web

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/assessgap/assessgap/internal/web/views"
)

// notifyResult is the signup form's response: the bare result for htmx to
// swap in, or a whole page when the form was posted without JavaScript.
func (s *Server) notifyResult(r *http.Request, c templ.Component) templ.Component {
	if r.Header.Get("HX-Request") == "true" {
		return c
	}
	return views.NotifyPage(s.site, c)
}
