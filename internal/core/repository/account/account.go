package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun"
	"github.com/uptrace/go-clickhouse/ch"

	"github.com/tonindexer/anton/addr"
	"github.com/tonindexer/anton/internal/core"
	"github.com/tonindexer/anton/internal/core/repository"
)

var _ repository.Account = (*Repository)(nil)

type Repository struct {
	ch *ch.DB
	pg *bun.DB
}

func NewRepository(ck *ch.DB, pg *bun.DB) *Repository {
	return &Repository{ch: ck, pg: pg}
}

func createIndexes(ctx context.Context, pgDB *bun.DB) error {
	// account data

	_, err := pgDB.NewCreateIndex().
		Model(&core.AccountState{}).
		Using("HASH").
		Column("owner_address").
		Where("owner_address IS NOT NULL").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address state owner pg create index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountState{}).
		Using("HASH").
		Column("minter_address").
		Where("minter_address IS NOT NULL").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address state minter pg create index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountState{}).
		Using("GIN").
		Column("types").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state contract types pg create index")
	}

	// account state

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountState{}).
		Unique().
		Column("address", "workchain", "shard", "block_seq_no").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address state address in block pg create unique index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountState{}).
		Using("HASH").
		Column("address").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address state address pg create index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountState{}).
		Using("BTREE").
		Column("last_tx_lt").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state last_tx_lt pg create index")
	}

	// latest account state

	_, err = pgDB.NewCreateIndex().
		Model(&core.LatestAccountState{}).
		Using("BTREE").
		Column("last_tx_lt").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "latest account state last_tx_lt pg create index")
	}

	return nil
}

func CreateTables(ctx context.Context, chDB *ch.DB, pgDB *bun.DB) error {
	_, err := pgDB.ExecContext(ctx, "CREATE TYPE account_status AS ENUM (?, ?, ?, ?)",
		core.Uninit, core.Active, core.Frozen, core.NonExist)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return errors.Wrap(err, "account status pg create enum")
	}

	_, err = pgDB.ExecContext(ctx, "CREATE TYPE label_category AS ENUM (?, ?)",
		core.CentralizedExchange, core.Scam)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return errors.Wrap(err, "address label category pg create enum")
	}

	_, err = chDB.NewCreateTable().
		IfNotExists().
		Engine("ReplacingMergeTree").
		Model(&core.AddressLabel{}).
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address label ch create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.AddressLabel{}).
		IfNotExists().
		WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address label pg create table")
	}

	_, err = chDB.NewCreateTable().
		IfNotExists().
		Engine("ReplacingMergeTree").
		Model(&core.AccountState{}).
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state ch create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.AccountState{}).
		IfNotExists().
		// WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state pg create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.LatestAccountState{}).
		IfNotExists().
		WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "latest account state pg create table")
	}

	return createIndexes(ctx, pgDB)
}

func (r *Repository) AddAddressLabel(ctx context.Context, label *core.AddressLabel) error {
	_, err := r.pg.NewInsert().Model(label).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "pg insert label")
	}
	_, err = r.ch.NewInsert().Model(label).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "ch insert label")
	}
	return nil
}

func (r *Repository) GetAddressLabel(ctx context.Context, a addr.Address) (*core.AddressLabel, error) {
	var label = core.AddressLabel{Address: a}

	err := r.pg.NewSelect().Model(&label).WherePK().Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &label, nil
}

func (r *Repository) AddAccountStates(ctx context.Context, tx bun.Tx, accounts []*core.AccountState) error {
	if len(accounts) == 0 {
		return nil
	}

	for _, a := range accounts {
		_, err := tx.NewInsert().Model(a).Exec(ctx)
		if err != nil {
			return errors.Wrapf(err, "cannot insert new %s acc state", a.Address.String())
		}
	}

	addrTxLT := make(map[addr.Address]uint64)
	for _, a := range accounts {
		if addrTxLT[a.Address] < a.LastTxLT {
			addrTxLT[a.Address] = a.LastTxLT
		}
	}

	for a, lt := range addrTxLT {
		_, err := tx.NewInsert().
			Model(&core.LatestAccountState{
				Address:  a,
				LastTxLT: lt,
			}).
			On("CONFLICT (address) DO UPDATE").
			Where("latest_account_state.last_tx_lt < ?", lt).
			Set("last_tx_lt = EXCLUDED.last_tx_lt").
			Exec(ctx)
		if err != nil {
			return errors.Wrapf(err, "cannot set latest state for %s", &a)
		}
	}

	_, err := r.ch.NewInsert().Model(&accounts).Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

func logAccountStateDataUpdate(acc *core.AccountState) {
	types, _ := json.Marshal(acc.Types)                   //nolint:errchkjson // no need
	getMethods, _ := json.Marshal(acc.ExecutedGetMethods) //nolint:errchkjson // no need

	log.Info().
		Str("address", acc.Address.Base64()).
		Uint64("last_tx_lt", acc.LastTxLT).
		RawJSON("types", types).
		RawJSON("executed_get_methods", getMethods).
		Msg("updating account state data")
}

func (r *Repository) UpdateAccountStates(ctx context.Context, accounts []*core.AccountState) error {
	if len(accounts) == 0 {
		return nil
	}

	for _, a := range accounts {
		logAccountStateDataUpdate(a)

		_, err := r.pg.NewUpdate().Model(a).
			Set("types = ?types").
			Set("owner_address = ?owner_address").
			Set("minter_address = ?minter_address").
			Set("fake = ?fake").
			Set("executed_get_methods = ?executed_get_methods").
			Set("content_uri = ?content_uri").
			Set("content_name = ?content_name").
			Set("content_description = ?content_description").
			Set("content_image = ?content_image").
			Set("content_image_data = ?content_image_data").
			Set("jetton_balance = ?jetton_balance").
			WherePK().
			Exec(ctx)
		if err != nil {
			return errors.Wrapf(err, "cannot update %s acc state data", a.Address.String())
		}
	}

	_, err := r.ch.NewInsert().Model(&accounts).Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}
