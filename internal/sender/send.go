package sender

import (
	"crypto/tls"
	"log"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/jawr/mxax/internal/smtp"
	smtpclient "github.com/jawr/mxax/internal/smtp/client"
	"github.com/pkg/errors"
)

const SEND_DEADLINE = time.Second * 60

func (s *Sender) sendEmail(rdns string, dialer net.Dialer, email *smtp.Email) (string, error) {
	parts := strings.Split(email.To, "@")
	if len(parts) != 2 {
		return "", errors.Errorf("bad destination: '%s'", email.To)
	}

	destinationMXs, err := s.getDestinationMXs(parts[1])
	if err != nil {
		return "", errors.WithMessagef(err, "getDestinationMXs for '%s'", parts[1])
	}

	if len(destinationMXs) == 0 {
		return "", errors.Errorf("found no ddestination mxs for '%s'", parts[1])
	}

	// TODO
	// try until we hit an mx successfully
	var dialErr error
	for _, mx := range destinationMXs {
		log.Printf("Checking %s", mx.Host)

		// reset err, if we hit a dial error, iterate to the next
		dialErr = nil
		conn, err := dialer.Dial("tcp", mx.Host+":25")
		if err != nil {
			log.Printf("Unable to dial '%s': %s", mx.Host, err)
			dialErr = errors.WithMessagef(err, "dial '%s'", mx.Host)
			continue
		}

		if err := conn.SetDeadline(time.Now().Add(SEND_DEADLINE)); err != nil {
			return "", errors.WithMessagef(err, "setDeadline: '%s'", mx.Host)
		}

		client, err := smtpclient.NewClient(conn, mx.Host)
		if err != nil {
			return "", errors.WithMessagef(err, "newclient: '%s'", mx.Host)
		}

		if err := client.Hello(rdns); err != nil {
			return "", errors.WithMessage(err, "Hello")
		}

		tlsConfig := &tls.Config{
			ServerName: mx.Host,
		}

		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return "", errors.WithMessage(err, "StartTLS")
			}
		}

		returnPath := email.ReturnPath
		if len(returnPath) == 0 {
			returnPath = email.From
		}

		if err := client.Mail(returnPath); err != nil {
			return "", errors.WithMessage(err, "Mail")
		}

		if err := client.Rcpt(email.To); err != nil {
			return "", errors.WithMessage(err, "Rcpt")
		}

		wc, err := client.Data()
		if err != nil {
			return "", errors.WithMessage(err, "Data")
		}

		if _, err := wc.Write(email.Message); err != nil {
			return "", errors.WithMessage(err, "Write")
		}

		_, reply, err := wc.Close()
		if err != nil {
			return "", err
		}

		if err := client.Quit(); err != nil {
			return "", errors.WithMessage(err, "Quit")
		}

		return reply, nil
	}

	// check for any dial errors
	if dialErr != nil {
		return "", err
	}

	return "", errors.New("should never get here")
}

func (s *Sender) getDestinationMXs(domain string) ([]*net.MX, error) {
	if mxs, ok := s.cache.Get("mx", domain); ok {
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

	s.cache.Set("mx", domain, mxs)

	return mxs, nil
}
