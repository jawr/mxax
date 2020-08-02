package smtp

import (
	"bytes"
	"crypto"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

type dkimSignHandlerFn func()

func (s *Server) makeDkimSignHandler(db *pgx.Conn) (dkimSignHandlerFn, error) {
	// get dkim and proceed to sign
	key, err := getDkimPrivateKey(db, dkimKeyCache, domain.ID)
	if err != nil {
		return errors.WithMessage(err, "getDkimPrivateKey")
	}

	// sign the email
	opts := dkim.SignOptions{
		Domain:   domain.Name,
		Selector: "mxax",
		Signer:   key,
		Hash:     crypto.SHA256,
	}

	b := s.bufferPool.Get().(*bytes.Buffer)
	b.Reset()
	defer s.bufferPool.Put(b)

	if err := dkim.Sign(b, final, &opts); err != nil {
		return errors.Wrap(err, "dkim.Sign")
	}

}
