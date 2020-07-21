package smtp

import (
	"bytes"
	"crypto"
	"crypto/tls"
	"fmt"
	"net"
	stdsmtp "net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

// On a successful relay, pass to the handler
type RelayHandler func(session *InboundSession) error

// create an inbound handler that handles the DATA hok
func MakeRelayHandler(db *pgx.Conn) (RelayHandler, error) {
	// create various caches
	destinationMXsCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

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

		receivedHeader := fmt.Sprintf(
			"Recived: from %s (%s [%s]) by %s with %s;%s\r\n\t%s\r\n",
			session.State.Hostname,
			rdns,
			remoteIP,
			session.ServerName,
			"ESMTP",
			tlsInfo,
			time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700 (MST)"),
		)

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

		err := db.Exec(
			"INSERT INTO return_paths (id, alias_id) VALUES ($1, $2)",
			session.ID,
			session.AliasID,
		)
		if err != nil {
			return errors.WithMessage(err, "Insert ReturnPath")
		}

		// write the received header to the buffer
		final := pool.Get().(*bytes.Buffer)
		final.Reset()
		defer pool.Put(final)

		// write received header
		if _, err := final.WriteString(receivedHeader); err != nil {
			return errors.WithMessage(err, "WriteString receivedHeader")
		}

		// write return path
		if _, err := final.WriteString(returnPathHeader); err != nil {
			return errors.WithMessage(err, "WriteString receivedHeader")
		}

		// write the actual message
		if _, err := final.ReadFrom(&session.Message); err != nil {
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

		destinations, err := getDestinations(db, destinationCache, session.AliasID)
		if err != nil {
			return errors.WithMessage(err, "getDestinations")
		}

		if len(destinations) == 0 {
			return errors.Errorf("no destinations found for alias %d", session.AliasID)
		}

		for _, destination := range destinations {

			break

			parts := strings.Split(destination.Address, "@")
			if len(parts) != 2 {
				return errors.Errorf("bad destination: '%s'", destination.Address)
			}

			destinationMXs, err := getDestinationMXs(destinationMXsCache, parts[1])
			if err != nil {
				return errors.WithMessagef(err, "getDestinationMXs for %d '%s'", destination.ID, parts[1])
			}

			// TODO
			// try until we hit an mx successfully
			var dialErr error
			for _, mx := range destinationMXs {
				// reset err, if we hit a dial error, iterate to the next
				dialErr = nil
				client, dialErr := stdsmtp.Dial(mx.Host + ":25")
				if dialErr != nil {
					dialErr = errors.WithMessagef(err, "Dial %d: '%s'", destination.ID, mx.Host)
					continue
				}

				if err := client.Hello(os.Getenv("MXAX_DOMAIN")); err != nil {
					return errors.WithMessage(err, "Hello")
				}

				tlsConfig := &tls.Config{
					ServerName: mx.Host,
				}

				if ok, _ := client.Extension("STARTTLS"); ok {
					if err := client.StartTLS(tlsConfig); err != nil {
						return errors.WithMessage(err, "StartTLS")
					}
				}

				if err := client.Mail(session.To); err != nil {
					return errors.WithMessage(err, "Mail")
				}

				if err := client.Rcpt(destination.Address); err != nil {
					return errors.WithMessage(err, "Rcpt")
				}

				wc, err := client.Data()
				if err != nil {
					return errors.WithMessage(err, "Data")
				}

				if _, err := b.WriteTo(wc); err != nil {
					return errors.WithMessage(err, "WriteTo")
				}

				if err := wc.Close(); err != nil {
					return errors.WithMessage(err, "Close")
				}

				if err := client.Quit(); err != nil {
					return errors.WithMessage(err, "Quit")
				}

				break
			}

			// check for any dial errors
			if dialErr != nil {
				return err
			}
		}

		return nil
	}, nil
}
