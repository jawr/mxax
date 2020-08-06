package site

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
	"github.com/jawr/mxax/internal/logger"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Site) getDashboard() (*route, error) {
	r := &route{
		path:    "/",
		methods: []string{"GET", "POST"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/dashboard.html")
	if err != nil {
		return r, err
	}

	// custom defines
	type Domain struct {
		account.Domain
		Aliases  int
		CatchAll int
		Records  int
		Status   string
		Expiring bool
		Expired  bool
	}

	type Destination struct {
		account.Destination
		Aliases int
		HID     string
	}

	// definte template data
	type data struct {
		Route string

		// domain
		Domains          []Domain
		DomainFormErrors FormErrors

		// destination
		Destinations          []Destination
		DestinationFormErrors FormErrors

		// stats
		Labels        []string
		InboundSend   []int
		InboundBounce []int
		InboundReject []int
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:                 "dashboard",
			DomainFormErrors:      newFormErrors(),
			DestinationFormErrors: newFormErrors(),
		}

		// handle domain/destination creation
		if req.Method == "POST" {

			if name := req.FormValue("domain"); len(name) > 0 {

				// get expires at for domain
				// also acts as an additional layer of
				// validation, might be too noisey/error prone
				expiresAt, err := account.GetDomainExpirationDate(name)
				if err != nil {
					log.Printf("domain expires at %s", err)
					d.DomainFormErrors.Add("domain", err.Error())
				}

				// TODO
				// what other checks do we want to introduce here

				if !d.DomainFormErrors.Error() {

					err := account.CreateDomain(
						req.Context(),
						tx,
						name,
						expiresAt,
					)
					if err != nil {
						return errors.WithMessage(err, "CreateDomain")
					}

					// redirect success to domains page
					http.Redirect(w, req, "/domain/manage/"+name, http.StatusFound)

					return nil
				}
			}

			if address := req.FormValue("address"); len(address) > 0 {
				if strings.Count(address, "@") != 1 {
					d.DestinationFormErrors.Add("address", "Does not look like an email address")
				}

				// TODO
				// what other checks do we want to introduce here

				if !d.DestinationFormErrors.Error() {

					_, err := tx.Exec(
						req.Context(),
						`
				INSERT INTO destinations (account_id, address) 
				VALUES (current_setting('mxax.current_account_id')::INT, $1) 
					ON CONFLICT (account_id, address) DO UPDATE SET deleted_at = NULL
				`,
						strings.ToLower(address),
					)
					if err != nil {
						d.DestinationFormErrors.Add("address", "Address already exists")
					}
				}
			}
		}

		// handle domains
		if err := pgxscan.Select(
			req.Context(),
			tx,
			&d.Domains,
			`
			SELECT 
				d.*,
				COALESCE(COUNT(DISTINCT a.id) FILTER (
					WHERE a.deleted_at IS NULL
				)) as aliases,
				COALESCE(COUNT(DISTINCT r.id) FILTER (
					WHERE r.last_verified_at IS NOT NULL 
					AND r.deleted_at IS NULL
					OR r.last_verified_at > NOW() - INTERVAL '24 hours'
				)) as records,
				COALESCE(COUNT(DISTINCT a.id) FILTER (WHERE rule = '.*')) as catch_all
			FROM domains AS d 
				LEFT JOIN aliases AS a ON d.id = a.domain_id 
				LEFT JOIN records AS r ON d.id = r.domain_id
			WHERE 
				d.deleted_at IS NULL
			GROUP BY d.id
			ORDER BY d.name
			`,
		); err != nil {
			return err
		}

		// setup domain status
		for idx, dom := range d.Domains {
			if dom.VerifiedAt.Time.IsZero() {
				d.Domains[idx].Status = "unverified"
			} else if dom.Records != 5 {
				d.Domains[idx].Status = "incomplete"
			} else {
				d.Domains[idx].Status = "ready"
			}

			if time.Until(dom.ExpiresAt.Time) < 0 {
				d.Domains[idx].Expired = true
			} else if time.Until(dom.ExpiresAt.Time) < time.Hour*24*30 {
				d.Domains[idx].Expiring = true
			}
		}

		// handle destinations
		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.Destinations,
			`
			SELECT 
				d.*, 
				COALESCE(COUNT(ad.*) FILTER (
					WHERE ad.deleted_at IS NULL
				), 0) AS aliases
			FROM destinations AS d
				LEFT JOIN alias_destinations AS ad ON d.id = ad.destination_id
			WHERE d.deleted_at IS NULL
			GROUP BY d.id
			`,
		)
		if err != nil {
			return err
		}

		for idx := range d.Destinations {
			d.Destinations[idx].HID, err = s.idHasher.Encode([]int{
				d.Destinations[idx].ID,
			})
			if err != nil {
				return err
			}
		}

		// handle stats
		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.Labels,
			`
			SELECT date_trunc('hour', i)::text  FROM 
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
