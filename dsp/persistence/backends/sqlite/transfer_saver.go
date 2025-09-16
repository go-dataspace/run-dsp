package sqlite

import (
	"context"
	"errors"

	transferopts "go-dataspace.eu/run-dsp/dsp/persistence/options/transfer"
	"go-dataspace.eu/run-dsp/dsp/transfer"
)

func (p *Provider) GetTransfer(ctx context.Context, opts ...transferopts.RequestOption) (*transfer.Request, error) {
	return nil, errors.New("not implemented")
}

func (p *Provider) GetTransfers(ctx context.Context, opts ...transferopts.RequestOption) ([]*transfer.Request, error) {
	return nil, errors.New("not implemented")
}

func (p *Provider) PutTransfer(ctx context.Context, req *transfer.Request) error     { return nil }
func (p *Provider) ReleaseTransfer(ctx context.Context, req *transfer.Request) error { return nil }
