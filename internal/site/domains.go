package site

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jawr/mxax/internal/account"
	"github.com/jawr/whois-parser-go"
	"github.com/julienschmidt/httprouter"
	"github.com/likexian/whois-go"
)

// template only routes
func (s *Site) getAddDomain() (*route, error) {
	return s.templateResponse("/domains/add", "GET", "domains", "templates/pages/add_domain.html")
}

// others
func (s *Site) getDomains() (*route, error) {
	r := &route{
		path:   "/domains",
		method: "GET",
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/domains.html")
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
	}

	// definte template data
	type data struct {
		Route string

		Domains []Domain
	}

	// actual handler
	r.h = s.auth(func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {

		d := data{
			Route: "domains",
		}

		if err := pgxscan.Select(
			req.Context(),
			s.db,
			&d.Domains,
			`
			SELECT 
				d.*,
				COALESCE(COUNT(a.*)) as aliases,
				COALESCE(COUNT(r.*)) as records,
				COALESCE(COUNT(a.*) FILTER (WHERE catch_all = true)) as catch_all
			FROM domains AS d 
				LEFT JOIN aliases AS a ON d.id = a.domain_id 
				LEFT JOIN records AS r ON d.id = r.domain_id
			WHERE d.account_id = $1
			GROUP BY d.id
			`,
			accountID,
		); err != nil {
			s.handleError(w, r, err)
			return
		}

		// setup status
		for idx, dom := range d.Domains {
			if dom.VerifiedAt.Time.IsZero() {
				d.Domains[idx].Status = "unverified"
			} else if dom.Records != 3 {
				d.Domains[idx].Status = "incomplete"
			} else {
				d.Domains[idx].Status = "ready"
			}
		}

		if err := tmpl.ExecuteTemplate(w, "base", d); err != nil {
			s.handleError(w, r, err)
			return
		}
	})

	return r, nil
}

func (s *Site) postAddDomain() (*route, error) {
	r := &route{
		path:   "/domains/add",
		method: "POST",
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/add_domain.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route string

		Name   string
		Errors FormErrors
	}

	// actual handler
	r.h = s.auth(func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) {

		d := data{
			Route:  "domains",
			Errors: newFormErrors(),
		}

		name := req.FormValue("domain")

		// validations
		if len(name) == 0 {
			d.Errors.Add("domain", "No domain provided")
		}

		// get expires at for domain
		whoisResult, err := whois.Whois(name)
		if err != nil {
			s.handleError(w, r, err)
			return
		}

		// TODO
		// what other checks do we want to introduce here

		// create a code
		var verifyCode string
		var tries int
		for {
			if tries > 10 {
				s.handleError(w, r, errors.New("Too many tries creating a verify code. Please contact support."))
				return
			}

			n := 9
			b := make([]byte, n)
			if _, err := rand.Read(b); err != nil {
				s.handleError(w, r, err)
				return
			}

			verifyCode = fmt.Sprintf("%X", b)

			var count int
			err := s.db.QueryRow(req.Context(), "SELECT COUNT(*) FROM domains WHERE verify_code = $1", s).Scan(&count)
			if err != nil {
				s.handleError(w, r, err)
				return
			}

			if count == 0 {
				break
			}
		}

		// insert
		_, err := s.db.Exec(
			req.Context(),
			`
			INSERT INTO domains (account_id, name, verify_code, expires_at) 
				VALUES ($1, $2, $3, $4)
				`,
			accountID,
			name,
			verifyCode,
			expiresAt,
		)
		if err != nil {
			s.handleError(w, r, err)
			return
		}

		if d.Errors.Error() {
			if err := tmpl.ExecuteTemplate(w, "base", d); err != nil {
				s.handleError(w, r, err)
				return
			}
		} else {
			// redirect success to domains page
			http.Redirect(w, req, "/domains", http.StatusFound)
		}
	})

	return r, nil
}
