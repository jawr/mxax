package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getDashboard() (*route, error) {
	r := &route{
		path:   "/",
		method: "GET",
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/dashboard.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string
	}

	// actual handler
	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {

		d := data{
			Route: "dashboard",
		}

		s.renderTemplate(w, tmpl, r, d)
	}

	return r, nil
}
