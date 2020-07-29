package account

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
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

func (d Domain) BuildVerifyRecord() string {
	return fmt.Sprintf(
		"%s 10800 IN TXT %s.mx.ax.",
		d.VerifyCode,
		d.VerifyCode,
	)
}

func GetDomain(ctx context.Context, db *pgx.Conn, domain *Domain, accountID int, name string) error {
	return pgxscan.Get(
		ctx,
		db,
		domain,
		`
			SELECT * 
			FROM domains 
			WHERE 
				account_id = $1 
				AND name = $2
				AND deleted_at IS NULL
			`,
		accountID,
		name,
	)
}

func GetDomainByID(ctx context.Context, db *pgx.Conn, domain *Domain, accountID, domainID int) error {
	return pgxscan.Get(
		ctx,
		db,
		domain,
		`
			SELECT * 
			FROM domains 
			WHERE 
				account_id = $1 
				AND id = $2
				AND deleted_at IS NULL
			`,
		accountID,
		domainID,
	)
}

func GetDomains(ctx context.Context, db *pgx.Conn, domains *[]Domain, accountID int) error {
	return pgxscan.Select(
		ctx,
		db,
		domains,
		`
			SELECT * 
			FROM domains 
			WHERE 
				account_id = $1 
				AND deleted_at IS NULL
			`,
		accountID,
	)
}

func DeleteDomain(ctx context.Context, db *pgx.Conn, domainID, accountID int) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return errors.WithMessage(err, "Begin")
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		`
		UPDATE records
			SET deleted_at = NOW()
		WHERE domain_id = $1
		`,
		domainID,
	)
	if err != nil {
		return errors.WithMessage(err, "UPDATE records")
	}

	_, err = tx.Exec(
		ctx,
		`
		UPDATE alias_destinations 
			SET deleted_at = NOW()
		WHERE alias_id IN (
			SELECT id FROM aliases WHERE domain_id = $1
		)
		`,
		domainID,
	)
	if err != nil {
		return errors.WithMessage(err, "UPDATE alias_destinations")
	}

	_, err = tx.Exec(
		ctx,
		"UPDATE aliases SET deleted_at = NOW() WHERE domain_id = $1",
		domainID,
	)
	if err != nil {
		return errors.WithMessage(err, "UPDATE aliases")
	}

	_, err = tx.Exec(
		ctx,
		"UPDATE domains SET deleted_at = NOW() WHERE id = $1",
		domainID,
	)
	if err != nil {
		return errors.WithMessage(err, "UPDATE domains")
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func CreateDomain(ctx context.Context, db *pgx.Conn, name string, accountID int, expiresAt time.Time) error {
	// create a code
	var verifyCode string
	var tries int
	for {
		if tries > 10 {
			return errors.New("Too many tries creating a verify code. Please contact support.")
		}

		n := 11
		b := make([]byte, n)
		if _, err := rand.Read(b); err != nil {
			return errors.WithMessage(err, "rand.Read")
		}

		verifyCode = fmt.Sprintf("mxax-%X", b)

		var count int
		err := db.QueryRow(
			ctx,
			"SELECT COUNT(*) FROM domains WHERE verify_code = $1",
			verifyCode,
		).Scan(&count)
		if err != nil {
			return errors.WithMessage(err, "Select VerifyCode count")
		}

		if count == 0 {
			break
		}
	}

	// insert, first create a transaction so we keep a clean state on
	// an error
	tx, err := db.Begin(ctx)
	if err != nil {
		return errors.WithMessage(err, "Begin")
	}
	defer tx.Rollback(ctx)

	var id int
	err = tx.QueryRow(
		ctx,
		`
		WITH e AS (
		INSERT INTO domains (account_id, name, verify_code, expires_at) 
			VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO 
			UPDATE deleted_at = null 
			WHERE
				account_id = EXCLUDED.account_id
		RETURNING id
		)
		SELECT * FROM e UNION SELECT id FROM domains WHERE name = $2 AND account_id = $1
		`,
		accountID,
		name,
		verifyCode,
		expiresAt,
	).Scan(&id)
	if err != nil {
		return errors.WithMessage(err, "Insert")
	}

	// create dkim record
	dkimKey, err := NewDkimKey(id)
	if err != nil {
		return errors.WithMessage(err, "NewDkimKey")
	}

	// insert dkim
	_, err = tx.Exec(
		ctx,
		"INSERT INTO dkim_keys (domain_id, private_key, public_key) VALUES ($1, $2, $3)",
		id,
		dkimKey.PrivateKey,
		dkimKey.PublicKey,
	)
	if err != nil {
		return errors.WithMessage(err, "Insert DkimKey")
	}

	// insert dkim record
	_, err = tx.Exec(
		ctx,
		"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
		id,
		"mxax._domainkey",
		"TXT",
		dkimKey.String(),
	)
	if err != nil {
		return errors.WithMessage(err, "Insert DkimKey Record")
	}

	// insert mx
	_, err = tx.Exec(
		ctx,
		"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
		id,
		"@",
		"MX",
		"10 ehlo.mx.ax.",
	)
	if err != nil {
		return errors.WithMessage(err, "Insert MX Record")
	}

	_, err = tx.Exec(
		ctx,
		"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
		id,
		"@",
		"MX",
		"20 helo.mx.ax.",
	)
	if err != nil {
		return errors.WithMessage(err, "Insert MX Record")
	}

	// TODO
	// host a second mx for redundancy

	// insert spf
	_, err = tx.Exec(
		ctx,
		"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
		id,
		"@",
		"TXT",
		`"v=spf1 include:spf.mx.ax ~all"`,
	)
	if err != nil {
		return errors.WithMessage(err, "Insert SPF Record")
	}

	// insert dmarc
	_, err = tx.Exec(
		ctx,
		"INSERT INTO records (domain_id, host, rtype, value) VALUES ($1, $2, $3, $4)",
		id,
		"_dmarc",
		"TXT",
		`"v=DMARC1; p=quarantine"`,
	)
	if err != nil {
		return errors.WithMessage(err, "Insert DkimKey Record")
	}

	if err := tx.Commit(ctx); err != nil {
		return errors.WithMessage(err, "Commit")
	}

	return nil
}
