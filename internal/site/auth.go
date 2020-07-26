package site

import (
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

type accountHandle func(accountID int, w http.ResponseWriter, r *http.Request, ps httprouter.Params) error

func (s *Site) auth(r *route) httprouter.Handle {
	// dirty cache of token to account id
	cache := make(map[string]int, 0)

	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {

		if os.Getenv("MXAX_DEV") == "1" {
			if err := r.h(1, w, req, ps); err != nil {
				s.handleError(w, r, err)
			}
			return
		}

		c, err := req.Cookie("mxax_session_token")
		if err != nil {
			if err == http.ErrNoCookie {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if accountID, ok := cache[c.Value]; ok {
			if err := r.h(accountID, w, req, ps); err != nil {
				s.handleError(w, r, err)
			}
			return
		}

		w.WriteHeader(http.StatusUnauthorized)
	}
}
