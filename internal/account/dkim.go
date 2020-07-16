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

func chunkString(s string, chunkSize int) []string {
	var chunks []string
	runes := []rune(s)

	if len(runes) == 0 {
		return []string{s}
	}

	for i := 0; i < len(runes); i += chunkSize {
		nn := i + chunkSize
		if nn > len(runes) {
			nn = len(runes)
		}
		chunks = append(chunks, string(runes[i:nn]))
	}
	return chunks
}

func (dk DkimKey) String() string {
	parts := chunkString(string(dk.PublicKey), 100)

	if len(parts) == 0 {
		return ""
	}

	s := fmt.Sprintf(`"v=DKIM1; k=rsa; p=%s"`, parts[0])

	for i := 1; i < len(parts); i++ {
		s += fmt.Sprintf(` "%s"`, parts[i])
	}

	return s
}
