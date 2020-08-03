package smtp

import (
	"context"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
)

const defaultCacheTTL = time.Minute * 5

// ttl cached lookup of mx records for a domain
// ttl cached lookup of destinations for an alias

// ttl cached lookup of domain for an alias
func getDomain(db *pgx.Conn, cache *ristretto.Cache, aliasID int) (*account.Domain, error) {
	if domain, ok := cache.Get(aliasID); ok {
		return domain.(*account.Domain), nil
	}

	var domain account.Domain
	err := pgxscan.Get(
		context.Background(),
		db,
		&domain,
		"SELECT d.* FROM domains AS d JOIN aliases AS a ON d.id = a.domain_id WHERE a.id = $1",
		aliasID,
	)
	if err != nil {
		return nil, err
	}

	cache.SetWithTTL(aliasID, &domain, 1, defaultCacheTTL)

	return &domain, nil
}

// ttl cached lookup of dkim private key for a domain
