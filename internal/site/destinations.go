package site

import (
	"net/http"

	"github.com/jackc/pgx/v4"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Site) getDeleteDestination() (*route, error) {
	r := &route{
		path:    "/destinations/delete/:hash",
		methods: []string{"GET"},
	}

	// actual handler
	r.h = s.verifyAction(func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		ids := s.idHasher.Decode(ps.ByName("hash"))
		if len(ids) != 1 {
			return errors.New("No id found")
		}

		destinationID := ids[0]

		var count int
		err := tx.QueryRow(
			req.Context(),
			"SELECT COUNT(*) FROM destinations WHERE id = $1 AND deleted_at IS NULL",
			destinationID,
		).Scan(&count)
		if err != nil {
			return err
		}

		if count != 1 {
			return errors.New("Destination not found")
		}

		_, err = tx.Exec(
			req.Context(),
			"UPDATE alias_destinations SET deleted_at = NOW() WHERE destination_id = $1",
			destinationID,
		)
		if err != nil {
			return err
		}

		_, err = tx.Exec(
			req.Context(),
			"UPDATE destinations SET deleted_at = NOW() WHERE id = $1",
			destinationID,
		)
		if err != nil {
			return err
		}

		http.Redirect(w, req, "/", http.StatusFound)

		return nil
	})

	return r, nil
}
