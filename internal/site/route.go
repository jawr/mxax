package site

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type route struct {
	path    string
	methods []string
	h       errorHandler
}

func (r route) String() string { return fmt.Sprintf("%v %s", r.methods, r.path) }

type errorHandler func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error

func (s *Site) handle(r *route) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		if err := r.h(w, req, ps); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
}

type routeFn func() (*route, error)

func (s *Site) setupRoutes() error {
	s.router = httprouter.New()

	// make these all accountID/auth handlers by default and apply the auth
	// middleware here
	routes := []routeFn{
		s.getLander,
	}

	for idx := range routes {
		r, err := routes[idx]()
		if err != nil {
			return errors.WithMessage(err, r.String())
		}

		for _, method := range r.methods {
			switch method {
			case "GET":
				s.router.GET(r.path, s.handle(r))
			case "POST":
				s.router.POST(r.path, s.handle(r))
			case "PUT":
				s.router.POST(r.path, s.handle(r))
			case "DELETE":
				s.router.POST(r.path, s.handle(r))
			}
		}
	}

	return nil
}
