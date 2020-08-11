package controlpanel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
)

const sessionDuration = time.Second * 60000

type session struct {
	ExpiresAt time.Time
	AccountID int
}

type accountHandle func(tx pgx.Tx, w http.ResponseWriter, r *http.Request, ps httprouter.Params) error

func (s *Site) auth(r *route) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {

		next := req.URL.Path
		if next == "/" || next == "/login" || next == "/logout" {
			next = ""
		} else {
			next = "?next=" + next
		}

		c, err := req.Cookie("mxax_session_token")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, req, "/login"+next, http.StatusFound)
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
			tx, err := s.db.Begin(req.Context())
			if err != nil {
				s.handleError(w, r, err)
				return
			}
			defer tx.Rollback(req.Context())

			setCurrentAccountID := fmt.Sprintf("SET mxax.current_account_id TO %d", accountID)

			if _, err := tx.Exec(req.Context(), setCurrentAccountID); err != nil {
				s.handleError(w, r, err)
				return
			}

			if err := r.h(tx, w, req, ps); err != nil {
				s.handleError(w, r, err)
				return
			}

			if err := tx.Commit(req.Context()); err != nil {
				s.handleError(w, r, err)
				return
			}

			return
		}

		http.Redirect(w, req, "/login"+next, http.StatusFound)
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
			email := req.FormValue("email")
			password := req.FormValue("password")

			var accountID int
			var hash []byte
			err := s.adminDB.QueryRow(
				req.Context(),
				"SELECT id, password FROM accounts WHERE email = $1 AND verified_at IS NOT NULL",
				email,
			).Scan(&accountID, &hash)
			if err != nil {
				log.Printf("SELECT: %s", err)
				d.Errors.Add("", "Email not found or Password is incorrect")
				goto FAIL
			}

			if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil {
				log.Printf("Password mismatch: %s", err)
				d.Errors.Add("", "Email not found or Password is incorrect")
				goto FAIL
			}

			if err := s.setCookie(w, req, accountID); err != nil {
				s.handleErrorPlain(w, r, err)
				return
			}

			next := "/"
			allNext, ok := req.URL.Query()["next"]
			if ok && len(allNext) > 0 {
				next = allNext[0]
			}

			// update last login
			_, err = s.adminDB.Exec(
				req.Context(),
				"UPDATE accounts SET last_login_at = NOW() WHERE id = $1",
				accountID,
			)
			if err != nil {
				s.handleErrorPlain(w, r, err)
				return
			}

			http.Redirect(w, req, next, http.StatusFound)
			return
		}

	FAIL:

		if err := tmpl.ExecuteTemplate(w, "login", d); err != nil {
			s.handleErrorPlain(w, r, err)
		}
	}, nil
}

func (s *Site) getLogout() (*route, error) {
	r := &route{
		path:    "/logout",
		methods: []string{"GET"},
	}

	r.h = func(_ pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

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
			AccountID: accountID,
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
		Name:     "mxax_session_token",
		Value:    sessionToken.String(),
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		Expires:  expiresAt,
	})

	return nil
}
