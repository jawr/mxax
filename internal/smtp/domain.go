package smtp

import (
	"context"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
	"github.com/pkg/errors"
)

// DomainHandler checks to see if the domain is valid
// and if the domain has any domaines attached that
// match this email address
type domainHandlerFn func(string) (int, int, error)

func (s *Server) makeDomainHandler(db *pgx.Conn) (domainHandlerFn, error) {

	nxdomain, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	domains, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	const defaultTTL = time.Minute * 5

	return func(email string) (int, int, error) {
		email = strings.ToLower(email)

		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			return 0, 0, errors.Errorf("bad email: '%s'", email)
		}

		domain := parts[1]

		if e, ok := domains.Get(domain); ok {
			d := e.(account.Domain)
			return d.AccountID, d.ID, nil
		}

		// check if this is a bad domain we have checked already
		if _, ok := nxdomain.Get(domain); ok {
			return 0, 0, errors.Errorf("nxdomain cache hit for '%s'", domain)
		}

		// search for domain in the database
		var dom account.Domain

		err := pgxscan.Get(
			context.Background(),
			db,
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
			nxdomain.SetWithTTL(domain, struct{}{}, 1, defaultTTL)
			return 0, 0, err
		}

		domains.SetWithTTL(domain, dom, 1, defaultTTL)

		return dom.AccountID, dom.ID, nil
	}, nil
}
