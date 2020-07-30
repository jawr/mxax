package site

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/site/funcs"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Site) renderTemplate(w http.ResponseWriter, t *template.Template, r *route, d interface{}) {
	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)

	b.Reset()

	if err := t.ExecuteTemplate(b, "base", d); err != nil {
		s.handleError(w, r, err)
		return
	}

	// write headers
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	b.WriteTo(w)
}

func (s *Site) loadTemplate(path string) (*template.Template, error) {

	tmpl := template.New(path).Funcs(funcs.Map())

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

func (s Site) templateResponse(path, method, routeName, templatePath string) (*route, error) {
	r := &route{
		path:    path,
		methods: []string{method},
	}

	// setup template
	tmpl, err := s.loadTemplate(templatePath)
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string
	}

	// actual handler
	r.h = func(_ pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: routeName,
		}

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}
