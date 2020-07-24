package site

import (
	"fmt"

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
