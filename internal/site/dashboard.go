package site

import (
	"net/http"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/logger"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Site) getDashboard() (*route, error) {
	r := &route{
		path:    "/",
		methods: []string{"GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/dashboard.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string

		Labels        []string
		InboundSend   []int
		InboundBounce []int
		InboundReject []int
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "dashboard",
		}

		err := pgxscan.Select(
			req.Context(),
			tx,
			&d.Labels,
			`
			SELECT to_char(date_trunc('hour', i), 'HH24:00 DD/MM')  FROM 
				generate_series(
					NOW() - INTERVAL '24 HOURS',
					NOW(),
					INTERVAL '1 HOUR'
			) AS t(i)
			`,
		)
		if err != nil {
			return errors.WithMessage(err, "Select Labels")
		}

		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.InboundSend,
			`
			WITH series AS (
				SELECT date_trunc(
					'hour',
					generate_series(
						NOW() - INTERVAL '24 HOURS',
						NOW(),
						INTERVAL '1 HOUR'
					)
				) AS hour
			), metrics AS (
				SELECT
					date_trunc('hour', l.time) AS hour,
					COUNT(l.*) AS cnt
				FROM logs AS l
					JOIN domains AS d ON l.domain_id = d.id
				WHERE
					time > NOW() - INTERVAL '24 HOURS'
					AND l.etype = $1
				GROUP BY 1
				ORDER BY 1
			)
			SELECT
				COALESCE(SUM(metrics.cnt), 0)
				
			FROM series
				LEFT JOIN metrics ON series.hour = metrics.hour

			GROUP BY series.hour
			ORDER BY series.hour
			`,
			logger.EntryTypeSend,
		)
		if err != nil {
			return errors.Wrap(err, "Select EntryTypeSend")
		}

		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.InboundBounce,
			`
			WITH series AS (
				SELECT date_trunc(
					'hour',
					generate_series(
						NOW() - INTERVAL '24 HOURS',
						NOW(),
						INTERVAL '1 HOUR'
					)
				) AS hour
			), metrics AS (
				SELECT
					date_trunc('hour', l.time) AS hour,
					COUNT(l.*) AS cnt
				FROM logs AS l
					JOIN domains AS d ON l.domain_id = d.id
				WHERE
					time > NOW() - INTERVAL '24 HOURS'
					AND l.etype = $1
				GROUP BY 1
				ORDER BY 1
			)
			SELECT
				COALESCE(SUM(metrics.cnt), 0)
				
			FROM series
				LEFT JOIN metrics ON series.hour = metrics.hour

			GROUP BY series.hour
			ORDER BY series.hour
			`,
			logger.EntryTypeBounce,
		)
		if err != nil {
			return errors.Wrap(err, "Select EntryTypeBounce")
		}

		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.InboundReject,
			`
			WITH series AS (
				SELECT date_trunc(
					'hour',
					generate_series(
						NOW() - INTERVAL '24 HOURS',
						NOW(),
						INTERVAL '1 HOUR'
					)
				) AS hour
			), metrics AS (
				SELECT
					date_trunc('hour', l.time) AS hour,
					COUNT(l.*) AS cnt
				FROM logs AS l
					JOIN domains AS d ON l.domain_id = d.id
				WHERE
					time > NOW() - INTERVAL '24 HOURS'
					AND l.etype = $1
				GROUP BY 1
				ORDER BY 1
			)
			SELECT
				COALESCE(SUM(metrics.cnt), 0)
				
			FROM series
				LEFT JOIN metrics ON series.hour = metrics.hour

			GROUP BY series.hour
			ORDER BY series.hour
			`,
			logger.EntryTypeReject,
		)
		if err != nil {
			return errors.Wrap(err, "Select EntryTypeReject")
		}

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}
