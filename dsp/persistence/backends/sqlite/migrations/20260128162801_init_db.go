package migrations

import (
	"context"

	"github.com/uptrace/bun"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite/models"
)

type index struct {
	model  any
	name   string
	column string
}

//nolint:funlen // Initial migration, long by nature.
func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			ctxslog.Info(ctx, "Initialising database")
			agreements := (*models.Agreement)(nil)
			contract_negotiations := (*models.ContractNegotiation)(nil)
			transfer_requests := (*models.TransferRequest)(nil)

			indexes := []index{
				{
					model:  contract_negotiations,
					name:   "idx_cn_provider_pid",
					column: "provider_pid",
				},
				{
					model:  contract_negotiations,
					name:   "idx_cn_consumer_pid",
					column: "consumer_pid",
				},
				{
					model:  contract_negotiations,
					name:   "idx_cn_agreement_id",
					column: "agreement_id",
				},
				{
					model:  contract_negotiations,
					name:   "idx_cn_callback_url",
					column: "callback_url",
				},
				{
					model:  contract_negotiations,
					name:   "idx_cn_role",
					column: "role",
				},
				{
					model:  contract_negotiations,
					name:   "idx_cn_state",
					column: "state",
				},
				{
					model:  transfer_requests,
					name:   "idx_tr_provider_pid",
					column: "provider_pid",
				},
				{
					model:  transfer_requests,
					name:   "idx_tr_consumer_pid",
					column: "consumer_pid",
				},
				{
					model:  transfer_requests,
					name:   "idx_tr_agreement_id",
					column: "agreement_id",
				},
				{
					model:  transfer_requests,
					name:   "idx_tr_callback_url",
					column: "callback_url",
				},
				{
					model:  transfer_requests,
					name:   "idx_tr_role",
					column: "role",
				},
				{
					model:  transfer_requests,
					name:   "idx_tr_state",
					column: "state",
				},
			}

			for _, model := range []any{agreements, contract_negotiations, transfer_requests} {
				_, err := db.NewCreateTable().WithForeignKeys().Model(model).IfNotExists().Exec(ctx)
				if err != nil {
					return err
				}
			}

			for _, idx := range indexes {
				_, err := db.NewCreateIndex().Model(idx.model).Index(idx.name).Column(idx.column).Exec(ctx)
				if err != nil {
					return err
				}
			}

			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			ctxslog.Info(ctx, "Refusing to remove initial tables")
			return nil
		},
	)
}
