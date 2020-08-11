package site

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"sync"

	"github.com/dpapathanasiou/go-recaptcha"
	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type Site struct {
	db         *pgx.Conn
	router     *httprouter.Router
	bufferPool sync.Pool

	emailPublisher *rabbitmq.Channel

	dkimKey *rsa.PrivateKey

	recaptchaPublicKey string
}

func NewSite(db *pgx.Conn, emailPublisher *rabbitmq.Channel) (*Site, error) {
	s := &Site{
		db: db,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
		emailPublisher:     emailPublisher,
		recaptchaPublicKey: os.Getenv("MXAX_RECAPTCHA_PUBLIC_KEY"),
	}

	recaptcha.Init(os.Getenv("MXAX_RECAPTCHA_PRIVATE_KEY"))

	var privateKey []byte
	err := db.QueryRow(
		context.Background(),
		`
		SELECT k.private_key 
		FROM dkim_keys AS k 
			JOIN domains AS d on k.domain_id = d.id
		WHERE 
			d.name = 'mx.ax'
		`,
	).Scan(&privateKey)
	if err != nil {
		return nil, err
	}

	d, _ := pem.Decode(privateKey)
	if d == nil {
		return nil, errors.New("pem.Decode")
	}

	s.dkimKey, err = x509.ParsePKCS1PrivateKey(d.Bytes)
	if err != nil {
		return nil, errors.WithMessage(err, "x509.ParsePKCS1PrivateKey")
	}

	if err := s.setupRoutes(); err != nil {
		return nil, errors.WithMessage(err, "setupRoutes")
	}

	return s, nil
}

func (s *Site) Run(addr string) error {
	return http.ListenAndServe(addr, s.router)
}
