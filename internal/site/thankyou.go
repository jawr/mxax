package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getThankyou() (*route, error) {
	r := &route{
		path:    "/thankyou",
		methods: []string{"GET"},
	}

	tmpl, err := s.loadTemplate("templates/pages/thankyou.html")
	if err != nil {
		return nil, err
	}

	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		return s.renderTemplate(w, tmpl, r, struct{}{})
	}

	return r, nil
}
