package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getDestinations() (*route, error) {
	r := &route{
		path:   "/destinations",
		method: "GET",
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/destinations.html")
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
			Route: "destinations",
		}

		if err := tmpl.ExecuteTemplate(w, "base", d); err != nil {
			s.handleError(w, r, err)
			return
		}
	}

	return r, nil
}

func (s *Site) getCreateDestination() (*route, error) {
	r := &route{
		path:   "/destinations/create",
		method: "GET",
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/create_destination.html")
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
			Route: "destinations",
		}

		if err := tmpl.ExecuteTemplate(w, "base", d); err != nil {
			s.handleError(w, r, err)
			return
		}
	}

	return r, nil
}
