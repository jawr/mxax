package site

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

	getPostRegister, err := s.getPostRegister()
	if err != nil {
		return errors.WithMessage(err, "getPostRegister")
	}

	s.router.GET("/register", getPostRegister)
	s.router.POST("/register", getPostRegister)

	getThankyou, err := s.getThankyou()
	if err != nil {
		return errors.WithMessage(err, "getThankyou")
	}

	s.router.GET("/thankyou", getThankyou)

	// make these all accountID/auth handlers by default and apply the auth
	// middleware here
	routes := []routeFn{
		s.getDashboard,
		s.getDomain,
		s.getDeleteDomain,
		s.getDeleteDestination,
		s.getDeleteAlias,
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
