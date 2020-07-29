package site

import (
	"net/http"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/google/uuid"
	"github.com/jawr/mxax/internal/logger"
	"github.com/julienschmidt/httprouter"
)

func (s *Site) getLog() (*route, error) {
	r := &route{
		path:    "/log",
		methods: []string{"GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/log.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string

		Entries []logger.Entry
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "log",
		}

		// get forward entries
		err := pgxscan.Select(
			req.Context(),
			s.db,
			&d.Entries,
			`
                SELECT                                                                 
					l.*
                FROM logs AS l
                    JOIN domains AS d ON l.domain_id = d.id
                WHERE
                    d.account_id = $1
                    AND l.time > NOW() - INTERVAL '48 HOURS'
                ORDER BY l.time DESC
			`,
			accountID,
		)
		if err != nil {
			return err
		}

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}

func (s *Site) getLogDetail() (*route, error) {
	r := &route{
		path:    "/log/detail/:id/:ltime",
		methods: []string{"GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/log_detail.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string
		Entry logger.Entry
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "log",
		}

		lid, err := uuid.Parse(ps.ByName("id"))
		if err != nil {
			return err
		}

		ltime, err := d.Entry.DecodeTime(ps.ByName("ltime"))
		if err != nil {
			return err
		}

		// get forward entries
		err = pgxscan.Get(
			req.Context(),
			s.db,
			&d.Entry,
			`
                SELECT                                                                 
					l.*
                FROM logs AS l
                    JOIN domains AS d ON l.domain_id = d.id
                WHERE
                    d.account_id = $1
                    AND l.time = $2
					AND l.id = $3
			`,
			accountID,
			ltime,
			lid,
		)
		if err != nil {
			return err
		}

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}
