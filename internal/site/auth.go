package site

import (
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

type accountHandle func(accountID int, w http.ResponseWriter, r *http.Request, ps httprouter.Params)

func (s *Site) auth(fn accountHandle) httprouter.Handle {
	// dirty cache of token to account id
	cache := make(map[string]int, 0)

	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {

		if os.Getenv("MXAX_DEV") == "1" {
			fn(1, w, r, ps)
			return
		}

		c, err := r.Cookie("mxax_session_token")
		if err != nil {
			if err == http.ErrNoCookie {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if accountID, ok := cache[c.Value]; ok {
			fn(accountID, w, r, ps)
			return
		}

		w.WriteHeader(http.StatusUnauthorized)
	}
}
