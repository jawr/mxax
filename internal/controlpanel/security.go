package controlpanel

import (
	"log"
	"net/http"

	"github.com/jackc/pgx/v4"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

func (s *Site) getPostSecurity() (*route, error) {
	r := &route{
		path:    "/security",
		methods: []string{"GET", "POST"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/controlpanel/security.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string

		Errors FormErrors

		Success bool
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "security",
			Errors: newFormErrors(),
		}

		if req.Method == "GET" {
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		password := req.FormValue("password")
		smtpPassword := req.FormValue("smtp-password")
		confirmSmtpPassword := req.FormValue("confirm-smtp-password")

		var hashedPassword []byte
		err := tx.QueryRow(
			req.Context(),
			`
			SELECT password 
			FROM accounts
			`,
		).Scan(&hashedPassword)
		if err != nil {
			log.Printf("ERR: %s", err)
			d.Errors.Add("password", "Wrong password")
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(password)); err != nil {
			d.Errors.Add("password", "Account Password is incorrect")
		}

		if len(smtpPassword) == 0 {
			d.Errors.Add("smtp-password", "Please enter a password")
		}

		if smtpPassword != confirmSmtpPassword {
			d.Errors.Add("confirm-smtp-password", "Password does not match")
		}

		if !d.Errors.Error() {

			smtpHashed, err := bcrypt.GenerateFromPassword([]byte(smtpPassword), bcrypt.DefaultCost)
			if err != nil {
				return errors.WithMessage(err, "bcrypt.GenerateFromPassword")
			}

			_, err = tx.Exec(
				req.Context(),
				`
			UPDATE accounts SET smtp_password = $1
			`,
				smtpHashed,
			)
			if err != nil {
				return errors.WithMessage(err, "Update")
			}

			d.Success = true
		}

		s.renderTemplate(w, tmpl, r, d)
		return nil
	}

	return r, nil
}
