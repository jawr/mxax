package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log"

	"fmt"
	"net"
	"strings"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jawr/mxax/internal/account"
	"github.com/jhillyerd/enmime"
	"github.com/pkg/errors"
)

// map for selecting tls version
var tlsVersions = map[uint16]string{
	tls.VersionSSL30: "SSL3.0",
	tls.VersionTLS10: "TLS1.0",
	tls.VersionTLS11: "TLS1.1",
	tls.VersionTLS12: "TLS1.2",
	tls.VersionTLS13: "TLS1.3",
}

// create an inbound handler that handles the DATA hok
func (s *Server) relay(session *SessionData) error {
	remoteAddr, ok := session.State.RemoteAddr.(*net.TCPAddr)
	if !ok {
		return errors.New("execpted *net.TCPAddr")
	}
	remoteIP := remoteAddr.IP.String()

	rdns, err := s.getRDNS(remoteIP)
	if err != nil {
		return errors.WithMessage(err, "getRDNS")
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

	returnPath, err := s.makeReturnPath(session)
	if err != nil {
		return errors.WithMessage(err, "makeReturnPath")
	}

	returnPathHeader := fmt.Sprintf(
		"Return-Path: <%s>\r\n",
		returnPath,
	)

	// get alias' destinations to forward on to
	destinations, err := s.getDestinations(session.Alias.ID)
	if err != nil {
		return errors.WithMessage(err, "getDestinations")
	}

	if len(destinations) == 0 {
		return errors.Errorf("no destinations found for alias %d", session.Alias.ID)
	}

	// create a reader
	message := bytes.NewReader(session.Message.Bytes())

	// read envelope to extract the from header
	env, err := enmime.ReadEnvelope(message)
	if err != nil {
		return errors.WithMessage(err, "unable to read envelope")
	}

	fromList, err := env.AddressList("From")
	if err != nil {
		return errors.WithMessage(err, "AddressList")
	}

	if len(fromList) == 0 {
		return errors.New("no from address found")
	}

	// rewrite the session From as it is stored in return_paths
	session.From = fromList[0].Address

	for _, destination := range destinations {
		receivedHeader := fmt.Sprintf(
			"Received: from %s (%s [%s]) by %s with %s id %s for <%s>;%s\r\n\t%s\r\n",
			session.State.Hostname,
			rdns,
			remoteIP,
			session.ServerName,
			"ESMTP",
			session.ID.String(),
			destination.Address,
			tlsInfo,
			time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700 (MST)"),
		)

		// rewind the io.Reader
		if _, err := message.Seek(0, io.SeekStart); err != nil {
			return errors.WithMessage(err, "unable to seek message")
		}

		log.Printf("RLY - %s - Send to %d '%s'", session.ID, destination.ID, destination.Address)

		final := s.bufferPool.Get().(*bytes.Buffer)
		final.Reset()
		defer s.bufferPool.Put(final)

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

		signed := s.bufferPool.Get().(*bytes.Buffer)
		signed.Reset()
		defer s.bufferPool.Put(signed)

		if err := s.dkimSignHandler(session, final, signed); err != nil {
			return errors.WithMessage(err, "dkimSignHandler")
		}

		err = session.server.queueEmail(Email{
			ID:            session.ID,
			ReturnPath:    returnPath,
			From:          session.From,
			Via:           session.To,
			To:            destination.Address,
			Message:       signed.Bytes(),
			AccountID:     session.Domain.AccountID,
			DomainID:      session.Domain.ID,
			AliasID:       session.Alias.ID,
			DestinationID: destination.ID,
		})
		if err != nil {
			return errors.Wrap(err, "queueEmail")
		}

	}

	return nil
}

func (s *Server) getRDNS(ip string) (string, error) {
	if v, ok := s.cache.Get("rdns", ip); ok {
		return v.(string), nil
	}

	var rdns string
	addr, err := net.LookupAddr(ip)
	if err != nil {
		if !strings.Contains(err.Error(), "no such host") {
			return "", errors.WithMessagef(err, "LookupAddr '%s'", ip)
		}

		addressSlice := strings.Split(ip, ".")
		reverseSlice := []string{}

		for i := range addressSlice {
			octet := addressSlice[len(addressSlice)-1-i]
			reverseSlice = append(reverseSlice, octet)
		}

		rdns = strings.Join(reverseSlice, ".") + ".in-addr.arpa"
	}

	if len(addr) > 0 {
		rdns = strings.Trim(addr[0], ".")
	}

	s.cache.Set("rdns", ip, rdns)

	return rdns, nil
}

func (s *Server) getDestinations(aliasID int) ([]account.Destination, error) {
	if destinations, ok := s.cache.Get("destinations", fmt.Sprintf("%d", aliasID)); ok {
		return destinations.([]account.Destination), nil
	}

	var destinations []account.Destination
	err := pgxscan.Select(
		context.Background(),
		s.db,
		&destinations,
		`
		SELECT d.* 
		FROM destinations AS d 
		JOIN alias_destinations AS ad ON d.id = ad.destination_id 
		WHERE ad.alias_id = $1
		AND ad.deleted_at IS NULL
		AND d.deleted_at IS NULL
		`,
		aliasID,
	)
	if err != nil {
		return nil, err
	}

	s.cache.Set("destinations", fmt.Sprintf("%d", aliasID), destinations)

	return destinations, nil
}
