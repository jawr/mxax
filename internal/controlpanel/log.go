package controlpanel

import (
	"net/http"
	"strconv"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
	"github.com/jawr/mxax/internal/logger"
	"github.com/julienschmidt/httprouter"
)

func (s *Site) getLog() (*route, error) {
	r := &route{
		path:    "/log",
		methods: []string{"GET", "POST"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/controlpanel/log.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route     string
		LogLevel  account.LogLevel
		LogLevels []account.LogLevel
		Entries   []logger.Entry
		Form      FormErrors
	}

	logLevels := []account.LogLevel{
		account.LogLevelAll,
		account.LogLevelBounceAndReject,
		account.LogLevelBounce,
		account.LogLevelReject,
		account.LogLevelNone,
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:     "log",
			LogLevels: logLevels,
			Form:      newFormErrors(),
		}

		if req.Method == "POST" {

			n, err := strconv.Atoi(req.FormValue("log-level"))
			if err != nil {
				return err
			}

			logLevel := account.LogLevel(n)
			_, err = tx.Exec(
				req.Context(),
				`UPDATE accounts SET log_level = $1`,
				logLevel,
			)
			if err != nil {
				return err
			}

			d.Form.Clear()
		}

		err := tx.QueryRow(
			req.Context(),
			`
			SELECT log_level FROM accounts
			`,
		).Scan(&d.LogLevel)
		if err != nil {
			return err
		}

		// get forward entries
		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.Entries,
			`
                SELECT                                                                 
					*
                FROM logs
                WHERE
                    time > NOW() - INTERVAL '48 HOURS'
                ORDER BY time DESC
			`,
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
	tmpl, err := s.loadTemplate("templates/controlpanel/log_detail.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string
		Entry logger.Entry
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

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
			tx,
			&d.Entry,
			`
                SELECT                                                                 
					*
                FROM logs
                WHERE
                    time = $1
					AND id = $2
			`,
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
