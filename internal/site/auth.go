package site

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
)

const sessionDuration = time.Second * 600

type session struct {
	ExpiresAt time.Time
	AccountID int
}

type accountHandle func(accountID int, w http.ResponseWriter, r *http.Request, ps httprouter.Params) error

func (s *Site) auth(r *route) httprouter.Handle {
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
				http.Redirect(w, req, "/login", http.StatusFound)
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var refresh bool
		var accountID int
		err = s.sessionStore.Update(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(c.Value))
			if err != nil {
				return err
			}

			if item.IsDeletedOrExpired() {
				return nil
			}

			return item.Value(func(val []byte) error {

				buf := s.bufferPool.Get().(*bytes.Buffer)
				defer s.bufferPool.Put(buf)
				buf.Reset()

				_, err := buf.Write(val)
				if err != nil {
					return err
				}

				var ses session
				if err := json.NewDecoder(buf).Decode(&ses); err != nil {
					return err
				}

				if time.Until(ses.ExpiresAt) < time.Duration(sessionDuration/4) {
					refresh = true
				}

				accountID = ses.AccountID
				return nil
			})
		})

		if refresh {
			if err := s.setCookie(w, req, accountID); err != nil {
				s.handleError(w, r, err)
				return
			}
		}

		if err == nil && accountID > 0 {
			if err := r.h(accountID, w, req, ps); err != nil {
				s.handleError(w, r, err)
			}
			return
		}

		http.Redirect(w, req, "/login", http.StatusFound)
	}
}

func (s *Site) getPostLogin() (httprouter.Handle, error) {
	r := &route{
		path:    "/login",
		methods: []string{"GET", "POST"},
	}

	type data struct {
		Route  string
		Errors FormErrors
	}

	tmpl, err := template.ParseFiles("templates/login.html")
	if err != nil {
		return nil, err
	}

	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		d := data{
			Route:  "login",
			Errors: newFormErrors(),
		}

		if req.Method == "POST" {
			username := req.FormValue("username")
			password := req.FormValue("password")

			if username == "trains@mx.ax" && password == "iliketrains" {
				if err := s.setCookie(w, req, 1); err != nil {
					s.handleError(w, r, err)
					return
				}

				http.Redirect(w, req, "/", http.StatusFound)
				return
			}

			d.Errors.Add("", "Username not found or Password is incorrect")
		}

		if err := tmpl.ExecuteTemplate(w, "login", d); err != nil {
			s.handleError(w, r, err)
		}
	}, nil
}

func (s *Site) getLogout() (*route, error) {
	r := &route{
		path:    "/logout",
		methods: []string{"GET"},
	}

	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		// delete from sessionStore

		http.SetCookie(w, &http.Cookie{
			Name:    "mxax_session_token",
			Value:   "",
			Path:    "/",
			Expires: time.Unix(0, 0),
		})
		http.Redirect(w, req, "/", http.StatusFound)

		return nil
	}

	return r, nil
}

func (s *Site) setCookie(w http.ResponseWriter, r *http.Request, accountID int) error {
	sessionToken, err := uuid.NewRandom()
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(sessionDuration)

	err = s.sessionStore.Update(func(txn *badger.Txn) error {
		ses := session{
			ExpiresAt: expiresAt,
			AccountID: 1,
		}

		buf := s.bufferPool.Get().(*bytes.Buffer)
		defer s.bufferPool.Put(buf)
		buf.Reset()

		if err := json.NewEncoder(buf).Encode(&ses); err != nil {
			return err
		}

		e := badger.NewEntry(
			[]byte(sessionToken.String()),
			buf.Bytes(),
		).WithTTL(sessionDuration)
		return txn.SetEntry(e)
	})
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:  "mxax_session_token",
		Value: sessionToken.String(),
		// Secure: true,
		Path:    "/",
		Expires: expiresAt,
	})

	return nil
}
