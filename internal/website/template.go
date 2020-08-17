package website

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/pkg/errors"
)

func (s *Site) renderTemplate(w http.ResponseWriter, t *template.Template, r *route, d interface{}) error {
	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)

	b.Reset()

	if err := t.ExecuteTemplate(b, "base", d); err != nil {
		return err
	}

	// write headers
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	b.WriteTo(w)

	return nil
}

func (s *Site) loadTemplate(path string) (*template.Template, error) {

	tmpl := template.New(path)

	var err error

	tmpl, err = tmpl.ParseFiles(path)
	if err != nil {
		return nil, errors.WithMessagef(err, "ParseFiles '%s'", path)
	}

	tmpl, err = tmpl.ParseGlob("templates/base/*.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseGlob base")
	}

	tmpl, err = tmpl.ParseGlob("templates/includes/*.html")
	if err != nil {
		return nil, errors.WithMessage(err, "ParseGlob includes")
	}

	return tmpl, nil
}
