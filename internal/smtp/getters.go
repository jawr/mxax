package smtp

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"sort"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
	"github.com/pkg/errors"
)

const defaultCacheTTL = time.Hour * 24

// ttl cached lookup of mx records for a domain
func getDestinationMXs(cache *ristretto.Cache, domain string) ([]*net.MX, error) {
	if mxs, ok := cache.Get(domain); ok {
		return mxs.([]*net.MX), nil
	}

	mxs, err := net.LookupMX(domain)
	if err != nil {
		return nil, errors.WithMessage(err, "LookupMX")
	}

	if len(mxs) == 0 {
		return nil, errors.Errorf("Found no MX domains for %s", domain)
	}

	sort.Slice(mxs, func(i, j int) bool {
		return mxs[i].Pref < mxs[j].Pref
	})

	cache.SetWithTTL(domain, mxs, 1, defaultCacheTTL)

	return mxs, nil
}

// ttl cached lookup of destinations for an alias
func getDestinations(db *pgx.Conn, cache *ristretto.Cache, aliasID int) ([]account.Destination, error) {
	if destinations, ok := cache.Get(aliasID); ok {
		return destinations.([]account.Destination), nil
	}

	var destinations []account.Destination
	err := pgxscan.Select(
		context.Background(),
		db,
		&destinations,
		"SELECT d.* FROM destinations AS d JOIN alias_destinations AS ad ON d.id = ad.destination_id WHERE ad.alias_id = $1",
		aliasID,
	)
	if err != nil {
		return nil, err
	}

	cache.SetWithTTL(aliasID, destinations, 1, defaultCacheTTL)

	return destinations, nil
}

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
func getDkimPrivateKey(db *pgx.Conn, cache *ristretto.Cache, domainID int) (*rsa.PrivateKey, error) {
	if key, ok := cache.Get(domainID); ok {
		return key.(*rsa.PrivateKey), nil
	}

	var privateKey []byte
	err := db.QueryRow(
		context.Background(),
		"SELECT private_key FROM dkim_keys WHERE domain_id = $1",
		domainID,
	).Scan(&privateKey)
	if err != nil {
		return nil, errors.WithMessage(err, "Select")
	}

	d, _ := pem.Decode(privateKey)
	if d == nil {
		return nil, errors.New("pem.Decode")
	}

	key, err := x509.ParsePKCS1PrivateKey(d.Bytes)
	if err != nil {
		return nil, errors.WithMessage(err, "x509.ParsePKCS1PrivateKey")
	}

	cache.SetWithTTL(domainID, key, 1, time.Hour*24)

	return key, nil
}
