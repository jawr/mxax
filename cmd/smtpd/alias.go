package main

import (
	"context"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jess/mxax/internal/account"
	"github.com/jess/mxax/internal/smtp"
	"github.com/pkg/errors"
)

func makeAliasHandler(db *pgx.Conn) (smtp.AliasHandler, error) {

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

	return func(ctx context.Context, email string) (int, error) {

		if _, ok := nxmatch.Get(email); ok {
			return 0, errors.New("No match")
		}

		if aliasID, ok := matches.Get(email); ok {
			return aliasID.(int), nil
		}

		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			return 0, errors.New("Malformed email address")
		}

		user := parts[0]
		domain := parts[1]

		// check if this is a bad domain we have checked already
		if _, ok := nxdomain.Get(domain); ok {
			return 0, errors.New("Domain not accepted")
		}

		// search for domain in the database
		all, ok := aliases.Get(domain)
		if !ok {
			if err := pgxscan.Select(ctx, db, &all, "SELECT a.* FROM aliases AS a JOIN domains AS d ON a.domain_id = d.id WHERE d.name = $1 AND d.deleted_at IS NULL AND d.verified_at IS NOT NULL", domain); err != nil {
				nxdomain.SetWithTTL(domain, struct{}{}, 1, defaultTTL)

				return 0, errors.New("Domain not accepted")
			}
			aliases.SetWithTTL(domain, all, 1, defaultTTL)
		}

		// check for matches
		for _, i := range all.([]account.Alias) {
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

		return 0, errors.New("No match")
	}, nil
}
