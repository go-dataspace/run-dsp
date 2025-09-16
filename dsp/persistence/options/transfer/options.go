package transfer

import (
	"net/url"

	"github.com/google/uuid"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/transfer"
)

type RequestQuery interface {
	SetConsumerPID(*uuid.UUID)
	SetProviderPID(*uuid.UUID)
	SetAgreementID(*uuid.UUID)
	SetCallback(*url.URL)
	SetRole(constants.DataspaceRole)
	SetState(transfer.State)
	SetDirection(transfer.Direction)
	SetRW()
}

type RequestOption func(RequestQuery)

func WithConsumerPID(pid *uuid.UUID) RequestOption {
	return func(rq RequestQuery) { rq.SetConsumerPID(pid) }
}

func WithProviderPID(pid *uuid.UUID) RequestOption {
	return func(rq RequestQuery) { rq.SetProviderPID(pid) }
}

func WithAgreementID(id *uuid.UUID) RequestOption {
	return func(rq RequestQuery) { rq.SetAgreementID(id) }
}

func WithCallback(u *url.URL) RequestOption {
	return func(rq RequestQuery) { rq.SetCallback(u) }
}

func WithRole(role constants.DataspaceRole) RequestOption {
	return func(rq RequestQuery) { rq.SetRole(role) }
}

func WithState(state transfer.State) RequestOption {
	return func(rq RequestQuery) { rq.SetState(state) }
}

func WithWR() RequestOption {
	return func(rq RequestQuery) { rq.SetRW() }
}
