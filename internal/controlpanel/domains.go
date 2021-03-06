package controlpanel

import (
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
	"github.com/jawr/mxax/internal/logger"
	"github.com/julienschmidt/httprouter"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
)

// get specific information about a domain
// depending on the state will display
// different templates
func (s *Site) getDomain() (*route, error) {
	r := &route{
		path:    "/domain/manage/:domain",
		methods: []string{"GET", "POST"},
	}

	// setup templates
	tmpl, err := s.loadTemplate("templates/controlpanel/domain.html")
	if err != nil {
		return r, err
	}

	type Domain struct {
		account.Domain
		Records []account.Record
	}

	type Alias struct {
		account.Alias
		Destinations string
		HID          string
	}

	// definte template data
	type data struct {
		Route           string
		Domain          Domain
		IsComplete      bool
		Errors          FormErrors
		Aliases         []Alias
		AliasFormErrors FormErrors
		Destinations    []account.Destination

		// stream
		Entries []logger.Entry

		// stats
		Labels        []string
		InboundSend   []int
		InboundBounce []int
		InboundReject []int
	}

	// go net.LookupCNAME follows the Canonical chain
	dnsConfig, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return r, errors.WithMessage(err, "dns.ClientConfigFromFile")
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:           "domains",
			Errors:          newFormErrors(),
			AliasFormErrors: newFormErrors(),
		}

		err := account.GetDomain(
			req.Context(),
			tx,
			&d.Domain.Domain,
			ps.ByName("domain"),
		)
		if err != nil {
			return errors.WithMessage(err, "GetDomain")
		}

		if d.Domain.VerifiedAt.Time.IsZero() {
			if err := d.Domain.CheckVerifyCode(dnsConfig); err != nil {
				d.Errors.Add("verify", err.Error())
			} else {
				// success
				d.Domain.VerifiedAt.Time = time.Now()

				_, err = tx.Exec(
					req.Context(),
					"UPDATE domains SET verified_at = NOW() WHERE id = $1",
					d.Domain.ID,
				)
				if err != nil {
					return err
				}
			}

		} else {
			// get records and check them
			err = account.GetRecords(
				req.Context(),
				tx,
				&d.Domain.Records,
				d.Domain.ID,
			)
			if err != nil {
				return err
			}

			// check if domain status is complete
			d.IsComplete = len(d.Domain.Records) == 5
			if d.IsComplete {
				for _, rr := range d.Domain.Records {
					if rr.LastVerifiedAt.Time.IsZero() || time.Since(rr.LastVerifiedAt.Time) > time.Duration(24*time.Hour) {
						if err := rr.Check(d.Domain.Name, dnsConfig); err != nil {
							d.Errors.Add(rr.Value, err.Error())
							d.IsComplete = false
							continue
						}

						_, err = tx.Exec(
							req.Context(),
							"UPDATE records SET last_verified_at = NOW() WHERE id = $1",
							rr.ID,
						)
						if err != nil {
							return err
						}

						if !rr.IsComplete() {
							d.IsComplete = false
						}
					}
				}
			}
		}

		// if complete get the aliases
		if d.IsComplete {
			if req.Method == "POST" {

				allowed, err := s.aclAliasCreateCheck(req.Context(), tx)
				if err != nil {
					return err
				}

				if !allowed {
					d.AliasFormErrors.Add("rule", "current subscription doesn't allow any more aliases")
				}

				rule := req.FormValue("rule")
				destinationID, err := strconv.Atoi(req.FormValue("destination"))
				if err != nil {
					// hard fail as smells of malicious intent
					return errors.WithMessage(err, "Atoi destinationID")
				}

				if len(rule) == 0 {
					d.AliasFormErrors.Add("rule", "Must enter a Rule")
				}

				_, err = regexp.Compile(rule)
				if err != nil {
					d.AliasFormErrors.Add("rule", err.Error())
				}

				if !d.AliasFormErrors.Error() {

					err = account.CreateAlias(
						req.Context(),
						tx,
						rule,
						d.Domain.ID,
						destinationID,
					)
					if err != nil {
						log.Printf("Error creating alias: %s", err)
						d.Errors.Add(
							"",
							"Unable to attach destination to alias. Please contact support.",
						)
					}
				}
			}

			err := pgxscan.Select(
				req.Context(),
				tx,
				&d.Aliases,
				`
				SELECT
					a.*,
					COALESCE(STRING_AGG(d.address, ', ') FILTER (
						WHERE d.deleted_at IS NULL AND ad.deleted_at IS NULL
					), '') AS destinations
				FROM 
					aliases AS a
					JOIN domains AS dom ON a.domain_id = dom.id
					LEFT JOIN alias_destinations AS ad ON a.id = ad.alias_id
					LEFT JOIN destinations AS d ON ad.destination_id = d.id
				WHERE
					a.deleted_at IS NULL
					AND d.deleted_at IS NULL
					AND dom.id = $1
				GROUP BY a.id, dom.name
				ORDER BY dom.name, a.rule
			`,
				d.Domain.ID,
			)
			if err != nil {
				return err
			}

			for idx := range d.Aliases {
				d.Aliases[idx].HID, err = s.idHasher.Encode([]int{
					d.Aliases[idx].ID,
				})
				if err != nil {
					return err
				}
			}

			err = account.GetDestinations(
				req.Context(),
				tx,
				&d.Destinations,
			)
			if err != nil {
				return errors.WithMessage(err, "GetDestinations")
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
					AND domain_id = $1
                ORDER BY time DESC
			`,
				d.Domain.ID,
			)
			if err != nil {
				return err
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
					AND l.domain_id = $2
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
				d.Domain.ID,
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
					AND l.domain_id = $2
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
				d.Domain.ID,
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
					AND l.domain_id = $2
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
				d.Domain.ID,
			)
			if err != nil {
				return errors.Wrap(err, "Select EntryTypeReject")
			}

		}

		s.renderTemplate(w, tmpl, r, d)
		return nil
	}

	return r, nil
}

func (s *Site) getDeleteDomain() (*route, error) {
	r := &route{
		path:    "/domain/delete/:domain",
		methods: []string{"GET"},
	}

	// actual handler
	r.h = s.confirmAction(func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		var domain account.Domain

		// get domain
		err := account.GetDomain(
			req.Context(),
			tx,
			&domain,
			ps.ByName("domain"),
		)
		if err != nil {
			return errors.WithMessage(err, "GetDomain")
		}

		err = account.DeleteDomain(
			req.Context(),
			tx,
			domain.ID,
		)
		if err != nil {
			return errors.WithMessage(err, "DeleteDomain")
		}

		http.Redirect(w, req, "/", http.StatusFound)

		return nil
	})

	return r, nil
}
