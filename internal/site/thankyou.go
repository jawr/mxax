package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getThankyou() (*route, error) {
	r := &route{
		path:    "/thankyou/:for",
		methods: []string{"GET"},
	}

	tmpl, err := s.loadTemplate("templates/pages/thankyou.html")
	if err != nil {
		return nil, err
	}

	type data struct {
		Message string
	}

	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		var d data

		switch ps.ByName("for") {
		case "register":
			d.Message = "You should receive a verification email shortly."
		case "contact":
			d.Message = "We will get back to you soon!"
		default:
			d.Message = "For being you. <3."
		}

		return s.renderTemplate(w, tmpl, r, &d)
	}

	return r, nil
}
