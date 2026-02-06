package sqlite

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite/models"
	agreementopts "go-dataspace.eu/run-dsp/dsp/persistence/options/agreement"

	"go-dataspace.eu/run-dsp/odrl"
)

func (p *Provider) GetAgreement(ctx context.Context, opts ...agreementopts.Option) (*odrl.Agreement, error) {
	agreement, err := newAgreementQueryBuilder(opts...).Get(ctx, p.h)
	if err != nil {
		return nil, err
	}
	return agreement.ToODRLAgreement()
}

func (p *Provider) PutAgreement(ctx context.Context, agreement *odrl.Agreement) error {
	rec, err := models.FromODRLAgreement(agreement)
	if err != nil {
		return err
	}
	_, err = p.h.NewInsert().Model(&rec).Exec(ctx)
	return err
}

type agreementBuilder struct {
	id *uuid.UUID
}

func newAgreementQueryBuilder(opts ...agreementopts.Option) *agreementBuilder {
	builder := &agreementBuilder{}
	for _, opt := range opts {
		opt(builder)
	}
	return builder
}

func (a *agreementBuilder) SetAgreementID(id uuid.UUID) { a.id = &id }

func (a *agreementBuilder) Get(ctx context.Context, h *bun.DB) (models.Agreement, error) {
	var agreement models.Agreement
	if a.id == nil {
		return models.Agreement{}, errors.New("id is compulsory for agreement queries")
	}
	err := h.NewSelect().Model(&agreement).Where("id = ?", a.id.URN()).Scan(ctx)
	return agreement, err
}
