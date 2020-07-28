package site

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jawr/mxax/internal/account"
	"github.com/julienschmidt/httprouter"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
)

// display overview information about all domains
func (s *Site) getDomains() (*route, error) {
	r := &route{
		path:    "/domains",
		methods: []string{"GET"},
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
		Expired  bool
	}

	// definte template data
	type data struct {
		Route string

		Domains []Domain
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

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
				COALESCE(COUNT(DISTINCT a.id)) as aliases,
				COALESCE(COUNT(DISTINCT r.id) FILTER (
					WHERE last_verified_at IS NOT NULL 
					OR last_verified_at > NOW() - INTERVAL '24 hours'
				)) as records,
				COALESCE(COUNT(DISTINCT a.id) FILTER (WHERE rule = '.*')) as catch_all
			FROM domains AS d 
				LEFT JOIN aliases AS a ON d.id = a.domain_id 
				LEFT JOIN records AS r ON d.id = r.domain_id
			WHERE d.account_id = $1
			GROUP BY d.id
			ORDER BY d.name
			`,
			accountID,
		); err != nil {
			return err
		}

		// setup status
		for idx, dom := range d.Domains {
			if dom.VerifiedAt.Time.IsZero() {
				d.Domains[idx].Status = "unverified"
			} else if dom.Records != 4 {
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

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}

// add a domain
// if there are validation issues it will return
// the add page and display said errors
// otherwise it will return to the main domains page
func (s *Site) getPostAddDomain() (*route, error) {
	r := &route{
		path:    "/domains/add",
		methods: []string{"GET", "POST"},
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
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "domains",
			Errors: newFormErrors(),
		}

		if req.Method == "GET" {
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		name := req.FormValue("domain")

		// validations
		if len(name) == 0 {
			d.Errors.Add("domain", "No domain provided")
		}

		// get expires at for domain
		// also acts as an additional layer of
		// validation, might be too noisey/error prone
		expiresAt, err := account.GetDomainExpirationDate(name)
		if err != nil {
			d.Errors.Add("domain", err.Error())
		}

		// TODO
		// what other checks do we want to introduce here

		if !d.Errors.Error() {
			// create a code
			var verifyCode string
			var tries int
			for {
				if tries > 10 {
					return errors.New("Too many tries creating a verify code. Please contact support.")
				}

				n := 11
				b := make([]byte, n)
				if _, err := rand.Read(b); err != nil {
					return err
				}

				verifyCode = fmt.Sprintf("mxax-%X", b)

				var count int
				err := s.db.QueryRow(req.Context(), "SELECT COUNT(*) FROM domains WHERE verify_code = $1", verifyCode).Scan(&count)
				if err != nil {
					return errors.WithMessage(err, "Select VerifyCode count")
				}

				if count == 0 {
					break
				}
			}

			// insert, first create a transaction so we keep a clean state on
			// an error
			tx, err := s.db.Begin(req.Context())
			if err != nil {
				return errors.WithMessage(err, "Insert")
			}
			// TODO:
			// will this context cancel the rollback??
			defer tx.Rollback(req.Context())

			var id int
			err = tx.QueryRow(
				req.Context(),
				`
			INSERT INTO domains (account_id, name, verify_code, expires_at) 
				VALUES ($1, $2, $3, $4)
				RETURNING id
				`,
				accountID,
				name,
				verifyCode,
				expiresAt,
			).Scan(&id)
			if err != nil {
				return errors.WithMessage(err, "Insert")
			}

			// create dkim record
			dkimKey, err := account.NewDkimKey(id)
			if err != nil {
				return errors.WithMessage(err, "NewDkimKey")
			}

			// insert dkim
			_, err = tx.Exec(
				req.Context(),
				"INSERT INTO dkim_keys (domain_id, private_key, public_key) VALUES ($1, $2, $3)",
				id,
				dkimKey.PrivateKey,
				dkimKey.PublicKey,
			)
			if err != nil {
				return errors.WithMessage(err, "Insert DkimKey")
			}

			// insert dkim record
			_, err = tx.Exec(
				req.Context(),
				"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
				id,
				"mxax._domainkey",
				"TXT",
				dkimKey.String(),
			)
			if err != nil {
				return errors.WithMessage(err, "Insert DkimKey Record")
			}

			// insert mx
			_, err = tx.Exec(
				req.Context(),
				"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
				id,
				"@",
				"MX",
				"10 smtp.mx.ax.",
			)
			if err != nil {
				return errors.WithMessage(err, "Insert MX Record")
			}

			// TODO
			// host a second mx for redundancy

			// insert spf
			_, err = tx.Exec(
				req.Context(),
				"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
				id,
				"@",
				"TXT",
				`"v=spf1 include:spf.mx.ax ~all"`,
			)
			if err != nil {
				return errors.WithMessage(err, "Insert SPF Record")
			}

			// insert dmarc
			_, err = tx.Exec(
				req.Context(),
				"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
				id,
				"_dmarc",
				"TXT",
				`"v=DMARC1; p=quarantine"`,
			)
			if err != nil {
				return errors.WithMessage(err, "Insert DkimKey Record")
			}

			if err := tx.Commit(req.Context()); err != nil {
				return errors.WithMessage(err, "Commit")
			}

			// redirect success to domains page
			http.Redirect(w, req, "/domain/"+name, http.StatusFound)

			return nil
		}

		// otherwise display errors
		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}

// get specific information about a domain
// depending on the state will display
// different templates
func (s *Site) getDomain() (*route, error) {
	r := &route{
		path:    "/domain/:domain",
		methods: []string{"GET"},
	}

	// setup templates
	verifyTmpl, err := s.loadTemplate("templates/pages/verify_domain.html")
	if err != nil {
		return r, err
	}

	tmpl, err := s.loadTemplate("templates/pages/view_domain.html")
	if err != nil {
		return r, err
	}

	type Domain struct {
		account.Domain
		Records []account.Record
	}

	// definte template data
	type data struct {
		Route  string
		Domain Domain
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route: "domains",
		}

		err := pgxscan.Get(
			req.Context(),
			s.db,
			&d.Domain,
			"SELECT * FROM domains WHERE account_id = $1 AND name = $2",
			accountID,
			ps.ByName("domain"),
		)
		if err != nil {
			return err
		}

		err = pgxscan.Select(
			req.Context(),
			s.db,
			&d.Domain.Records,
			"SELECT * FROM records WHERE domain_id = $1 ORDER BY rtype, host, id",
			d.Domain.ID,
		)
		if err != nil {
			return err
		}

		// check if domain status is complete
		isComplete := len(d.Domain.Records) == 4
		for _, rr := range d.Domain.Records {
			if !rr.IsComplete() {
				isComplete = false
				break
			}
		}

		// verify domain
		if d.Domain.VerifiedAt.Time.IsZero() && !isComplete {
			s.renderTemplate(w, verifyTmpl, r, d)
			return nil
		}

		// if incomplete
		if !isComplete {
			http.Redirect(w, req, fmt.Sprintf("/domain/%s/check", d.Domain.Name), http.StatusFound)
			return nil
		}

		// finally complete
		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}

// check and see if the associated verify code exists
func (s *Site) postVerifyDomain() (*route, error) {
	r := &route{
		path:    "/domain/:domain/verify",
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
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "domains",
			Errors: newFormErrors(),
		}

		err := pgxscan.Get(
			req.Context(),
			s.db,
			&d.Domain,
			"SELECT * FROM domains WHERE account_id = $1 AND name = $2",
			accountID,
			ps.ByName("domain"),
		)
		if err != nil {
			return err
		}

		if err := d.Domain.CheckVerifyCode(dnsConfig); err != nil {
			d.Errors.Add("", err.Error())
		}

		if d.Errors.Error() {
			s.renderTemplate(w, tmpl, r, d)
			return nil
		}

		_, err = s.db.Exec(
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

// check the records associated with a domain exist
func (s *Site) getCheckDomain() (*route, error) {
	r := &route{
		path:    "/domain/:domain/check",
		methods: []string{"GET"},
	}

	// setup templates
	tmpl, err := s.loadTemplate("templates/pages/check_domain.html")
	if err != nil {
		return r, err
	}

	// definte template data
	type data struct {
		Route   string
		Errors  FormErrors
		Domain  account.Domain
		Records []account.Record
	}

	// go net.LookupCNAME follows the Canonical chain
	dnsConfig, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return r, errors.WithMessage(err, "dns.ClientConfigFromFile")
	}

	// actual handler
	r.h = func(accountID int, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "domains",
			Errors: newFormErrors(),
		}

		err := pgxscan.Get(
			req.Context(),
			s.db,
			&d.Domain,
			"SELECT * FROM domains WHERE account_id = $1 AND name = $2 AND verified_at IS NOT NULL",
			accountID,
			ps.ByName("domain"),
		)
		if err != nil {
			return err
		}

		err = pgxscan.Select(
			req.Context(),
			s.db,
			&d.Records,
			"SELECT * FROM records WHERE domain_id = $1 ORDER BY rtype, host, id",
			d.Domain.ID,
		)
		if err != nil {
			return err
		}

		for _, rr := range d.Records {
			if err := rr.Check(d.Domain.Name, dnsConfig); err != nil {
				d.Errors.Add(rr.Value, err.Error())
				continue
			}

			_, err = s.db.Exec(
				req.Context(),
				"UPDATE records SET last_verified_at = NOW() WHERE id = $1",
				rr.ID,
			)
			if err != nil {
				return err
			}
		}

		s.renderTemplate(w, tmpl, r, d)
		return nil
	}

	return r, nil
}
