package site

import (
	"net/http"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jawr/mxax/internal/account"
	"github.com/julienschmidt/httprouter"
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
					STRING_AGG(d.address, ',') AS destinations
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
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "aliases",
		}

		// get domains and destinations

		if req.Method == "GET" {
			return err
		}

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}
