package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	agreementopts "go-dataspace.eu/run-dsp/dsp/persistence/options/agreement"
	contractopts "go-dataspace.eu/run-dsp/dsp/persistence/options/contract"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

func (p *Provider) GetContract(ctx context.Context, opts ...contractopts.NegotiationOption) (*contract.Negotiation, error) {
	builder := newContractBuilder(opts...)
	query, args := builder.Build()
	row := p.handle.QueryRowContext(ctx, query, args...)
	return p.rowToNegotiation(ctx, row, builder.RW)
}

func (p *Provider) GetContracts(ctx context.Context, opts ...contractopts.NegotiationOption) ([]*contract.Negotiation, error) {
	builder := newContractBuilder(opts...)
	query, args := builder.Build()
	rows, err := p.handle.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("couldn't run contract negotiation query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	negotiations := make([]*contract.Negotiation, 0)
	for rows.Next() {
		negotiation, err := p.rowToNegotiation(ctx, rows, builder.RW)
		if err != nil {
			return nil, fmt.Errorf("could not scan row: %w", err)
		}
		negotiations = append(negotiations, negotiation)
	}
	return negotiations, nil
}

func (p *Provider) PutContract(ctx context.Context, contract *contract.Negotiation) error {
	return errors.New("Not implemented")
}

func (p *Provider) ReleaseContract(ctx context.Context, contract *contract.Negotiation) error {
	return errors.New("Not implemented")
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (p *Provider) rowToNegotiation(ctx context.Context, row rowScanner, rw bool) (*contract.Negotiation, error) {
	var provider_pid, consumer_pid, agreement_id *string
	var offer, state, callback_url, self_url, role string
	var requester_info string
	var auto_accept bool
	if err := row.Scan(
		provider_pid, consumer_pid, agreement_id,
		&offer, &state, &callback_url, &self_url, &role,
		&auto_accept, &requester_info,
	); err != nil {
		return nil, fmt.Errorf("could not find row: %w", err)
	}
	var agreement *odrl.Agreement
	if agreement_id != nil {
		agreement_uuid, err := uuid.Parse(*agreement_id)
		if err != nil {
			return nil, fmt.Errorf("contract negotiation had unparsable agreement UUID: %s: %w", *agreement_id, err)
		}
		// TODO: This should be converted to a JOIN for performance reasons.
		agreement, err = p.GetAgreement(ctx, agreementopts.WithAgreementID(agreement_uuid))
		if err != nil {
			return nil, err
		}
	}

	return newContractNegotiation(
		ctx, provider_pid, consumer_pid, agreement,
		offer, state, callback_url, self_url, role, requester_info,
		auto_accept, rw,
	)
}

func newContractNegotiation(
	ctx context.Context,
	providerPIDString, consumerPIDString *string,
	agreement *odrl.Agreement,
	offerJSON, stateString, callbackString, selfString, roleString, requesterInfoString string,
	auto_accept, rw bool,
) (*contract.Negotiation, error) {
	providerPID := uuid.UUID{}
	if providerPIDString != nil {
		var err error
		providerPID, err = uuid.Parse(*providerPIDString)
		if err != nil {
			return nil, fmt.Errorf("contract negotiation had unparsable provider UUID: %s: %w", *providerPIDString, err)
		}
	}
	consumerPID := uuid.UUID{}
	if consumerPIDString != nil {
		var err error
		consumerPID, err = uuid.Parse(*consumerPIDString)
		if err != nil {
			return nil, fmt.Errorf("contract negotiation had unparsable consumer UUID: %s: %w", *consumerPIDString, err)
		}
	}
	var offer odrl.Offer
	if err := json.Unmarshal([]byte(offerJSON), &offer); err != nil {
		return nil, fmt.Errorf("contract negotiation had unparsable odrl.Offer: %s: %w", offerJSON, err)
	}
	state, err := contract.ParseState(stateString)
	if err != nil {
		return nil, fmt.Errorf("contract negotiation had unparsable state: %s: %w", stateString, err)
	}
	callbackURL, err := url.Parse(callbackString)
	if err != nil {
		return nil, fmt.Errorf("contract negotiation had unparsable callback URL: %s: %w", callbackString, err)
	}
	selfURL, err := url.Parse(selfString)
	if err != nil {
		return nil, fmt.Errorf("contract negotiation had unparsable self URL: %s: %w", selfString, err)
	}
	var role constants.DataspaceRole
	switch roleString {
	case "consumer":
		role = constants.DataspaceConsumer
	case "provider":
		role = constants.DataspaceProvider
	default:
		return nil, fmt.Errorf("contract negotiation had unparsable role: %s", roleString)
	}
	var requesterInfo dsrpc.RequesterInfo
	if err := json.Unmarshal([]byte(requesterInfoString), &requesterInfo); err != nil {
		return nil, fmt.Errorf("contract negotiation had unparsable requester info: %s: %w", requesterInfoString, err)
	}
	neg := contract.New(ctx, providerPID, consumerPID, state, offer, callbackURL, selfURL, role, auto_accept, &requesterInfo)
	if !rw {
		neg.SetReadOnly()
	}

	if agreement != nil {
		neg.SetAgreement(agreement)
	}
	return neg, nil
}

type contractBuilder struct {
	consumerPID *uuid.UUID
	providerPID *uuid.UUID
	agreementID *uuid.UUID
	callback    *url.URL
	role        *constants.DataspaceRole
	state       *contract.State
	RW          bool
}

func newContractBuilder(opts ...contractopts.NegotiationOption) *contractBuilder {
	builder := &contractBuilder{}
	for _, opt := range opts {
		opt(builder)
	}
	return builder
}

func (qb *contractBuilder) SetConsumerPID(pid uuid.UUID)      { qb.consumerPID = &pid }
func (qb *contractBuilder) SetProviderPID(pid uuid.UUID)      { qb.providerPID = &pid }
func (qb *contractBuilder) SetAgreementID(id uuid.UUID)       { qb.agreementID = &id }
func (qb *contractBuilder) SetCallback(u *url.URL)            { qb.callback = u }
func (qb *contractBuilder) SetRole(r constants.DataspaceRole) { qb.role = &r }
func (qb *contractBuilder) SetState(s contract.State)         { qb.state = &s }
func (qb *contractBuilder) SetRW()                            { qb.RW = true }

func (qb *contractBuilder) Build() (string, []any) {
	args := make([]any, 0)
	queryString := `
	SELECT
		provider_pid, consumer_pid, agreement_pid, offer,
	 	state, callback_url, self_url, role, auto_accept,
		requester_info, trace_info
	FROM contract_negotations
	WHERE
	locked=false`

	if qb.consumerPID != nil {
		queryString += "AND consumer_pid=?"
		args = append(args, qb.consumerPID.String())
	}
	if qb.providerPID != nil {
		queryString += "AND provider_pid=?"
		args = append(args, qb.providerPID.String())
	}
	if qb.agreementID != nil {
		queryString += "AND agreement_id=?"
		args = append(args, qb.agreementID.String())
	}
	if qb.callback != nil {
		queryString += "AND callback_url=?"
		args = append(args, qb.callback.String())
	}
	if qb.role != nil {
		queryString += "AND role=?"
		args = append(args, qb.callback.String())
	}
	return queryString, args
}
