package site

import (
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
)

func (s *Site) getPostRegister() (*route, error) {
	r := &route{
		path:    "/register",
		methods: []string{"GET", "POST"},
	}

	type data struct {
		Form Form
	}

	tmpl, err := s.loadTemplate("templates/pages/register.html")
	if err != nil {
		return nil, err
	}

	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		d := data{
			Form: newForm(),
		}

		if req.Method == "POST" {
			if err := s.registerUser(req, &d.Form); err != nil {
				return err
			}

			if !d.Form.Error() {
				http.Redirect(w, req, "/thankyou", http.StatusFound)
				return nil
			}
		}

		return s.renderTemplate(w, tmpl, r, &d)
	}

	return r, nil
}

func (s *Site) registerUser(req *http.Request, form *Form) error {
	email := req.FormValue("email")
	password := req.FormValue("password")
	confirmPassword := req.FormValue("confirm-password")

	if !isEmailValid(email) {
		form.AddError("email", "Address is not valid")
		return nil
	}

	var count int
	err := s.db.QueryRow(
		req.Context(),
		"SELECT COUNT(*) FROM accounts WHERE email = $1",
		email,
	).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		form.AddError("email", "Address has already registered.")
		return nil
	}

	if password != confirmPassword {
		form.AddError("password", "")
		form.AddError("confirm-password", "Does not match")
		return nil
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		req.Context(),
		`
				INSERT INTO accounts (email,password)
					VALUES ($1,$2)
				`,
		email,
		hashed,
	)
	if err != nil {
		return err
	}

	// TODO
	// fire transactional email

	return nil
}

var emailRegex = regexp.MustCompile(
	"^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$",
)

func isEmailValid(e string) bool {
	if len(e) < 3 && len(e) > 254 {
		return false
	}
	if !emailRegex.MatchString(e) {
		return false
	}
	parts := strings.Split(e, "@")
	mx, err := net.LookupMX(parts[1])
	if err != nil || len(mx) == 0 {
		return false
	}
	return true
}
