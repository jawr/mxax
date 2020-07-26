package site

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type Site struct {
	db         *pgx.Conn
	router     *httprouter.Router
	bufferPool sync.Pool

	errorTemplate *template.Template

	sessionStore *badger.DB
}

// eventually if we want to do lots of testing we might want
// to swap out db for a bunch of interfaces for each route
func NewSite(db *pgx.Conn) (*Site, error) {
	tmpl, err := template.ParseFiles("templates/errors/index.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseFiles errors/index.html")
	}

	tmpl, err = tmpl.ParseGlob("templates/base/*.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseGlob base")
	}

	// create session keystore
	sessionStore, err := badger.Open(badger.DefaultOptions("sessions.bdgr"))
	if err != nil {
		return nil, errors.WithMessage(err, "badger.Open")
	}

	s := &Site{
		db: db,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
		errorTemplate: tmpl,
		sessionStore:  sessionStore,
	}

	if err := s.setupRoutes(); err != nil {
		return nil, errors.WithMessage(err, "setupRoutes")
	}

	return s, nil
}

func (s *Site) Run(addr string) error {
	defer s.sessionStore.Close()
	return http.ListenAndServe(addr, s.router)
}

type Error struct {
	StatusCode int
	Message    string
	Route      string
}

func (s *Site) handleError(w http.ResponseWriter, r *route, err error) {
	id, uerr := uuid.NewRandom()
	if uerr != nil {
		log.Printf("%v %s ERROR: %s (%s)", r.methods, r.path, uerr, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("%v %s ERROR: %s (%s)", r.methods, r.path, err, id)

	d := &Error{
		StatusCode: http.StatusInternalServerError,
		Message:    fmt.Sprintf("Internal Server Error (%s)", id),
		Route:      "",
	}

	if err := s.errorTemplate.ExecuteTemplate(w, "base", d); err != nil {
		log.Printf("%v %s ERROR: %s (%s)", r.methods, r.path, err, id)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
