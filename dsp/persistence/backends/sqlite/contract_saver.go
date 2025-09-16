package sqlite

import (
	"context"
	"errors"

	"go-dataspace.eu/run-dsp/dsp/contract"
	contractopts "go-dataspace.eu/run-dsp/dsp/persistence/options/contract"
)

func (p *Provider) GetContract(ctx context.Context, opts ...contractopts.NegotiationOption) (*contract.Negotiation, error) {
	return nil, errors.New("Not implemented")
}

func (p *Provider) GetContracts(ctx context.Context, opts ...contractopts.NegotiationOption) ([]*contract.Negotiation, error) {
	return make([]*contract.Negotiation, 0), errors.New("Not implemented")
}

func (p *Provider) PutContract(ctx context.Context, contract *contract.Negotiation) error {
	return errors.New("Not implemented")
}

func (p *Provider) ReleaseContract(ctx context.Context, contract *contract.Negotiation) error {
	return errors.New("Not implemented")
}
