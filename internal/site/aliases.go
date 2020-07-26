package site

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jawr/mxax/internal/account"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Site) getAliases() (*route, error) {
	r := &route{
		path:    "/aliases",
		methods: []string{"GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/aliases.html")
	if err != nil {
		return r, err
	}

	type Alias struct {
		account.Alias
		Domain       string
		Destinations string
	}

	// definte template data
	type data struct {
		Route string

		Aliases []Alias
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "aliases",
		}

		err := pgxscan.Select(
			req.Context(),
			s.db,
			&d.Aliases,
			`
				SELECT
					a.*,
					dom.name AS domain,
					COALESCE(STRING_AGG(d.address, ', '), '') AS destinations
				FROM 
					aliases AS a
					JOIN domains AS dom ON a.domain_id = dom.id
					LEFT JOIN alias_destinations AS ad ON a.id = ad.alias_id
					LEFT JOIN destinations AS d ON ad.destination_id = d.id
				WHERE
					dom.account_id = $1
				GROUP BY a.id, dom.name
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

func (s *Site) getPostCreateAlias() (*route, error) {
	r := &route{
		path:    "/aliases/create",
		methods: []string{"GET", "POST"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/create_alias.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string

		Domains      []account.Domain
		Destinations []account.Destination

		Errors FormErrors
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "aliases",
			Errors: newFormErrors(),
		}

		// get domains and destinations
		err := pgxscan.Select(
			req.Context(),
			s.db,
			&d.Domains,
			`SELECT * FROM domains WHERE account_id = $1 AND verified_at IS NOT NULL`,
			accountID,
		)
		if err != nil {
			return errors.WithMessage(err, "Select domains")
		}

		err = pgxscan.Select(
			req.Context(),
			s.db,
			&d.Destinations,
			`SELECT * FROM destinations WHERE account_id = $1`,
			accountID,
		)
		if err != nil {
			return errors.WithMessage(err, "Select destinations")
		}

		if req.Method == "GET" {
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		domainID, err := strconv.Atoi(req.FormValue("domain"))
		if err != nil {
			return errors.WithMessage(err, "Atoi domain")
		}

		destinationID, err := strconv.Atoi(req.FormValue("destination"))
		if err != nil {
			return errors.WithMessage(err, "Atoi destination")
		}

		// validate regexp
		rule := req.FormValue("rule")
		if len(rule) == 0 {
			d.Errors.Add("rule", "Please add a rule.")
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		_, err = regexp.Compile(rule)
		if err != nil {
			d.Errors.Add("rule", err.Error())
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		// used for validation

		var count int
		// validate that domain belongs to this account
		err = s.db.QueryRow(
			req.Context(),
			`SELECT COUNT(*) FROM domains WHERE id = $1 AND account_id = $2`,
			domainID,
			accountID,
		).Scan(&count)
		if err != nil {
			return errors.WithMessage(err, "Validate domain")
		}

		if count != 1 {
			d.Errors.Add("domain", "Invalid domain")
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		// validate that destination belongs to this account
		err = s.db.QueryRow(
			req.Context(),
			`SELECT COUNT(*) FROM destinations WHERE id = $1 AND account_id = $2`,
			destinationID,
			accountID,
		).Scan(&count)
		if err != nil {
			return errors.WithMessage(err, "Validate domain")
		}

		if count != 1 {
			d.Errors.Add("domain", "Invalid domain")
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		// insert alias
		var aliasID int
		err = s.db.QueryRow(
			req.Context(),
			`
			WITH e AS (
				INSERT INTO aliases (domain_id, rule, catch_all) 
				VALUES ($1, $2, $3) 
				ON CONFLICT (domain_id, rule) DO NOTHING RETURNING id
			)
			SELECT * FROM e UNION SELECT id FROM aliases WHERE domain_id = $1 AND rule = $2
			`,
			domainID,
			rule,
			false,
		).Scan(&aliasID)
		if err != nil {
			log.Printf("Error inserting alias (%d,%s,%t): %s", domainID, rule, false, err)
			d.Errors.Add(
				"",
				fmt.Sprintf(
					"Unable to create alais. Please contact support. (%s)",
					time.Now(),
				),
			)
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		_, err = s.db.Exec(
			req.Context(),
			"INSERT INTO alias_destinations (alias_id, destination_id) VALUES ($1, $2)",
			aliasID,
			destinationID,
		)
		if err != nil {
			log.Printf("Error inserting alias_destination (%d,%d): %s", aliasID, destinationID, err)
			d.Errors.Add(
				"",
				fmt.Sprintf(
					"Unable to attach destination to alias. Please contact support. (%s)",
					time.Now(),
				),
			)
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		http.Redirect(w, req, "/aliases", http.StatusFound)

		return nil
	}

	return r, nil
}
