package site

import (
	"bytes"
	"net/http"
	"sync"

	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type Site struct {
	router     *httprouter.Router
	bufferPool sync.Pool
}

func NewSite() (*Site, error) {
	s := &Site{
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
