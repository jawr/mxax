package controlpanel

import (
	"fmt"

	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type route struct {
	path    string
	methods []string
	h       accountHandle
}

func (r route) String() string { return fmt.Sprintf("%v %s", r.methods, r.path) }

type routeFn func() (*route, error)

func (s *Site) setupRoutes() error {
	s.router = httprouter.New()

	getPostLogin, err := s.getPostLogin()
	if err != nil {
		return errors.WithMessage(err, "getPostLogin")
	}

	s.router.GET("/login", getPostLogin)
	s.router.POST("/login", getPostLogin)

	// make these all accountID/auth handlers by default and apply the auth
	// middleware here
	routes := []routeFn{
		s.getDashboard,
		s.getDomain,
		s.getDeleteDomain,
		s.getDeleteDestination,
		s.getDeleteAlias,
		s.getLog,
		s.getLogDetail,
		s.getPostSecurity,
		s.getPostManageAlias,
		s.getDeleteAliasDestination,
		// logout
		s.getLogout,
	}

	for idx := range routes {
		r, err := routes[idx]()
		if err != nil {
			return errors.WithMessage(err, r.String())
		}

		for _, method := range r.methods {
			switch method {
			case "GET":
				s.router.GET(r.path, s.auth(r))
			case "POST":
				s.router.POST(r.path, s.auth(r))
			case "PUT":
				s.router.POST(r.path, s.auth(r))
			case "DELETE":
				s.router.POST(r.path, s.auth(r))
			}
		}
	}

	return nil
}
