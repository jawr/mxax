package account

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/pkg/errors"
)

type DkimKey struct {
	ID int

	DomainID int

	PrivateKey []byte
	PublicKey  []byte

	MetaData
}

func NewDkimKey(domainID int) (*DkimKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, errors.WithMessage(err, "rsa.GenerateKey")
	}

	// private
	privBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	privateKey := pem.EncodeToMemory(privBlock)
	if privateKey == nil {
		return nil, errors.New("encoding private key to pem")
	}

	// public
	pubKeyPKIX, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, errors.WithMessage(err, "x509.MarshalPKIXPublicKey")
	}

	pubBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyPKIX,
	}

	publicKey := pem.EncodeToMemory(pubBlock)
	if publicKey == nil {
		return nil, errors.New("encoding public key to pem")
	}

	// create
	publicKey = bytes.Replace(publicKey, []byte("-----BEGIN PUBLIC KEY-----"), []byte(""), -1)
	publicKey = bytes.Replace(publicKey, []byte("-----END PUBLIC KEY-----"), []byte(""), -1)
	publicKey = bytes.Replace(publicKey, []byte("\n"), []byte(""), -1)

	dkimKey := DkimKey{
		DomainID:   domainID,
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}

	return &dkimKey, nil
}

func (dk DkimKey) String() string {
	return fmt.Sprintf(
		"v=DKIM1; k=rsa; p=%s",
		string(dk.PublicKey),
	)
}
