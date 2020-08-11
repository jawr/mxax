package controlpanel

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
	"github.com/jawr/mxax/internal/transactional"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
	"github.com/speps/go-hashids"
)

type Site struct {
	db *pgx.Conn
	// full access
	adminDB *pgx.Conn

	router     *httprouter.Router
	bufferPool sync.Pool

	emailPublisher *transactional.Publisher

	errorTemplate   *template.Template
	confirmTemplate *template.Template

	sessionStore *badger.DB

	idHasher *hashids.HashID
}

// eventually if we want to do lots of testing we might want
// to swap out db for a bunch of interfaces for each route
func NewSite(db, adminDB *pgx.Conn, emailPublisher *transactional.Publisher) (*Site, error) {
	// errorTemplate
	errorTemplate, err := template.ParseFiles("templates/errors/index.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseFiles errors/index.html")
	}

	errorTemplate, err = errorTemplate.ParseGlob("templates/base/*.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseGlob base")
	}

	confirmTemplate, err := template.ParseFiles("templates/pages/confirm.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseFiles errors/index.html")
	}

	confirmTemplate, err = confirmTemplate.ParseGlob("templates/base/*.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseGlob base")
	}

	// create id hasher
	idhData := hashids.NewData()
	idhData.Salt = "6kbEkDwRLqbbm3n8"
	idhData.MinLength = 6

	idHasher, err := hashids.NewWithData(idhData)
	if err != nil {
		return nil, errors.WithMessage(err, "NewWithData")
	}

	// create session keystore
	sessionStore, err := badger.Open(badger.DefaultOptions("sessions.bdgr"))
	if err != nil {
		return nil, errors.WithMessage(err, "badger.Open")
	}

	s := &Site{
		db:             db,
		adminDB:        adminDB,
		emailPublisher: emailPublisher,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
		errorTemplate:   errorTemplate,
		confirmTemplate: confirmTemplate,
		sessionStore:    sessionStore,
		idHasher:        idHasher,
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

func (s *Site) handleErrorPlain(w http.ResponseWriter, r *route, err error) {
	id, uerr := uuid.NewRandom()
	if uerr != nil {
		log.Printf("%v %s ERROR: %s (%s)", r.methods, r.path, uerr, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("%v %s ERROR: %s (%s)", r.methods, r.path, err, id)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
