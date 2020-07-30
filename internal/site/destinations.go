package site

import (
	"log"
	"net/http"
	"strings"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
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
		HID     string
	}

	// definte template data
	type data struct {
		Route string

		Destinations []Destination
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "destinations",
		}

		err := pgxscan.Select(
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
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

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
				log.Printf("Insert err: %s", err)
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
func (s *Site) getDeleteDestination() (*route, error) {
	r := &route{
		path:    "/destinations/delete/:hash",
		methods: []string{"GET"},
	}

	// actual handler
	r.h = s.verifyAction(func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		ids := s.idHasher.Decode(ps.ByName("hash"))
		if len(ids) != 1 {
			return errors.New("No id found")
		}

		destinationID := ids[0]

		var count int
		err := tx.QueryRow(
			req.Context(),
			"SELECT COUNT(*) FROM destinations WHERE id = $1 AND deleted_at IS NULL",
			destinationID,
		).Scan(&count)
		if err != nil {
			return err
		}

		if count != 1 {
			return errors.New("Destination not found")
		}

		_, err = tx.Exec(
			req.Context(),
			"UPDATE alias_destinations SET deleted_at = NOW() WHERE destination_id = $1",
			destinationID,
		)
		if err != nil {
			return err
		}

		_, err = tx.Exec(
			req.Context(),
			"UPDATE destinations SET deleted_at = NOW() WHERE id = $1",
			destinationID,
		)
		if err != nil {
			return err
		}

		http.Redirect(w, req, "/destinations", http.StatusFound)

		return nil
	})

	return r, nil
}
