package site

import (
	"net/http"
	"strings"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jawr/mxax/internal/account"
	"github.com/julienschmidt/httprouter"
)

func (s *Site) getDestinations() (*route, error) {
	r := &route{
		path:    "/destinations",
		methods: []string{"GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/destinations.html")
	if err != nil {
		return r, err
	}

	type Destination struct {
		account.Destination
		Aliases int
	}

	// definte template data
	type data struct {
		Route string

		Destinations []Destination
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "destinations",
		}

		err := pgxscan.Select(
			req.Context(),
			s.db,
			&d.Destinations,
			`
			SELECT 
				d.*, 
				COALESCE(COUNT(ad.*), 0) AS aliases
			FROM destinations AS d
				LEFT JOIN alias_destinations AS ad ON d.id = ad.destination_id
			WHERE d.account_id = $1
			GROUP BY d.id
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

func (s *Site) getPostCreateDestination() (*route, error) {
	r := &route{
		path:    "/destinations/create",
		methods: []string{"POST", "GET"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/create_destination.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string

		Address string
		Errors  FormErrors
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "destinations",
			Errors: newFormErrors(),
		}

		if req.Method == "GET" {
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		address := req.FormValue("address")

		// validations
		if len(address) == 0 {
			d.Errors.Add("address", "No address provided")
		}

		if strings.Count(address, "@") != 1 {
			d.Errors.Add("address", "Does not look like an email address")
		}

		// TODO
		// what other checks do we want to introduce here

		if !d.Errors.Error() {

			_, err := s.db.Exec(
				req.Context(),
				"INSERT INTO destinations (account_id, address) VALUES ($1, $2)",
				accountID,
				strings.ToLower(address),
			)
			if err != nil {
				d.Errors.Add("address", "Address already exists")

			} else {
				// redirect success to addresss page
				http.Redirect(w, req, "/destinations", http.StatusFound)
				return nil
			}
		}

		// otherwise display errors
		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}
