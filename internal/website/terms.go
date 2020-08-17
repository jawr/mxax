package website

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getTerms() (*route, error) {
	r := &route{
		path:    "/terms",
		methods: []string{"GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/terms.html")
	if err != nil {
		return r, err
	}

	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		return s.renderTemplate(w, tmpl, r, struct{}{})
	}

	return r, nil
}
