package site

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (s *Site) verifyAction(fn accountHandle) accountHandle {
	type data struct {
		Route string
		Next  string
	}

	return func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		if _, ok := req.URL.Query()["verify"]; ok {
			return fn(accountID, w, req, ps)
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
