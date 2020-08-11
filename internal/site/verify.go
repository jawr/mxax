package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) getVerify() (*route, error) {
	r := &route{
		path:    "/verify/:code",
		methods: []string{"GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/success.html")
	if err != nil {
		return r, err
	}

	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		_, err := s.db.Exec(
			req.Context(),
			`
			UPDATE accounts
			SET verified_at = NOW()
			WHERE verified_at IS NULL
				AND verify_code = $1
			`,
			ps.ByName("code"),
		)
		if err != nil {
			return err
		}
		return s.renderTemplate(w, tmpl, r, struct{}{})
	}

	return r, nil
}
