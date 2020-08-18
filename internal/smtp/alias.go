package smtp

import (
	"context"
	"strings"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jawr/mxax/internal/account"
	"github.com/pkg/errors"
)

// AliasDetector checks to see if the domain is valid
// and if the domain has any aliases attached that
// match this email address
func (s *Server) detectAlias(email string) (account.Alias, error) {
	email = strings.ToLower(email)

	if _, ok := s.cache.Get("alias:nxmatch", email); ok {
		return account.Alias{}, errors.Errorf("nxmatch cache hit for '%s'", email)
	}

	if alias, ok := s.cache.Get("alias:match", email); ok {
		return alias.(account.Alias), nil
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return account.Alias{}, errors.Errorf("bad email: '%s'", email)
	}

	user := parts[0]
	domain := parts[1]

	// check if this is a bad domain we have checked already
	if _, ok := s.cache.Get("nxdomain", domain); ok {
		return account.Alias{}, errors.Errorf("nxdomain cache hit for '%s'", domain)
	}

	// search for domain in the database
	var all []account.Alias
	cacheAll, ok := s.cache.Get("aliases", domain)

	if !ok {
		err := pgxscan.Select(
			context.Background(),
			s.db,
			&all,
			`
				SELECT a.* 
				FROM aliases AS a 
					JOIN domains AS d ON a.domain_id = d.id 
				WHERE d.name = $1 
					AND a.deleted_at IS NULL 
					AND d.deleted_at IS NULL 
					AND d.verified_at IS NOT NULL
				ORDER BY LENGTH(a.rule) DESC
				`,
			domain,
		)
		if err != nil {
			s.cache.Set("alias:nxdomain", domain, struct{}{})
			return account.Alias{}, err
		}

		s.cache.Set("alias:domain", domain, all)

	} else {
		all = cacheAll.([]account.Alias)
	}

	// check for matches
	for _, i := range all {
		ok, err := i.Check(user)
		if err != nil {
			continue
		}
		if ok {
			s.cache.Set("alias:match", email, i)
			return i, nil
		}
	}

	// no matches found, update nxmatch and return
	s.cache.Set("nxmatch", email, struct{}{})

	return account.Alias{}, errors.New("nxmatch")
}
