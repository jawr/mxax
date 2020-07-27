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

// AliasHandler checks to see if the domain is valid
// and if the domain has any aliases attached that
// match this email address
type aliasHandlerFn func(string) (int, error)

func (s *Server) makeAliasHandler(db *pgx.Conn) (aliasHandlerFn, error) {

	nxdomain, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	nxmatch, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	matches, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	aliases, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	const defaultTTL = time.Minute * 5

	return func(email string) (int, error) {

		if _, ok := nxmatch.Get(email); ok {
			return 0, errors.Errorf("nxmatch cache hit for '%s'", email)
		}

		if aliasID, ok := matches.Get(email); ok {
			return aliasID.(int), nil
		}

		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			return 0, errors.Errorf("bad email: '%s'", email)
		}

		user := parts[0]
		domain := parts[1]

		// check if this is a bad domain we have checked already
		if _, ok := nxdomain.Get(domain); ok {
			return 0, errors.Errorf("nxdomain cache hit for '%s'", domain)
		}

		// search for domain in the database
		var all []account.Alias
		cacheAll, ok := aliases.Get(domain)

		if !ok {
			err := pgxscan.Select(
				context.Background(),
				db,
				&all,
				`
				SELECT a.* 
				FROM aliases AS a 
					JOIN domains AS d ON a.domain_id = d.id 
				WHERE d.name = $1 
					AND d.deleted_at IS NULL 
					AND d.verified_at IS NOT NULL`,
				domain,
			)
			if err != nil {
				nxdomain.SetWithTTL(domain, struct{}{}, 1, defaultTTL)
				return 0, err
			}

			aliases.SetWithTTL(domain, all, 1, defaultTTL)

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
				matches.SetWithTTL(email, i.ID, 1, defaultTTL)
				return i.ID, nil
			}
		}

		// no matches found, update nxmatch and return
		nxmatch.SetWithTTL(email, struct{}{}, 1, defaultTTL)

		return 0, errors.New("nxmatch")
	}, nil
}
