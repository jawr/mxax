package site

import (
	"html/template"
	"log"
	"net/http"

	"github.com/jackc/pgx"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type Site struct {
	db     *pgx.Conn
	router *httprouter.Router
}

// eventually if we want to do lots of testing we might want
// to swap out db for a bunch of interfaces for each route
func NewSite(db *pgx.Conn) (*Site, error) {
	s := &Site{
		db: db,
	}

	if err := s.setupRoutes(); err != nil {
		return nil, errors.WithMessage(err, "setupRoutes")
	}

	return s, nil
}

func (s *Site) Run(addr string) error {
	log.Printf("Listening on http://%s", addr)
	return http.ListenAndServe(addr, s.router)
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
