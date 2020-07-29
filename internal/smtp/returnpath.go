package smtp

import (
	"context"
	"strings"

	"github.com/dgraph-io/ristretto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

type returnPathHandlerFn = func(string) (uuid.UUID, string, error)

func (s *Server) makeReturnPathHandler(db *pgx.Conn) (returnPathHandlerFn, error) {
	nx, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	return func(to string) (uuid.UUID, string, error) {
		// check nx
		if _, ok := nx.Get(to); ok {
			return uuid.Nil, "", errors.Errorf("nx cache hit for '%s'", to)
		}

		parts := strings.Split(to, "@")
		if len(parts) != 2 {
			return uuid.Nil, "", errors.Errorf("bad email: '%s'", to)
		}

		parts = strings.Split(parts[0], "=")
		if len(parts) != 2 {
			return uuid.Nil, "", errors.Errorf("not an mxax retun path: '%s'", to)
		}

		returnPath := parts[1]

		// dirty check
		id, err := uuid.Parse(returnPath)
		if err != nil {
			return uuid.Nil, "", errors.WithMessagef(err, "Parse: '%s'", returnPath)
		}
		if id == uuid.Nil {
			return uuid.Nil, "", errors.Errorf("Nil uuid for '%s'", returnPath)
		}

		// check db
		var replyTo string
		err = db.QueryRow(
			context.Background(),
			"SELECT return_to FROM return_paths WHERE id = $1",
			id,
		).Scan(&replyTo)
		if err != nil {
			return uuid.Nil, "", errors.WithMessage(err, "Select")
		}

		// if nothing found update and return
		if len(replyTo) == 0 {
			nx.Set(to, struct{}{}, 1)
			return uuid.Nil, "", nil
		}

		_, err = db.Exec(
			context.Background(),
			"UPDATE return_paths SET returned_at = NOW() WHERE id = $1",
			id,
		)
		if err != nil {
			return uuid.Nil, "", errors.WithMessage(err, "Update")
		}

		return id, replyTo, nil
	}, nil
}
