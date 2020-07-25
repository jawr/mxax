package site

import (
	"html/template"
	"net/http"

	"github.com/jawr/mxax/internal/site/funcs"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

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
		path:   path,
		method: method,
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
	r.h = func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {

		d := data{
			Route: routeName,
		}

		if err := tmpl.ExecuteTemplate(w, "base", d); err != nil {
			s.handleError(w, r, err)
			return
		}
	}

	return r, nil
}
