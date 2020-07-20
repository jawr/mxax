package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	stdsmtp "net/smtp"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jess/mxax/internal/account"
	"github.com/jess/mxax/internal/smtp"
	"github.com/pkg/errors"
)

func getDestinationMXs(cache *ristretto.Cache, domain string) ([]*net.MX, error) {
	if mxs, ok := cache.Get(domain); ok {
		return mxs.([]*net.MX), nil
	}

	mxs, err := net.LookupMX(domain)
	if err != nil {
		return nil, errors.WithMessage(err, "LookupMX")
	}

	if len(mxs) == 0 {
		return nil, errors.Errorf("Found no MX domains for %s", domain)
	}

	sort.Slice(mxs, func(i, j int) bool {
		return mxs[i].Pref < mxs[j].Pref
	})

	return mxs, nil
}

func getDestinations(db *pgx.Conn, cache *ristretto.Cache, aliasID int) ([]account.Destination, error) {
	if destinations, ok := cache.Get(aliasID); ok {
		return destinations.([]account.Destination), nil
	}

	var destinations []account.Destination
	err := pgxscan.Select(
		context.Background(),
		db,
		destinations,
		"SELECT d.* FROM destinations AS d JOIN alias_destinations AS ad ON d.id = ad.destination_id WHERE ad.alias_id = $1",
		aliasID,
	)
	if err != nil {
		return nil, err
	}

	cache.SetWithTTL(aliasID, destinations, 1, time.Hour*24)

	return destinations, nil
}

func getDomain(db *pgx.Conn, cache *ristretto.Cache, aliasID int) (*account.Domain, error) {
	if domain, ok := cache.Get(aliasID); ok {
		return domain.(*account.Domain), nil
	}

	var domain *account.Domain
	err := pgxscan.Get(
		context.Background(),
		db,
		domain,
		"SELECT d.* FROM domains AS d JOIN aliases AS a ON d.id = a.domain_id WHERE a.id = $1",
		aliasID,
	)
	if err != nil {
		return nil, err
	}

	cache.SetWithTTL(aliasID, domain, 1, time.Hour*24)

	return domain, nil
}

func getDkimPrivateKey(db *pgx.Conn, cache *ristretto.Cache, aliasID int) (*rsa.PrivateKey, error) {
	if key, ok := cache.Get(aliasID); ok {
		return key.(*rsa.PrivateKey), nil
	}

	var privateKey []byte
	err := db.QueryRow(
		context.Background(),
		"SELECT private_key FROM dkim_keys WHERE domain_id = $1",
		aliasID,
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

	cache.SetWithTTL(aliasID, key, 1, time.Hour*24)

	return key, nil
}

func makeRelayHandler(db *pgx.Conn) (smtp.RelayHandler, error) {
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

	tlsConfig := &tls.Config{}

	tlsVersions := map[uint16]string{
		tls.VersionSSL30: "SSL3.0",
		tls.VersionTLS10: "TLS1.0",
		tls.VersionTLS11: "TLS1.1",
		tls.VersionTLS12: "TLS1.2",
		tls.VersionTLS13: "TLS1.3",
	}

	// setup a pool for bytes
	pool := sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	return func(session *smtp.InboundSession) error {

		// create a received header
		remoteAddr, ok := session.State.RemoteAddr.(*net.TCPAddr)
		if !ok {
			return errors.New("Execpted TCPAddr")
		}
		remoteIP := remoteAddr.IP.String()

		var rdns string
		addr, err := net.LookupAddr(remoteIP)
		if err != nil {
			return errors.WithMessage(err, "LookupAddr")
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

		// write the received header to the buffer
		final := pool.Get().(*bytes.Buffer)
		final.Reset()
		defer pool.Put(final)

		if _, err := final.WriteString(receivedHeader); err != nil {
			return errors.WithMessage(err, "Write receivedHeader")
		}

		if _, err := final.ReadFrom(&session.Message); err != nil {
			return errors.WithMessage(err, "Read Message")
		}

		// get domain
		domain, err := getDomain(db, domainCache, session.AliasID)
		if err != nil {
			return errors.WithMessage(err, "getDomain")
		}

		// add dkim
		key, err := getDkimPrivateKey(db, dkimKeyCache, session.AliasID)
		if err != nil {
			return errors.WithMessage(err, "getDkimPrivateKey")
		}

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
			return errors.Wrap(err, "Sign")
		}

		destinations, err := getDestinations(db, destinationCache, session.AliasID)
		if err != nil {
			return errors.WithMessage(err, "getDestinations")
		}

		if len(destinations) == 0 {
			return errors.Errorf("No destinations found for alias %d", session.AliasID)
		}

		for _, destination := range destinations {
			parts := strings.Split(destination.Address, "@")
			if len(parts) != 2 {
				return errors.Errorf("Bad destination Address: %s", destination.Address)
			}

			destinationMXs, err := getDestinationMXs(destinationMXsCache, parts[1])
			if err != nil {
				return errors.WithMessage(err, "getDestinationMXs")
			}

			// TODO
			// try until we hit an mx successfully
			var dialErr error
			for _, mx := range destinationMXs {
				// reset err, if we hit a dial error, iterate to the next
				dialErr = nil
				client, dialErr := stdsmtp.Dial(mx.Host + ":25")
				if dialErr != nil {
					dialErr = errors.WithMessagef(err, "Dial: %s", mx.Host)
					continue
				}

				if err := client.Hello(os.Getenv("MXAX_DOMAIN")); err != nil {
					return errors.WithMessage(err, "Hello")
				}

				if err := client.StartTLS(tlsConfig); err != nil {
					return errors.WithMessage(err, "StartTLS")
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
				return errors.WithMessage(err, "Dial")
			}
		}

		return nil
	}, nil
}
