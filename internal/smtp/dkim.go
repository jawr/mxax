package smtp

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/pkg/errors"
)

func (s *Server) dkimSignHandler(session *SessionData, reader io.Reader, writer io.Writer) error {
	key, err := s.getDkimPrivateKey(session.Domain.ID)
	if err != nil {
		return errors.WithMessage(err, "getDkimPrivateKey")
	}

	opts := dkim.SignOptions{
		Domain:   session.Domain.Name,
		Selector: "mxax",
		Signer:   key,
		Hash:     crypto.SHA256,
	}

	if err := dkim.Sign(writer, reader, &opts); err != nil {
		return errors.Wrap(err, "dkim.Sign")
	}

	return nil
}

func (s *Server) getDkimPrivateKey(domainID int) (*rsa.PrivateKey, error) {
	if key, ok := s.cacheGet("dkim", fmt.Sprintf("%d", domainID)); ok {
		return key.(*rsa.PrivateKey), nil
	}

	var privateKey []byte
	err := s.db.QueryRow(
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

	s.cacheSet("dkim", fmt.Sprintf("%d", domainID), key)

	return key, nil
}
