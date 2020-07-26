package account

import (
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgtype"
	whoisParser "github.com/jawr/whois-parser-go"
	"github.com/likexian/whois-go"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
)

// Domain is attached to an Account and must
// be verified before being used. Alias' are
// attached to the Domain for forwarding
type Domain struct {
	ID int

	AccountID int

	Name string

	// Verification
	VerifyCode string
	VerifiedAt pgtype.Timestamp

	// when the domain expires
	ExpiresAt pgtype.Date

	MetaData
}

type Record struct {
	ID             int
	DomainID       int
	Host           string
	Rtype          string
	Value          string
	LastVerifiedAt pgtype.Timestamp
}

func (rr Record) IsComplete() bool {
	return !rr.LastVerifiedAt.Time.IsZero() && time.Since(rr.LastVerifiedAt.Time) > time.Hour*24

}

func (r Record) String() string {
	return fmt.Sprintf(
		"%s 10800 IN %s %s",
		r.Host,
		r.Rtype,
		r.Value,
	)
}

func (r Record) Check(domain string, config *dns.ClientConfig) error {
	client := new(dns.Client)

	record := fmt.Sprintf(
		"%s.%s.",
		r.Host,
		domain,
	)

	record = strings.TrimPrefix(record, "@.")

	dnsType, ok := dns.StringToType[r.Rtype]
	if !ok {
		return errors.Errorf("unknown record type '%s'", r.Rtype)
	}

	m := new(dns.Msg)
	m.SetQuestion(record, dnsType)
	m.RecursionDesired = true

	if len(config.Servers) == 0 {
		return errors.New("no dns servers found.")
	}

	resp, _, err := client.Exchange(m, config.Servers[0]+":"+config.Port)
	if err != nil {
		return errors.WithMessage(err, "Exchange")
	}

	if len(resp.Answer) == 0 {
		return errors.New("No record found.")
	}

	if len(resp.Answer) > 1 {
		return errors.New("Too many records found.")
	}

	for _, a := range resp.Answer {
		switch r.Rtype {
		case "MX":
			found := fmt.Sprintf("%d %s", a.(*dns.MX).Preference, a.(*dns.MX).Mx)

			if found == r.Value {
				return nil
			}

		case "TXT":
			txt := strings.Join(a.(*dns.TXT).Txt, "")
			found := fmt.Sprintf(`"%s"`, txt)

			found = strings.Replace(found, `" "`, "", -1)
			found = strings.Trim(found, `"`)

			against := strings.Replace(r.Value, `" "`, "", -1)
			against = strings.Trim(against, `"`)

			if found == against {
				return nil
			}
		}
	}

	return errors.New("No match")
}

func GetDomainExpirationDate(name string) (time.Time, error) {
	whoisResult, err := whois.Whois(name)
	if err != nil {
		return time.Time{}, err
	}

	// parse and extract expiresAt
	whoisParsed, err := whoisParser.Parse(whoisResult)
	if err != nil {
		return time.Time{}, err
	}

	return whoisParsed.Domain.ExpirationDate, nil
}

func (d Domain) CheckVerifyCode(config *dns.ClientConfig) error {
	client := new(dns.Client)

	expected := fmt.Sprintf(
		"%s.%s",
		d.VerifyCode,
		d.Name,
	)

	m := new(dns.Msg)
	m.SetQuestion(expected+".", dns.TypeCNAME)
	m.RecursionDesired = true

	if len(config.Servers) == 0 {
		return errors.New("no dns servers found.")
	}

	r, _, err := client.Exchange(m, config.Servers[0]+":"+config.Port)
	if err != nil {
		return errors.WithMessage(err, "Exchange")
	}

	if len(r.Answer) == 0 {
		return errors.New("No record found.")
	}

	if len(r.Answer) > 1 {
		return errors.New("Too many records found.")
	}

	found := r.Answer[0].(*dns.CNAME).Target
	if strings.ToLower(found) != strings.ToLower(fmt.Sprintf("%s.mx.ax.", d.VerifyCode)) {
		return errors.New("Does not match")
	}

	return nil
}
