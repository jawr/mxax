package site

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type route struct {
	path   string
	method string
	h      httprouter.Handle
}

func (r route) String() string { return fmt.Sprintf("%s %s", r.method, r.path) }

type routeFn func() (*route, error)

type Site struct {
	router *httprouter.Router
}

func NewSite() (*Site, error) {
	s := &Site{}

	if err := s.setupRoutes(); err != nil {
		return nil, errors.WithMessage(err, "setupRoutes")
	}

	return s, nil
}

func (s *Site) Run(addr string) error {
	log.Printf("Listening on http://%s", addr)
	return http.ListenAndServe(addr, s.router)
}

func (s *Site) setupRoutes() error {
	s.router = httprouter.New()

	routes := []routeFn{
		s.getDashboard,
		s.getDomains,
		s.getAddDomain,
		s.getDestinations,
		s.getCreateDestination,
		s.getAliases,
		s.getCreateAlias,
		s.getLog,
		s.getSecurity,
	}

	for idx := range routes {
		r, err := routes[idx]()
		if err != nil {
			return errors.WithMessage(err, r.String())
		}

		switch r.method {
		case "GET":
			s.router.GET(r.path, r.h)
		case "POST":
			s.router.POST(r.path, r.h)
		case "PUT":
			s.router.POST(r.path, r.h)
		case "DELETE":
			s.router.POST(r.path, r.h)
		}
	}

	return nil

}

func (s *Site) loadTemplate(path string) (*template.Template, error) {
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return nil, errors.WithMessagef(err, "ParseFiles '%s'", path)
	}

	tmpl, err = tmpl.ParseGlob("templates/base/*.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseGlob base")
	}

	return tmpl, nil
}

func (s *Site) handleError(w http.ResponseWriter, r *route, err error) {
	log.Printf("%s %s ERROR: %s", r.method, r.path, err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
