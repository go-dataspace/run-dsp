package sqlite

import (
	"context"
	"errors"

	agreementopts "go-dataspace.eu/run-dsp/dsp/persistence/options/agreement"

	"go-dataspace.eu/run-dsp/odrl"
)

func (p *Provider) GetAgreement(ctx context.Context, opts ...agreementopts.Option) (*odrl.Agreement, error) {
	return nil, errors.New("Not implemented")
}

func (p *Provider) PutAgreement(ctx context.Context, agreement *odrl.Agreement) error {
	return errors.New("Not implemented")
}
