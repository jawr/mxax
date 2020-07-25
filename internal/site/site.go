package site

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type Site struct {
	db                 *pgx.Conn
	router             *httprouter.Router
	templateBufferPool sync.Pool
}

// eventually if we want to do lots of testing we might want
// to swap out db for a bunch of interfaces for each route
func NewSite(db *pgx.Conn) (*Site, error) {
	s := &Site{
		db: db,
		templateBufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}

	if err := s.setupRoutes(); err != nil {
		return nil, errors.WithMessage(err, "setupRoutes")
	}

	return s, nil
}

func (s *Site) Run(addr string) error {
	return http.ListenAndServe(addr, s.router)
}

func (s *Site) handleError(w http.ResponseWriter, r *route, err error) {
	id, uerr := uuid.NewRandom()
	if uerr != nil {
		log.Printf("%s %s ERROR: %s (%s)", r.method, r.path, uerr, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("%s %s ERROR: %s (%s)", r.method, r.path, err, id)
	http.Error(
		w,
		fmt.Sprintf("Internal Server Error (%s)", id),
		http.StatusInternalServerError,
	)
}
