package smtp

import (
	"context"
	"strings"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jawr/mxax/internal/account"
	"github.com/pkg/errors"
)

// DomainDetector checks to see if the domain is valid
// and if the domain has any domaines attached that
// match this email address
func (s *Server) detectDomain(email string) (account.Domain, error) {
	email = strings.ToLower(email)

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return account.Domain{}, errors.Errorf("bad email: '%s'", email)
	}

	domain := parts[1]

	if e, ok := s.cacheGet("domain", domain); ok {
		d := e.(account.Domain)
		return d, nil
	}

	// check if this is a bad domain we have checked already
	if _, ok := s.cacheGet("nxdomain", domain); ok {
		return account.Domain{}, errors.Errorf("nxdomain cache hit for '%s'", domain)
	}

	// search for domain in the database
	var dom account.Domain
	err := pgxscan.Get(
		context.Background(),
		s.db,
		&dom,
		`
				SELECT * FROM domains
				WHERE name = $1 
					AND deleted_at IS NULL 
					AND verified_at IS NOT NULL
				LIMIT 1
				`,
		domain,
	)
	if err != nil {
		s.cacheSet("nxdomain", domain, struct{}{})
		return account.Domain{}, err
	}

	s.cacheSet("domain", domain, dom)

	return dom, nil
}
