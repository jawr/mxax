package site

import (
	"bytes"
	"net/http"
	"sync"

	"github.com/jackc/pgx/v4"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type Site struct {
	db         *pgx.Conn
	router     *httprouter.Router
	bufferPool sync.Pool
}

func NewSite(db *pgx.Conn) (*Site, error) {
	s := &Site{
		db: db,
		bufferPool: sync.Pool{
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
