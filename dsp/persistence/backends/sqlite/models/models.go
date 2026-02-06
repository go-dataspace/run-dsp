// Package models contains models for bun
package models

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"go-dataspace.eu/run-dsp/odrl"
)

// Agreement is an ODRL agreement record, these should be immutable so won't have a modified
// field.
type Agreement struct {
	bun.BaseModel `bun:"table:agreements,alias:a"`

	ID   string `bun:",pk,notnull,unique"`
	ODRL string `bun:",notnull"`

	Created time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

func FromODRLAgreement(ag *odrl.Agreement) (Agreement, error) {
	// Parsing ID so that it can be normalised to an URN.
	id, err := uuid.Parse(ag.ID)
	if err != nil {
		return Agreement{}, err
	}
	text, err := json.Marshal(ag)
	if err != nil {
		return Agreement{}, err
	}
	return Agreement{
		ID:   id.URN(),
		ODRL: string(text),
	}, nil
}

func (a Agreement) ToODRLAgreement() (*odrl.Agreement, error) {
	var o odrl.Agreement
	err := json.Unmarshal([]byte(a.ODRL), &o)
	return &o, err
}

// ContractNegotiation is a contract negotiation record.
type ContractNegotiation struct {
	bun.BaseModel `bun:"table:contract_negotiations,alias:c"`

	// As it's unwise to have nullable columns as a PK, we set an internal PK
	ID int64 `bun:",pk,autoincrement"`

	ProviderPID *string `bun:",unique"`
	ConsumerPID *string `bun:",unique"`

	AgreementID *string    `bun:"agreement_id,unique"`
	Agreement   *Agreement `bun:"rel:has-one,join:agreement_id=id"`

	Offer string `bun:",notnull"`
	State string `bun:",notnull"`

	CallbackURL string `bun:",notnull"`
	SelfURL     string `bun:",notnull"`

	Role string `bun:",notnull"`

	AutoAccept bool `bun:",notnull"`

	RequesterInfo *string
	TraceInfo     string

	Locked bool `bun:",notnull"`

	Created  time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	Modified time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

var _ bun.BeforeAppendModelHook = (*ContractNegotiation)(nil)

func (m *ContractNegotiation) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	switch query.(type) { //nolint:gocritic // This is a switch to make extra query hooks easy in the future.
	case *bun.UpdateQuery:
		m.Modified = time.Now()
	}
	return nil
}

// TransferRequest is a transfer request record.
type TransferRequest struct {
	bun.BaseModel `bun:"table:transfer_requests,alias:t"`

	// As it's unwise to have nullable columns as a PK, we set an internal PK
	ID int64 `bun:",pk,autoincrement"`

	ProviderPID *string `bun:",unique"`
	ConsumerPID *string `bun:",unique"`

	AgreementID string    `bun:"agreement_id,notnull"`
	Agreement   Agreement `bun:"rel:has-one,join:agreement_id=id"`

	Target      string `bun:",notnull"`
	Format      string `bun:",notnull"`
	PublishInfo *string
	Direction   string
	State       string `bun:",notnull"`

	CallbackURL string `bun:",notnull"`
	SelfURL     string `bun:",notnull"`

	Role string `bun:",notnull"`

	RequesterInfo string `bun:",notnull"`
	TraceInfo     string `bun:",notnull"`

	Locked bool `bun:",notnull"`

	Created  time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	Modified time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

var _ bun.BeforeAppendModelHook = (*TransferRequest)(nil)

func (m *TransferRequest) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	switch query.(type) { //nolint:gocritic // This is a switch to make extra query hooks easy in the future.
	case *bun.UpdateQuery:
		m.Modified = time.Now()
	}
	return nil
}
