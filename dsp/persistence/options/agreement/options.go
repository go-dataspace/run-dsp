package agreement

import (
	"github.com/google/uuid"
)

type Query interface {
	SetAgreementID(*uuid.UUID)
}

type Option func(Query)

func WithAgreementID(id *uuid.UUID) Option {
	return func(nq Query) { nq.SetAgreementID(id) }
}
