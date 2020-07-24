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

		if err := tmpl.ExecuteTemplate(w, "base", d); err != nil {
			s.handleError(w, r, err)
			return
		}
	}

	return r, nil
}
