package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite/models"
	contractopts "go-dataspace.eu/run-dsp/dsp/persistence/options/contract"
)

func (p *Provider) GetContract(
	ctx context.Context,
	opts ...contractopts.NegotiationOption,
) (*contract.Negotiation, error) {
	params := newNegotiationParamBuilder(opts...)
	if params.RW {
		return p.getContractLocked(ctx, params)
	}

	var cn models.ContractNegotiation
	var ok bool
	qb := p.h.NewSelect().Model(&cn).Relation("Agreement")
	qb, ok = params.GetWhereGroup(qb.QueryBuilder(), nil).Unwrap().(*bun.SelectQuery)
	if !ok {
		// This should never happen, but if it does, best to crash and burn.
		panic("could not unwrap query")
	}
	err := qb.Scan(ctx)
	if err != nil {
		return nil, err
	}
	return contract.FromModel(cn, true)
}

//nolint:dupl // The duplication here makes the code simpler.
func (p *Provider) GetContracts(ctx context.Context,
	opts ...contractopts.NegotiationOption,
) ([]*contract.Negotiation, error) {
	params := newNegotiationParamBuilder(opts...)
	if params.RW {
		return nil, errors.New("can't get multiple records in read-write mode")
	}

	var cns []models.ContractNegotiation
	var ok bool
	qb := p.h.NewSelect().Model(&cns).Relation("Agreement")
	qb, ok = params.GetWhereGroup(qb.QueryBuilder(), nil).Unwrap().(*bun.SelectQuery)
	if !ok {
		// This should never happen, but if it does, best to crash and burn.
		panic("could not unwrap query")
	}
	err := qb.Scan(ctx)
	if err != nil {
		return nil, err
	}
	negs := make([]*contract.Negotiation, len(cns))
	for i, cn := range cns {
		negs[i], err = contract.FromModel(cn, true)
		if err != nil {
			return nil, err
		}
	}
	return negs, err
}

func (p *Provider) getContractLocked(
	ctx context.Context,
	params *negotiationParamBuilder,
) (*contract.Negotiation, error) {
	var ok bool
	// First check if there's any records at all.
	qb := p.h.NewSelect().Model((*models.ContractNegotiation)(nil))
	qb, ok = params.GetWhereGroup(qb.QueryBuilder(), nil).Unwrap().(*bun.SelectQuery)
	if !ok {
		// This should never happen, but if it does, best to crash and burn.
		panic("could not unwrap query")
	}
	count, err := qb.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count != 1 {
		// TODO: Make persistence specific errors.
		return nil, fmt.Errorf("found non-1 amount of instances: %d", count)
	}

	var cn models.ContractNegotiation
	for range p.lockTimeout {
		qb := p.h.NewUpdate().Model(&cn).Set("locked = ?", true)
		f := false
		var ok bool
		qb, ok = params.GetWhereGroup(qb.QueryBuilder(), &f).Unwrap().(*bun.UpdateQuery)
		if !ok {
			// This should never happen, but if it does, best to crash and burn.
			panic("could not unwrap query")
		}
		err := qb.Returning("*").Scan(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				ctxslog.Debug(ctx, "Could not acquire lock, sleeping...")
				time.Sleep(1 * time.Second)
				continue
			}
			return nil, err
		}

		// As UPDATE ... RETURNING does not support JOINS we have to do this manually.
		if cn.AgreementID != nil {
			var ag models.Agreement
			err = p.h.NewSelect().Model(&ag).Where("id = ?", cn.AgreementID).Scan(ctx)
			if err != nil {
				return nil, err
			}
			cn.Agreement = &ag
		}
		return contract.FromModel(cn, false)
	}
	return nil, fmt.Errorf("could not acquire lock")
}

func (p *Provider) PutContract(ctx context.Context, contract *contract.Negotiation) error {
	if contract.ReadOnly() {
		panic("trying to save a read only model, this is certainly a bug")
	}
	if !contract.Modified() {
		return nil
	}
	model, err := contract.ToModel()
	if err != nil {
		return err
	}
	_, err = p.h.NewInsert().
		Model(&model).
		On("CONFLICT (id) DO UPDATE").
		Set(`
			provider_pid = EXCLUDED.provider_pid,
			consumer_pid = EXCLUDED.consumer_pid,
			agreement_id = EXCLUDED.agreement_id,
			offer = EXCLUDED.offer,
			state = EXCLUDED.state,
			callback_url = EXCLUDED.callback_url,
			self_url = EXCLUDED.self_url,
			auto_accept = EXCLUDED.auto_accept,
			requester_info = EXCLUDED.requester_info,
			trace_info = EXCLUDED.trace_info,
			locked = EXCLUDED.locked
		`).Exec(ctx)
	return err
}

func (p *Provider) ReleaseContract(ctx context.Context, contract *contract.Negotiation) error {
	model, err := contract.ToModel()
	if err != nil {
		return err
	}
	_, err = p.h.NewUpdate().
		Model(&model).
		Set("locked = ?", false).
		WherePK().
		Exec(ctx)
	return err
}

type negotiationParamBuilder struct {
	consumerPID *uuid.UUID
	providerPID *uuid.UUID
	agreementID *uuid.UUID
	callback    *url.URL
	role        *constants.DataspaceRole
	state       *contract.State
	RW          bool
}

func newNegotiationParamBuilder(opts ...contractopts.NegotiationOption) *negotiationParamBuilder {
	builder := &negotiationParamBuilder{}
	for _, opt := range opts {
		opt(builder)
	}
	return builder
}

func (cb *negotiationParamBuilder) SetConsumerPID(pid uuid.UUID)      { cb.consumerPID = &pid }
func (cb *negotiationParamBuilder) SetProviderPID(pid uuid.UUID)      { cb.providerPID = &pid }
func (cb *negotiationParamBuilder) SetAgreementID(id uuid.UUID)       { cb.agreementID = &id }
func (cb *negotiationParamBuilder) SetCallback(u *url.URL)            { cb.callback = u }
func (cb *negotiationParamBuilder) SetRole(r constants.DataspaceRole) { cb.role = &r }
func (cb *negotiationParamBuilder) SetState(s contract.State)         { cb.state = &s }
func (cb *negotiationParamBuilder) SetRW()                            { cb.RW = true }

func (cb negotiationParamBuilder) GetWhereGroup(qb bun.QueryBuilder, locked *bool) bun.QueryBuilder {
	return qb.WhereGroup(" AND ", func(qb bun.QueryBuilder) bun.QueryBuilder {
		if locked != nil {
			qb = qb.Where("locked = ?", *locked)
		}
		if cb.consumerPID != nil {
			qb = qb.Where("consumer_pid = ?", cb.consumerPID.URN())
		}
		if cb.providerPID != nil {
			qb = qb.Where("provider_pid = ?", cb.providerPID.URN())
		}
		if cb.agreementID != nil {
			qb = qb.Where("agreement_id = ?", cb.agreementID.URN())
		}
		if cb.callback != nil {
			qb = qb.Where("callback_url = ?", cb.callback.String())
		}
		if cb.role != nil {
			qb = qb.Where("role = ?", cb.role.String())
		}
		if cb.state != nil {
			qb = qb.Where("state = ?", cb.state.String())
		}
		return qb
	})
}
