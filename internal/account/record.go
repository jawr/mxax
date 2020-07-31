package account

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
)

type Record struct {
	ID int

	AccountID int
	DomainID  int
	Host      string
	Rtype     string
	Value     string

	MetaData
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

func GetRecords(ctx context.Context, db pgx.Tx, records *[]Record, domainID int) error {
	return pgxscan.Select(
		ctx,
		db,
		records,
		`
			SELECT * 
			FROM records
			WHERE 
				domain_id = $1 
				AND deleted_at IS NULL
			ORDER BY rtype, host, id
			`,
		domainID,
	)
}
