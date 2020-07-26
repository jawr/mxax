package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getDashboard() (*route, error) {
	r := &route{
		path:    "/",
		methods: []string{"GET"},
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
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "dashboard",
		}

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}
