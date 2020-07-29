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
		HID          string
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
					AND a.deleted_at IS NULL
				GROUP BY a.id, dom.name
				ORDER BY dom.name, a.rule
			`,
			accountID,
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
			`SELECT * FROM domains WHERE account_id = $1 AND verified_at IS NOT NULL AND deleted_at IS NULL ORDER BY name`,
			accountID,
		)
		if err != nil {
			return errors.WithMessage(err, "Select domains")
		}

		err = pgxscan.Select(
			req.Context(),
			s.db,
			&d.Destinations,
			`SELECT * FROM destinations WHERE account_id = $1 AND deleted_at IS NULL ORDER BY address`,
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
			`SELECT COUNT(*) FROM destinations WHERE id = $1 AND account_id = $2 AND deleted_at IS NULL`,
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
				ON CONFLICT (domain_id, rule) UPDATE deleted_at = NULL RETURNING id
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

func (s *Site) getPostManageAlias() (*route, error) {
	r := &route{
		path:    "/alias/manage/:hash",
		methods: []string{"GET", "POST"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/manage_alias.html")
	if err != nil {
		return r, err
	}

	type Destination struct {
		account.Destination
		HID string
	}

	// definte template data
	type data struct {
		Route string

		Alias                account.Alias
		Domain               account.Domain
		Destinations         []account.Destination
		ExistingDestinations []Destination

		Errors FormErrors
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "aliases",
			Errors: newFormErrors(),
		}

		ids := s.idHasher.Decode(ps.ByName("hash"))
		if len(ids) != 1 {
			return errors.New("No id found")
		}

		aliasID := ids[0]

		// get alias
		err = pgxscan.Get(
			req.Context(),
			s.db,
			&d.Alias,
			`
			SELECT a.* 
			FROM aliases AS a 
				JOIN domains AS d ON a.domain_id = d.id 
			WHERE a.id = $1 AND d.account_id = $2 AND d.deleted_at IS NULL
			`,
			aliasID,
			accountID,
		)
		if err != nil {
			return errors.WithMessage(err, "Get Alias")
		}

		// get domain
		err = pgxscan.Get(
			req.Context(),
			s.db,
			&d.Domain,
			`
			SELECT * 
			FROM domains
			WHERE id = $1
			`,
			d.Alias.DomainID,
		)
		if err != nil {
			return errors.WithMessage(err, "Get Domain")
		}

		// select destinations
		err = pgxscan.Select(
			req.Context(),
			s.db,
			&d.Destinations,
			`
			SELECT * 
			FROM destinations 
			WHERE account_id = $1 AND id NOT IN (
				SELECT destination_id FROM alias_destinations WHERE alias_id = $2		
			) AND deleted_at IS NULL ORDER BY address`,
			accountID,
			aliasID,
		)
		if err != nil {
			return errors.WithMessage(err, "Select destinations")
		}

		err = pgxscan.Select(
			req.Context(),
			s.db,
			&d.ExistingDestinations,
			`
			SELECT d.* 
			FROM destinations AS d 
				JOIN alias_destinations AS ad on ad.destination_id = d.id
			WHERE d.account_id = $1 AND ad.alias_id = $2 AND d.deleted_at IS NULL
			ORDER BY d.address
			`,
			accountID,
			d.Alias.ID,
		)
		if err != nil {
			return errors.WithMessage(err, "Select destinations")
		}

		for idx := range d.ExistingDestinations {
			d.ExistingDestinations[idx].HID, err = s.idHasher.Encode([]int{
				aliasID,
				d.ExistingDestinations[idx].ID,
			})
			if err != nil {
				return err
			}
		}

		if req.Method == "GET" {
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		destinationID, err := strconv.Atoi(req.FormValue("destination"))
		if err != nil {
			return errors.WithMessage(err, "Atoi destination")
		}

		// validate that destination belongs to this account
		var count int
		err = s.db.QueryRow(
			req.Context(),
			`SELECT COUNT(*) FROM destinations WHERE id = $1 AND account_id = $2 AND delted_at IS NULL`,
			destinationID,
			accountID,
		).Scan(&count)
		if err != nil {
			return errors.WithMessage(err, "Validate Destination")
		}

		if count != 1 {
			d.Errors.Add("domain", "Invalid Destination")
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

func (s *Site) getDeleteAliasDestination() (*route, error) {
	r := &route{
		path:    "/alias/destination/delete/:hash",
		methods: []string{"GET"},
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		ids := s.idHasher.Decode(ps.ByName("hash"))
		if len(ids) != 2 {
			return errors.New("No id found")
		}

		aliasID := ids[0]
		destinationID := ids[1]

		var alias account.Alias
		var destination account.Destination

		// get alias
		err := pgxscan.Get(
			req.Context(),
			s.db,
			&alias,
			`
			SELECT a.* 
			FROM aliases AS a 
				JOIN domains AS d ON a.domain_id = d.id 
			WHERE a.id = $1 AND d.account_id = $2 AND a.deleted_at IS NULL
			`,
			aliasID,
			accountID,
		)
		if err != nil {
			return errors.WithMessage(err, "Get Alias")
		}

		// get destination
		err = pgxscan.Get(
			req.Context(),
			s.db,
			&destination,
			`
			SELECT * 
			FROM destinations
			WHERE id = $1 AND account_id = $2 AND deleted_at IS NULL
			`,
			destinationID,
			accountID,
		)
		if err != nil {
			return errors.WithMessage(err, "Get Alias")
		}

		_, err = s.db.Exec(
			req.Context(),
			"DELETE FROM alias_destinations WHERE alias_id = $1 AND destination_id = $2",
			aliasID,
			destinationID,
		)
		if err != nil {
			return errors.WithMessage(err, "Delete")
		}

		aliasHID, err := s.idHasher.Encode([]int{aliasID})
		if err != nil {
			return err
		}

		http.Redirect(w, req, "/alias/manage/"+aliasHID, http.StatusFound)

		return nil
	}

	return r, nil
}

func (s *Site) getDeleteAlias() (*route, error) {
	r := &route{
		path:    "/alias/delete/:hash",
		methods: []string{"GET"},
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		ids := s.idHasher.Decode(ps.ByName("hash"))
		if len(ids) != 1 {
			return errors.New("No id found")
		}

		aliasID := ids[0]

		var alias account.Alias

		// get alias
		err := pgxscan.Get(
			req.Context(),
			s.db,
			&alias,
			`
			SELECT a.* 
			FROM aliases AS a 
				JOIN domains AS d ON a.domain_id = d.id 
			WHERE a.id = $1 AND d.account_id = $2
			`,
			aliasID,
			accountID,
		)
		if err != nil {
			return errors.WithMessage(err, "Get Alias")
		}

		tx, err := s.db.Begin(req.Context())
		if err != nil {
			return err
		}
		defer tx.Rollback(req.Context())

		_, err = tx.Exec(
			req.Context(),
			"DELETE FROM alias_destinations WHERE alias_id = $1",
			alias.ID,
		)
		if err != nil {
			return errors.WithMessage(err, "Delete")
		}

		_, err = tx.Exec(
			req.Context(),
			"UPDATE aliases SET deleted_at = NOW() WHERE ID = $1",
			alias.ID,
		)
		if err != nil {
			return errors.WithMessage(err, "Delete")
		}

		if err := tx.Commit(req.Context()); err != nil {
			return err
		}

		http.Redirect(w, req, "/aliases", http.StatusFound)

		return nil
	}

	return r, nil
}
