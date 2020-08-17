package website

import (
	"bytes"
	"crypto"
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/google/uuid"
	"github.com/jawr/mxax/internal/smtp"
	"github.com/jhillyerd/enmime"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"github.com/stripe/stripe-go/v71"
	"github.com/stripe/stripe-go/v71/customer"
	"golang.org/x/crypto/bcrypt"
)

func (s *Site) getPostRegister() (*route, error) {
	r := &route{
		path:    "/register/:accountType",
		methods: []string{"GET", "POST"},
	}

	type data struct {
		Form Form
	}

	tmpl, err := s.loadTemplate("templates/pages/register.html")
	if err != nil {
		return r, err
	}

	// email templates
	emailTmpl, err := template.ParseFiles("templates/emails/verify_email.html")
	if err != nil {
		return r, err
	}

	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		d := data{
			Form: newForm(),
		}

		accountType := ps.ByName("accountType")

		if req.Method == "POST" {
			code, err := s.registerUser(req, &d.Form, accountType)
			if err != nil {
				return err
			}

			if !d.Form.Error() {
				if err := s.queueVerifyEmail(req.FormValue("email"), code, emailTmpl); err != nil {
					return err
				}

				if accountType == "subscription" {
					http.Redirect(w, req, "/subscription/"+code, http.StatusFound)

				} else {
					http.Redirect(w, req, "/thankyou/register", http.StatusFound)
				}

				return nil
			}
		}

		return s.renderTemplate(w, tmpl, r, &d)
	}

	return r, nil
}

func (s *Site) queueVerifyEmail(address, code string, tmpl *template.Template) error {

	data := struct {
		Code    string
		Address string
	}{
		Code:    code,
		Address: address,
	}

	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)
	b.Reset()

	if err := tmpl.ExecuteTemplate(b, "verify_email", &data); err != nil {
		return err
	}

	message, err := enmime.Builder().
		From("Do Not Reply", "noreply@mx.ax").
		Subject("MX - Please verify your email address").
		HTML(b.Bytes()).
		To(address, address).
		Build()
	if err != nil {
		return err
	}

	b.Reset()

	if err := message.Encode(b); err != nil {
		return err
	}

	signed := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(signed)
	signed.Reset()

	opts := dkim.SignOptions{
		Domain:   "mx.ax",
		Selector: "mxax",
		Signer:   s.dkimKey,
		Hash:     crypto.SHA256,
	}

	if err := dkim.Sign(signed, b, &opts); err != nil {
		return err
	}

	id, err := uuid.Parse(code)
	if err != nil {
		return err
	}

	err = s.queueEmail(smtp.Email{
		ID:      id,
		From:    "noreply@mx.ax",
		To:      address,
		Message: signed.Bytes(),
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *Site) queueEmail(email smtp.Email) error {
	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)
	b.Reset()

	if err := json.NewEncoder(b).Encode(&email); err != nil {
		return errors.WithMessage(err, "Encode")
	}

	msg := amqp.Publishing{
		Timestamp:   time.Now(),
		ContentType: "application/json",
		Body:        b.Bytes(),
	}

	err := s.emailPublisher.Publish(
		"",
		"emails",
		false, // mandatory
		false, // immediate
		msg,
	)
	if err != nil {
		return errors.WithMessage(err, "Publish")
	}

	return nil
}

func (s *Site) registerUser(req *http.Request, form *Form, accountType string) (string, error) {
	terms := req.FormValue("terms")
	if terms != "on" {
		form.AddError("terms", "Must accept Terms of Use")
		return "", nil
	}

	email := req.FormValue("email")
	password := req.FormValue("password")
	confirmPassword := req.FormValue("confirm-password")

	if !isEmailValid(email) {
		form.AddError("email", "Address is not valid")
		return "", nil
	}

	var count int
	err := s.db.QueryRow(
		req.Context(),
		"SELECT COUNT(*) FROM accounts WHERE email = $1",
		email,
	).Scan(&count)
	if err != nil {
		return "", err
	}

	if count > 0 {
		form.AddError("email", "Address has already registered.")
		return "", nil
	}

	if password != confirmPassword {
		form.AddError("password", "")
		form.AddError("confirm-password", "Does not match")
		return "", nil
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	var code string
	err = s.db.QueryRow(
		req.Context(),
		`
				INSERT INTO accounts (email,password)
					VALUES ($1,$2)
					RETURNING verify_code
				`,
		email,
		hashed,
	).Scan(&code)
	if err != nil {
		return "", err
	}

	if accountType == "subscription" {
		params := &stripe.CustomerParams{
			Email: stripe.String(email),
		}

		c, err := customer.New(params)
		if err != nil {
			return "", err
		}

		_, err = s.db.Exec(
			req.Context(),
			`
		UPDATE accounts SET stripe_customer_id = $1 WHERE email = $2
		`,
			c.ID,
			email,
		)
		if err != nil {
			return "", err
		}
	}

	return code, nil
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
