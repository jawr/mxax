package smtp

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

func (s *Server) detectReturnPath(to string) (uuid.UUID, string, error) {
	// check nx
	if _, ok := s.cache.Get("nxreturnpath", to); ok {
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
	err = s.db.QueryRow(
		context.Background(),
		"SELECT return_to FROM return_paths WHERE id = $1",
		id,
	).Scan(&replyTo)
	if err != nil {
		return uuid.Nil, "", errors.WithMessage(err, "Select")
	}

	// if nothing found update and return
	if len(replyTo) == 0 {
		s.cache.Set("nx", to, struct{}{})
		return uuid.Nil, "", nil
	}

	_, err = s.db.Exec(
		context.Background(),
		"UPDATE return_paths SET returned_at = NOW() WHERE id = $1",
		id,
	)
	if err != nil {
		return uuid.Nil, "", errors.WithMessage(err, "Update")
	}

	return id, replyTo, nil
}

func (s *Server) makeReturnPath(session *SessionData) (string, error) {
	parts := strings.Split(strings.Replace(session.To, "=", "", -1), "@")

	if len(parts) != 2 {
		return "", errors.Errorf("Invalid email: '%s'", session.To)
	}

	returnPath := fmt.Sprintf("%s=%s@%s", parts[0], session.ID, session.Domain.Name)

	// write return path

	_, err := s.db.Exec(
		context.Background(),
		"INSERT INTO return_paths (id, account_id, alias_id, return_to) VALUES ($1, $2, $3, $4)",
		session.ID,
		session.Domain.AccountID,
		session.Alias.ID,
		session.From,
	)
	if err != nil {
		return "", errors.WithMessage(err, "Insert ReturnPath")
	}

	return returnPath, nil
}
