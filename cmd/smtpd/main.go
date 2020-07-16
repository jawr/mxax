package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"math/rand"
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
	tlsConfig := &tls.Config{}

	return func(session *smtp.InboundSession) error {

		receivedHeader := fmt.Sprintf(
			"Recived: from %s ([%s]) by %s with %s;%s\r\n\t%s\r\n",
			session.State.Hostname,
			session.State.RemoteAddr,
			session.ServerName,
			// tls info
			// https://github.com/decke/smtprelay/blob/master/main.go
			"",
			"smtp",
		)

		messageID := fmt.Sprintf("<%s@pageup.me>", randSeq(12))
		boundaryID := randSeq(12)
		subject := "Test forward"

		/*
			MIME-Version: 1.0
			Date: Thu, 16 Jul 2020 14:23:37 +0900
			References: <ada296ac-0386-41d8-b1e7-5e46d1115c5e@xtgap4s7mta4152.xt.local>
			In-Reply-To: <ada296ac-0386-41d8-b1e7-5e46d1115c5e@xtgap4s7mta4152.xt.local>
			Message-ID: <CAMjuidsKex495ZucSoRvvMabAW7f4xVWNgJRgYhFcw-MvFDb_Q@mail.gmail.com>
			Subject: Fwd: Crunch - Payment Deadline Approaching
			From: Jess Lawrence <jess@lawrence.pm>
			To: jess lawrence <jess@lawrence.pm>
			Content-Type: multipart/alternative; boundary="000000000000d4b0e705aa883abd"

			--000000000000d4b0e705aa883abd
			Content-Type: text/plain; charset="UTF-8"
			Content-Transfer-Encoding: quoted-printable

			---------- Forwarded message ---------
		*/
		header := fmt.Sprintf(
			`MIME-Version: 1.0
Date: %s
Message-ID: <%s>
Subject: %s
From: Hi <hi@pageup.me>
To: <jessjlawrence@gmail.com>
Content-Type: multipart/alternative; boundary="%s"

--%s
Content-Type: text/plain; charset="UTF-8"
Content-Transfer-Encoding: quoted-printable
---------- Forwarded message ---------
%s
`,
			time.Now().Format(time.RFC1123Z),
			messageID,
			subject,
			boundaryID,
			boundaryID,
			receivedHeader,
		)

		final := bytes.NewBufferString(header)

		if _, err := final.ReadFrom(&session.Message); err != nil {
			return err
		}

		client, err := stdsmtp.Dial("gmail-smtp-in.l.google.com:25")
		if err != nil {
			return err
		}

		if err := client.Hello("pageup.me"); err != nil {
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
