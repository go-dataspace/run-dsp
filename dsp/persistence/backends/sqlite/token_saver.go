package sqlite

import (
	"context"
	"errors"
)

func (p *Provider) GetToken(ctx context.Context, key string) (string, error) {
	return "", errors.New("not implemented")
}

func (p *Provider) DelToken(ctx context.Context, key string) error {
	return errors.New("not implemented")
}

func (p *Provider) PutToken(ctx context.Context, key string, token string) error {
	return errors.New("not implemented")
}
