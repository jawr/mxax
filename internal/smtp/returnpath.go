package smtp

import (
	"context"
	"strings"

	"github.com/dgraph-io/ristretto"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

type ReturnPathHandler = func(string) (string, error)

func MakeReturnPathHandler(db *pgx.Conn) (ReturnPathHandler, error) {
	nx, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	return func(to string) (string, error) {
		// check nx
		if _, ok := nx.Get(to); ok {
			return "", errors.Errorf("nx cache hit for '%s'", to)
		}

		parts := strings.Split(to, "@")
		if len(parts) != 2 {
			return "", errors.Errorf("bad email: '%s'", to)
		}

		// check db
		var replyTo string
		err := db.QueryRow(
			context.Background(),
			"SELECT return_to FROM return_paths WHERE id = $1",
			parts[0],
		).Scan(&replyTo)
		if err != nil {
			return "", errors.WithMessage(err, "Select")
		}

		// if nothing found update and return
		if len(replyTo) == 0 {
			nx.Set(to, struct{}{}, 1)
			return "", nil
		}

		// TODO
		// update db
		return replyTo, nil
	}, nil
}
