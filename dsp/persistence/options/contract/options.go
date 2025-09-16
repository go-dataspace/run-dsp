package contract

import (
	"net/url"

	"github.com/google/uuid"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
)

type NegotiationQuery interface {
	SetConsumerPID(uuid.UUID)
	SetProviderPID(uuid.UUID)
	SetAgreementID(uuid.UUID)
	SetCallback(*url.URL)
	SetRole(constants.DataspaceRole)
	SetState(contract.State)
	SetRW()
}

type NegotiationOption func(NegotiationQuery)

func WithConsumerPID(pid uuid.UUID) NegotiationOption {
	return func(nq NegotiationQuery) { nq.SetConsumerPID(pid) }
}

func WithProviderPID(pid uuid.UUID) NegotiationOption {
	return func(nq NegotiationQuery) { nq.SetProviderPID(pid) }
}

func WithRolePID(pid uuid.UUID, role constants.DataspaceRole) NegotiationOption {
	switch role {
	case constants.DataspaceConsumer:
		return WithConsumerPID(pid)
	case constants.DataspaceProvider:
		return WithProviderPID(pid)
	default:
		panic("Undefined dataspace role given")
	}
}

func WithAgreementID(id uuid.UUID) NegotiationOption {
	return func(nq NegotiationQuery) { nq.SetAgreementID(id) }
}

func WithCallback(u *url.URL) NegotiationOption {
	return func(nq NegotiationQuery) { nq.SetCallback(u) }
}

func WithRole(role constants.DataspaceRole) NegotiationOption {
	return func(nq NegotiationQuery) { nq.SetRole(role) }
}

func WithState(state contract.State) NegotiationOption {
	return func(nq NegotiationQuery) { nq.SetState(state) }
}

func WithRW() NegotiationOption {
	return func(nq NegotiationQuery) { nq.SetRW() }
}
