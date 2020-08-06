package site

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
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
	verifyTmpl, err := s.loadTemplate("templates/pages/domain.html")
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
				rule := req.FormValue("rule")
				destinationID, err := strconv.Atoi(req.FormValue("destination"))
				if err != nil {
					return errors.WithMessage(err, "Atoi destinationID")
				}

				if len(rule) == 0 {
					d.AliasFormErrors.Add("rule", "Must enter a Rule")

				} else {

					var aliasID int
					err = tx.QueryRow(
						req.Context(),
						`
					INSERT INTO aliases (account_id, rule, domain_id)
					VALUES (current_setting('mxax.current_account_id')::INT, $1, $2)
					ON CONFLICT (rule, domain_id) DO UPDATE SET deleted_at = NULL
					RETURNING id
					`,
						rule,
						d.Domain.ID,
					).Scan(&aliasID)
					if err != nil {
						log.Printf("Inserting alias: %s", err)
						d.AliasFormErrors.Add("", "Unable to create Alias.")

					} else {
						_, err := tx.Exec(
							req.Context(),
							`
						INSERT INTO alias_destinations (account_id, alias_id, destination_id)
						VALUES (current_setting('mxax.current_account_id')::INT, $1, $2)
						ON CONFLICT (alias_id, destination_id) DO UPDATE SET deleted_at = NULL
						`,
							aliasID,
							destinationID,
						)
						if err != nil {
							log.Printf("Inserting alias_destination: %s", err)
							d.AliasFormErrors.Add("", "Unable to create Alias Destination.")

						}
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

		}

		s.renderTemplate(w, verifyTmpl, r, d)
		return nil
	}

	return r, nil
}

// check and see if the associated verify code exists
func (s *Site) postVerifyDomain() (*route, error) {
	r := &route{
		path:    "/domain/verify/:domain",
		methods: []string{"POST"},
	}

	// setup templates
	tmpl, err := s.loadTemplate("templates/pages/verify_domain.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route  string
		Errors FormErrors
		Domain account.Domain
	}

	// go net.LookupCNAME follows the Canonical chain
	dnsConfig, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return r, errors.WithMessage(err, "dns.ClientConfigFromFile")
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "domains",
			Errors: newFormErrors(),
		}

		err := account.GetDomain(
			req.Context(),
			tx,
			&d.Domain,
			ps.ByName("domain"),
		)
		if err != nil {
			return errors.WithMessage(err, "GetDomain")
		}

		if err := d.Domain.CheckVerifyCode(dnsConfig); err != nil {
			d.Errors.Add("", err.Error())
		}

		if d.Errors.Error() {
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		_, err = tx.Exec(
			req.Context(),
			"UPDATE domains SET verified_at = NOW() WHERE id = $1",
			d.Domain.ID,
		)
		if err != nil {
			return err
		}

		http.Redirect(w, req, "/domains", http.StatusFound)

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
	r.h = s.verifyAction(func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

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

		http.Redirect(w, req, "/domains", http.StatusFound)

		return nil
	})

	return r, nil
}
