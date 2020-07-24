package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getDomains() (*route, error) {
	r := &route{
		path:   "/domains",
		method: "GET",
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/domains.html")
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
			Route: "domains",
		}

		if err := tmpl.ExecuteTemplate(w, "base", d); err != nil {
			s.handleError(w, r, err)
			return
		}
	}

	return r, nil
}

func (s *Site) getAddDomain() (*route, error) {
	r := &route{
		path:   "/domains/add",
		method: "GET",
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/add_domain.html")
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
			Route: "domains",
		}

		if err := tmpl.ExecuteTemplate(w, "base", d); err != nil {
			s.handleError(w, r, err)
			return
		}
	}

	return r, nil
}
