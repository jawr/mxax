package site

import (
	"net/http"

	"github.com/jackc/pgx/v4"
	"github.com/julienschmidt/httprouter"
)

func (s *Site) verifyAction(fn accountHandle) accountHandle {
	type data struct {
		Route string
		Next  string
	}

	return func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		if _, ok := req.URL.Query()["verify"]; ok {
			return fn(tx, w, req, ps)
		}

		d := data{
			Route: "verify",
			Next:  req.URL.Path,
		}

		if err := s.verifyTemplate.ExecuteTemplate(w, "base", d); err != nil {
			return err
		}

		return nil

	}
}
