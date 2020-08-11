package site

import (
	"bytes"
	"crypto"
	"html/template"
	"net/http"

	"github.com/dpapathanasiou/go-recaptcha"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/google/uuid"
	"github.com/jawr/mxax/internal/smtp"
	"github.com/jhillyerd/enmime"
	"github.com/julienschmidt/httprouter"
)

func (s *Site) getPostContact() (*route, error) {
	r := &route{
		path:    "/contact",
		methods: []string{"GET", "POST"},
	}

	type data struct {
		Form         Form
		RecaptchaKey string
	}

	tmpl, err := s.loadTemplate("templates/pages/contact.html")
	if err != nil {
		return r, err
	}

	// email templates
	emailTmpl, err := template.ParseFiles("templates/emails/contact.html")
	if err != nil {
		return r, err
	}

	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {
		d := data{
			Form:         newForm(),
			RecaptchaKey: s.recaptchaPublicKey,
		}

		if req.Method == "POST" {
			if err := s.contact(req, &d.Form, emailTmpl); err != nil {
				return err
			}

			if !d.Form.Error() {
				http.Redirect(w, req, "/thankyou/contact", http.StatusFound)
				return nil
			}
		}

		return s.renderTemplate(w, tmpl, r, &d)
	}

	return r, nil
}

func readUserIP(r *http.Request) string {
	addr := r.Header.Get("X-Real-Ip")
	if addr == "" {
		addr = r.Header.Get("X-Forwarded-For")
	}
	if addr == "" {
		addr = r.RemoteAddr
	}
	return addr
}

func (s *Site) contact(req *http.Request, form *Form, tmpl *template.Template) error {
	from := req.FormValue("from")
	if !isEmailValid(from) {
		form.AddError("from", "invalid email address")
		return nil
	}

	message := req.FormValue("message")
	if len(message) < 10 {
		form.AddError("message", "message not long enough")
		return nil
	}

	recaptchaResult, err := recaptcha.Confirm(readUserIP(req), req.FormValue("g-recaptcha-response"))
	if err != nil {
		return err
	}

	if !recaptchaResult {
		form.AddError("recaptcha", "recaptcha failed")
		return nil
	}

	data := struct {
		From    string
		Message string
	}{
		From:    from,
		Message: message,
	}

	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)
	b.Reset()

	if err := tmpl.ExecuteTemplate(b, "contact", &data); err != nil {
		return err
	}

	email, err := enmime.Builder().
		From("Contact", "contact@mx.ax").
		Subject("MX - Contact Received").
		HTML(b.Bytes()).
		To("MX Contact", "contact@mx.ax").
		Build()
	if err != nil {
		return err
	}

	b.Reset()

	if err := email.Encode(b); err != nil {
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

	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}

	err = s.queueEmail(smtp.Email{
		ID:      id,
		From:    "contact@mx.ax",
		To:      "contact@mx.ax",
		Message: signed.Bytes(),
	})
	if err != nil {
		return err
	}

	return nil
}
