package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getSecurity() (*route, error) {
	r := &route{
		path:    "/security",
		methods: []string{"GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/security.html")
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
			Route: "security",
		}

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}
