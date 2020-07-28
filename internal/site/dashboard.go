package site

import (
	"net/http"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/julienschmidt/httprouter"
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

		Labels         []string
		InboundForward []int
		InboundBounce  []int
		InboundReject  []int
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "dashboard",
		}

		err := pgxscan.Select(
			req.Context(),
			s.db,
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
			return err
		}

		err = pgxscan.Select(
			req.Context(),
			s.db,
			&d.InboundForward,
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
        date_trunc('hour', m.time) AS hour,
        COUNT(m.*) AS cnt
    FROM metrics__inbound_forwards AS m
        JOIN domains AS d ON m.domain_id = d.id
    WHERE
        d.account_id = $1
        AND time > NOW() - INTERVAL '24 HOURS'
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
			accountID,
		)
		if err != nil {
			return err
		}

		err = pgxscan.Select(
			req.Context(),
			s.db,
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
        date_trunc('hour', m.time) AS hour,
        COUNT(m.*) AS cnt
    FROM metrics__inbound_bounces AS m
        JOIN domains AS d ON m.domain_id = d.id
    WHERE
        d.account_id = $1
        AND time > NOW() - INTERVAL '24 HOURS'
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
			accountID,
		)
		if err != nil {
			return err
		}

		err = pgxscan.Select(
			req.Context(),
			s.db,
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
        date_trunc('hour', m.time) AS hour,
        COUNT(m.*) AS cnt
    FROM metrics__inbound_rejects AS m
        JOIN domains AS d ON m.domain_id = d.id
    WHERE
        d.account_id = $1
        AND time > NOW() - INTERVAL '24 HOURS'
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
