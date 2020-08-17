package controlpanel

import (
	"context"

	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
)

func getAccounType(ctx context.Context) account.AccountType {
	at, ok := ctx.Value("account_type").(account.AccountType)
	if !ok {
		return account.AccountTypeFree
	}
	return at
}

func (s *Site) aclDomainCreateCheck(ctx context.Context, tx pgx.Tx) (bool, error) {
	accountType := getAccounType(ctx)

	if accountType == account.AccountTypeSubscription {
		return true, nil
	}

	var count int
	err := tx.QueryRow(
		ctx,
		`
		SELECT COUNT(*) FROM domains
		WHERE deleted_at IS NULL
		`,
	).Scan(&count)
	if err != nil {
		return false, err
	}

	if count > 0 {
		return false, nil
	}

	return true, nil
}

func (s *Site) aclDestinationCreateCheck(ctx context.Context, tx pgx.Tx) (bool, error) {
	accountType := getAccounType(ctx)

	switch accountType {
	case account.AccountTypeSubscription:
		return true, nil
	}

	var count int
	err := tx.QueryRow(
		ctx,
		`
		SELECT COUNT(*) FROM destinations
		WHERE deleted_at IS NULL
		`,
	).Scan(&count)
	if err != nil {
		return false, err
	}

	if count > 0 {
		return false, nil
	}

	return true, nil
}

func (s *Site) aclAliasCreateCheck(ctx context.Context, tx pgx.Tx) (bool, error) {
	accountType := getAccounType(ctx)

	switch accountType {
	case account.AccountTypeSubscription:
		return true, nil
	}

	var count int
	err := tx.QueryRow(
		ctx,
		`
		SELECT COUNT(*) FROM aliases
		WHERE deleted_at IS NULL
		`,
	).Scan(&count)
	if err != nil {
		return false, err
	}

	if count > 0 {
		return false, nil
	}

	return true, nil
}
