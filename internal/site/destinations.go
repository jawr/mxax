package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getCreateDestination() (*route, error) {
	return s.templateResponse("/destinations/create", "GET", "destinations", "templates/pages/create_destination.html")
}

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

		s.renderTemplate(w, tmpl, r, d)
	}

	return r, nil
}
