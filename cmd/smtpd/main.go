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
	"log"
	"math/rand"
	"net"
	stdsmtp "net/smtp"
	"os"
	"strings"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jess/mxax/internal/account"
	"github.com/jess/mxax/internal/smtp"
	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	db, err := pgx.Connect(ctx, os.Getenv("MXAX_DATABASE_URL"))
	if err != nil {
		return errors.WithMessage(err, "pgx.Connect")
	}
	defer db.Close(ctx)

	log.Println("DB Connected")

	server := smtp.NewServer(makeAliasHandler(db), makeRelayHandler(db))

	log.Println("Starting SMTP Server...")

	if err := server.Run(os.Getenv("MXAX_DOMAIN")); err != nil {
		return errors.WithMessage(err, "server.Run")
	}

	return nil
}

func makeAliasHandler(db *pgx.Conn) smtp.AliasHandler {
	// replace with real concurrent safe caches that allows
	// for ttl

	nxdomain := make(map[string]struct{}, 0)
	nxmatch := make(map[string]struct{}, 0)
	matches := make(map[string]int, 0)
	aliases := make(map[string][]account.Alias, 0)

	return func(ctx context.Context, email string) (int, error) {
		if _, ok := nxmatch[email]; ok {
			return 0, errors.New("No match")
		}

		if aliasID, ok := matches[email]; ok {
			return aliasID, nil
		}

		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			return 0, errors.New("Malformed email address")
		}

		user := parts[0]
		domain := parts[1]

		// check if this is a bad domain we have checked already
		if _, ok := nxdomain[domain]; ok {
			return 0, errors.New("Domain not accepted")
		}

		// search for domain in the database
		all, ok := aliases[domain]
		if !ok {
			if err := pgxscan.Select(ctx, db, &all, "SELECT a.* FROM aliases AS a JOIN domains AS d ON a.domain_id = d.id WHERE d.name = $1 AND d.deleted_at IS NULL AND d.verified_at IS NOT NULL", domain); err != nil {
				nxdomain[domain] = struct{}{}

				return 0, errors.New("Domain not accepted")
			}
			aliases[domain] = all
		}

		// check for matches
		for _, i := range all {
			ok, err := i.Check(user)
			if err != nil {
				continue
			}
			if ok {
				matches[email] = i.ID
				return i.ID, nil
			}
		}

		// no matches found, update nxmatch and return
		nxmatch[email] = struct{}{}

		return 0, errors.New("No match")
	}
}

func makeRelayHandler(db *pgx.Conn) smtp.RelayHandler {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	tlsVersions := map[uint16]string{
		tls.VersionSSL30: "SSL3.0",
		tls.VersionTLS10: "TLS1.0",
		tls.VersionTLS11: "TLS1.1",
		tls.VersionTLS12: "TLS1.2",
		tls.VersionTLS13: "TLS1.3",
	}

	var privateKey []byte
	if err := db.QueryRow("SELECT private_key FROM dkim_keys").Scan(&privateKey); err != nil {
		panic(err)
	}

	d, _ := pem.Decode(privateKey)
	if d == nil {
		panic(errors.New("pem.Decode"))
	}

	key, err := x509.ParsePKCS1PrivateKey(d.Bytes)
	if err != nil {
		panic(errors.Wrap(err, "x509.ParsePKCS1PrivateKey"))
	}

	var rsaKey *rsa.PrivateKey
	rsaKey = key

	return func(session *smtp.InboundSession) error {
		remoteAddr, ok := session.State.RemoteAddr.(*net.TCPAddr)
		if !ok {
			return errors.New("Execpted TCPAddr")
		}
		remoteIP := remoteAddr.IP.String()

		var rdns string
		addr, err := net.LookupAddr(remoteIP)
		if err != nil {
			return err
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

		final := bytes.NewBufferString(receivedHeader)

		if _, err := final.ReadFrom(&session.Message); err != nil {
			return err
		}

		// add dkim

		opts := dkim.SignOptions{
			Domain:   "pageup.me",
			Selector: "default",
			Signer:   rsaKey,
			Hash:     crypto.SHA256,
		}

		if err := dkim.Sign(b, final, &opts); err != nil {
			return errors.Wrap(err, "Sign")
		}

		log.Printf("MESSAGE:\n%s", string(final.Bytes()))

		client, err := stdsmtp.Dial("gmail-smtp-in.l.google.com:25")
		if err != nil {
			return err
		}

		if err := client.Hello("pageup.uk"); err != nil {
			return err
		}

		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}

		if err := client.Mail("hi@pageup.me"); err != nil {
			return err
		}

		if err := client.Rcpt("jessjlawrence@gmail.com"); err != nil {
			return err
		}

		wc, err := client.Data()
		if err != nil {
			return err
		}

		if _, err := final.WriteTo(wc); err != nil {
			return err
		}

		if err := wc.Close(); err != nil {
			return err
		}

		if err := client.Quit(); err != nil {
			return err
		}

		return nil
	}
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
