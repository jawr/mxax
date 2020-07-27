package smtp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/tls"
	"io"
	"log"

	// "io/ioutil"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

// On a successful forward, pass to the handler
type forwardHandlerFn func(session *InboundSession) error

// create an inbound handler that handles the DATA hok
func (s *Server) makeForwardHandler(db *pgx.Conn) (forwardHandlerFn, error) {
	// create various caches
	dkimKeyCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	destinationCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	domainCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	// map for selecting tls version
	tlsVersions := map[uint16]string{
		tls.VersionSSL30: "SSL3.0",
		tls.VersionTLS10: "TLS1.0",
		tls.VersionTLS11: "TLS1.1",
		tls.VersionTLS12: "TLS1.2",
		tls.VersionTLS13: "TLS1.3",
	}

	// setup a pool for bytes.Buffers
	pool := sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	return func(session *InboundSession) error {

		// create a received header
		remoteAddr, ok := session.State.RemoteAddr.(*net.TCPAddr)
		if !ok {
			return errors.New("execpted *net.TCPAddr")
		}
		remoteIP := remoteAddr.IP.String()

		var rdns string
		addr, err := net.LookupAddr(remoteIP)
		if err != nil {
			return errors.WithMessagef(err, "LookupAddr '%s'", remoteIP)
		}

		if len(addr) > 0 {
			rdns = strings.Trim(addr[0], ".")
		}

		var tlsInfo string
		if session.State.TLS.Version > 0 {
			version := "unknown"
			if val, ok := tlsVersions[session.State.TLS.Version]; ok {
				version = val
			}

			tlsInfo = fmt.Sprintf(
				"\r\n\t(version=%s cipher=%s);",
				version,
				tls.CipherSuiteName(session.State.TLS.CipherSuite),
			)
		}

		// get domain
		domain, err := getDomain(db, domainCache, session.AliasID)
		if err != nil {
			return errors.WithMessage(err, "getDomain")
		}

		// write return path
		returnPathHeader := fmt.Sprintf(
			"Return-Path: <%s@%s>\r\n",
			session.ID,
			domain.Name,
		)

		// TODO
		// at the moment we are using the from address as our eventual return path
		// we are also not stripping out any existing return-path headers; dp we want
		// to set the return to to from if we dont find (and strip) any return-path
		// header values
		// also how do we handle this when we have multiple addresses
		_, err = db.Exec(
			context.Background(),
			"INSERT INTO return_paths (id, alias_id, return_to) VALUES ($1, $2, $3)",
			session.ID,
			session.AliasID,
			session.From,
		)
		if err != nil {
			return errors.WithMessage(err, "Insert ReturnPath")
		}

		// get alias' destinations to forward on to
		destinations, err := getDestinations(db, destinationCache, session.AliasID)
		if err != nil {
			return errors.WithMessage(err, "getDestinations")
		}

		if len(destinations) == 0 {
			return errors.Errorf("no destinations found for alias %d", session.AliasID)
		}

		// create a reader
		message := bytes.NewReader(session.Message.Bytes())

		for _, destination := range destinations {
			if _, err := message.Seek(0, io.SeekStart); err != nil {
				return errors.WithMessage(err, "unable to seek message")
			}

			log.Printf("%s - Send to '%s'", session, destination.Address)

			// break

			receivedHeader := fmt.Sprintf(
				"Received: from %s (%s [%s]) by %s with %s id %s for <%s>;%s\r\n\t%s\r\n",
				session.State.Hostname,
				rdns,
				remoteIP,
				session.ServerName,
				"ESMTP",
				session.String(),
				destination.Address,
				tlsInfo,
				time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700 (MST)"),
			)

			// write the received header to the buffer
			final := pool.Get().(*bytes.Buffer)
			final.Reset()
			defer pool.Put(final)

			// write return path
			if _, err := final.WriteString(returnPathHeader); err != nil {
				return errors.WithMessage(err, "WriteString receivedHeader")
			}

			// write received header
			if _, err := final.WriteString(receivedHeader); err != nil {
				return errors.WithMessage(err, "WriteString receivedHeader")
			}

			// write the actual message
			if _, err := final.ReadFrom(message); err != nil {
				return errors.WithMessage(err, "ReadFrom Message")
			}

			// get dkim and proceed to sign
			key, err := getDkimPrivateKey(db, dkimKeyCache, domain.ID)
			if err != nil {
				return errors.WithMessage(err, "getDkimPrivateKey")
			}

			// sign the email
			opts := dkim.SignOptions{
				Domain:   domain.Name,
				Selector: "default",
				Signer:   key,
				Hash:     crypto.SHA256,
			}

			b := pool.Get().(*bytes.Buffer)
			b.Reset()
			defer pool.Put(b)

			if err := dkim.Sign(b, final, &opts); err != nil {
				return errors.Wrap(err, "dkim.Sign")
			}

			err = session.server.queueEmailHandler(Email{
				ID:      session.ID,
				From:    session.To,
				To:      destination.Address,
				Message: b.Bytes(),
			})
			if err != nil {
				return errors.Wrap(err, "queueEmailHandler")
			}

		}

		return nil
	}, nil
}
