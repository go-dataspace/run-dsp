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
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite/models"
	transferopts "go-dataspace.eu/run-dsp/dsp/persistence/options/transfer"
	"go-dataspace.eu/run-dsp/dsp/transfer"
)

func (p *Provider) GetTransfer(ctx context.Context, opts ...transferopts.RequestOption) (*transfer.Request, error) {
	params := newTransferParamBuilder(opts...)
	if params.RW {
		return p.getTransferLocked(ctx, params)
	}

	var tr models.TransferRequest
	var ok bool
	qb := p.h.NewSelect().Model(&tr).Relation("Agreement")
	qb, ok = params.GetWhereGroup(qb.QueryBuilder(), nil).Unwrap().(*bun.SelectQuery)
	if !ok {
		// This should never happen, but if it does, best to crash and burn.
		panic("could not unwrap query")
	}
	err := qb.Scan(ctx)
	if err != nil {
		return nil, err
	}
	return transfer.FromModel(tr, true)
}

//nolint:dupl // The duplication here makes the code simpler.
func (p *Provider) GetTransfers(ctx context.Context, opts ...transferopts.RequestOption) ([]*transfer.Request, error) {
	params := newTransferParamBuilder(opts...)
	if params.RW {
		return nil, errors.New("can't get multiple records in read-write mode")
	}

	var trs []models.TransferRequest
	var ok bool
	qb := p.h.NewSelect().Model(&trs).Relation("Agreement")
	qb, ok = params.GetWhereGroup(qb.QueryBuilder(), nil).Unwrap().(*bun.SelectQuery)
	if !ok {
		// This should never happen, but if it does, best to crash and burn.
		panic("could not unwrap query")
	}
	err := qb.Scan(ctx)
	if err != nil {
		return nil, err
	}
	reqs := make([]*transfer.Request, len(trs))
	for i, tr := range trs {
		reqs[i], err = transfer.FromModel(tr, true)
		if err != nil {
			return nil, err
		}
	}
	return reqs, err
}

func (p *Provider) getTransferLocked(ctx context.Context, params *transferParamBuilder) (*transfer.Request, error) {
	// First check if there's any records at all.
	var ok bool
	qb := p.h.NewSelect().Model((*models.TransferRequest)(nil))
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

	var tr models.TransferRequest
	for range p.lockTimeout {
		qb := p.h.NewUpdate().Model(&tr).Set("locked = ?", true)
		f := false
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
		var ag models.Agreement
		err = p.h.NewSelect().Model(&ag).Where("id = ?", tr.AgreementID).Scan(ctx)
		if err != nil {
			return nil, err
		}
		tr.Agreement = ag
		return transfer.FromModel(tr, false)
	}
	return nil, fmt.Errorf("could not acquire lock")
}

func (p *Provider) PutTransfer(ctx context.Context, req *transfer.Request) error {
	if req.ReadOnly() {
		panic("trying to save a read only model, this is certainly a bug")
	}
	if !req.Modified() {
		return nil
	}
	model, err := req.ToModel()
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
			target = EXCLUDED.target,
			format = EXCLUDED.format,
			publish_info = EXCLUDED.publish_info,
			direction = EXCLUDED.direction,
			state = EXCLUDED.state,
			callback_url = EXCLUDED.callback_url,
			self_url = EXCLUDED.self_url,
			requester_info = EXCLUDED.requester_info,
			trace_info = EXCLUDED.trace_info,
			locked = EXCLUDED.locked
		`).Exec(ctx)
	return err
}

func (p *Provider) ReleaseTransfer(ctx context.Context, req *transfer.Request) error {
	model, err := req.ToModel()
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

type transferParamBuilder struct {
	consumerPID *uuid.UUID
	providerPID *uuid.UUID
	agreementID *uuid.UUID
	callback    *url.URL
	role        *constants.DataspaceRole
	state       *transfer.State
	direction   *transfer.Direction
	RW          bool
}

func newTransferParamBuilder(opts ...transferopts.RequestOption) *transferParamBuilder {
	builder := &transferParamBuilder{}
	for _, opt := range opts {
		opt(builder)
	}
	return builder
}

func (cb *transferParamBuilder) SetConsumerPID(pid uuid.UUID)      { cb.consumerPID = &pid }
func (cb *transferParamBuilder) SetProviderPID(pid uuid.UUID)      { cb.providerPID = &pid }
func (cb *transferParamBuilder) SetAgreementID(id uuid.UUID)       { cb.agreementID = &id }
func (cb *transferParamBuilder) SetCallback(u *url.URL)            { cb.callback = u }
func (cb *transferParamBuilder) SetRole(r constants.DataspaceRole) { cb.role = &r }
func (cb *transferParamBuilder) SetState(s transfer.State)         { cb.state = &s }
func (cb *transferParamBuilder) SetDirection(s transfer.Direction) { cb.direction = &s }
func (cb *transferParamBuilder) SetRW()                            { cb.RW = true }

func (cb transferParamBuilder) GetWhereGroup(qb bun.QueryBuilder, locked *bool) bun.QueryBuilder {
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
		if cb.direction != nil {
			qb = qb.Where("direction = ?", cb.direction.String())
		}
		return qb
	})
}
