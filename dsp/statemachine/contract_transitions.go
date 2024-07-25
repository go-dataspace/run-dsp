// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statemachine

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/provider/v1"
	"github.com/google/uuid"
)

var (
	emptyUUID     = uuid.UUID{}
	dspaceContext = jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}})
)

type Contracter interface {
	GetProviderPID() uuid.UUID
	GetConsumerPID() uuid.UUID
	GetState() ContractState
	GetCallback() *url.URL
	SetCallback(u string) error
	GetSelf() *url.URL
	SetState(state ContractState) error
	GetContract() *Contract
	GetOffer() odrl.Offer
	GetContractNegotiation() shared.ContractNegotiation
}

// ContractArchiver is an interface to retrieving and saving of contract negotiations.
// ContractPhase represents a contract in a certain state.
type ContractNegotiationState interface {
	Contracter
	Recv(ctx context.Context, message any) (context.Context, ContractNegotiationState, error)
	Send(ctx context.Context) (func(), error)
	GetArchiver() Archiver
	GetProvider() providerv1.ProviderServiceClient
	GetReconciler() *Reconciler
}

type stateMachineDeps struct {
	a Archiver
	p providerv1.ProviderServiceClient
	r *Reconciler
}

func (cd *stateMachineDeps) GetArchiver() Archiver                         { return cd.a }
func (cd *stateMachineDeps) GetProvider() providerv1.ProviderServiceClient { return cd.p }
func (cd *stateMachineDeps) GetReconciler() *Reconciler                    { return cd.r }

// ContractNegotiationInitial is an initial state for a contract that hasn't been actually
// been submitted yet.
type ContractNegotiationInitial struct {
	*Contract
	stateMachineDeps
}

// Recv on the initial state gets called on both the provider and consumer, it's only called
// when a consumer receives an initial request message, or a provider receives an initial offer
// message. It will set the desired states but not generate the missing PID.
func (cn *ContractNegotiationInitial) Recv(
	ctx context.Context, message any,
) (context.Context, ContractNegotiationState, error) {
	ctx, logger := logging.InjectLabels(ctx, "recv_type", fmt.Sprintf("%T", cn))
	logger.Debug("Receiving message")
	switch t := message.(type) {
	case shared.ContractRequestMessage:
		ctx, logger = logging.InjectLabels(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
			"dataset_target", cn.GetOffer().Target,
		)
		logger.Debug("Received message")

		target, err := uuid.Parse(cn.GetOffer().Target)
		if err != nil {
			logger.Error("target is not a valid UUID", "err", err)
			return ctx, nil, fmt.Errorf("target is not a valid UUID: %w", err)
		}
		// This is the initial request, we can assume all data is freshly made based on the request.
		_, err = cn.GetProvider().GetDataset(ctx, &providerv1.GetDatasetRequest{
			DatasetId: target.String(),
		})
		if err != nil {
			logger.Error("target dataset not found", "err", err)
			return ctx, nil, fmt.Errorf("dataset %s: %w", cn.GetOffer().Target, ErrNotFound)
		}
		if err := cn.SetState(ContractStates.REQUESTED); err != nil {
			logger.Error("could not transition state", "err", err)
			return ctx, nil, fmt.Errorf("could not set state: %w", err)
		}
		cn.Contract.providerPID = uuid.New()
		cn.Contract.initial = true
		if err := cn.a.PutProviderContract(ctx, cn.GetContract()); err != nil {
			logger.Error("failed to save contract", "err", err)
			return ctx, nil, fmt.Errorf("failed to save contract: %w", err)
		}
		ctx, cns := GetContractNegotiation(ctx, cn.a, cn.GetContract(), cn.GetProvider(), cn.GetReconciler())
		return ctx, cns, nil
	case shared.ContractOfferMessage:
		ctx, logger = logging.InjectLabels(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
			"dataset_target", cn.GetOffer().Target,
		)
		// This is the initial offer, we can assuem all data is freshly made based on the offer.
		if err := cn.SetState(ContractStates.OFFERED); err != nil {
			logger.Error("could not transition state", "err", err)
			return ctx, nil, fmt.Errorf("could not set state: %w", err)
		}
		cn.Contract.consumerPID = uuid.New()
		cn.Contract.initial = true
		if err := cn.a.PutConsumerContract(ctx, cn.GetContract()); err != nil {
			logger.Error("failed to save contract", "err", err)
			return ctx, nil, fmt.Errorf("failed to save contract: %w", err)
		}
		ctx, cns := GetContractNegotiation(ctx, cn.a, cn.GetContract(), cn.GetProvider(), cn.GetReconciler())
		return ctx, cns, nil
	default:
		return ctx, nil, fmt.Errorf("Message type %s is not supported at this stage", t)
	}
}

// Send progresses to the next state for the INITIAL state.
// This needs either the contract's consumer or provider PID set, but not both.
// If the provider PID is set, it will send out a contract offer to the callback.
// If the consumer PID is set, it will send out a contract request to the callback.
func (cn *ContractNegotiationInitial) Send(ctx context.Context) (func(), error) { //nolint: cyclop
	ctx, logger := logging.InjectLabels(ctx, "send_type", fmt.Sprintf("%T", cn))
	if (cn.GetConsumerPID() == emptyUUID && cn.GetProviderPID() == emptyUUID) ||
		(cn.GetConsumerPID() != emptyUUID && cn.GetProviderPID() != emptyUUID) {
		logger.Error("can't deduce if provider or consumer")
		return func() {}, fmt.Errorf("can't deduce if provider or consumer contract")
	}

	switch {
	case cn.GetConsumerPID() != emptyUUID:
		if err := cn.a.PutConsumerContract(ctx, cn.GetContract()); err != nil {
			logger.Error("failed to save contract", "err", err)
			return func() {}, fmt.Errorf("could not save contract: %w", err)
		}
		return sendContractRequest(ctx, cn.GetReconciler(), cn.GetContract())
	case cn.GetProviderPID() != emptyUUID:
		u, err := uuid.Parse(cn.GetOffer().Target)
		if err != nil {
			logger.Error("invalid UUID", "err", err)
			return func() {}, fmt.Errorf("invalid UUID `%s`: %w", cn.GetOffer().Target, err)
		}
		_, err = cn.GetProvider().GetDataset(ctx, &providerv1.GetDatasetRequest{
			DatasetId: u.String(),
		})
		if err != nil {
			logger.Error("Dataset not found", "err", err)
			return nil, ErrNotFound
		}
		if err := cn.a.PutProviderContract(ctx, cn.GetContract()); err != nil {
			logger.Error("Could not send contract", "err", err)
			return func() {}, fmt.Errorf("could not save contract: %w", err)
		}
		return sendContractOffer(ctx, cn.GetReconciler(), cn.GetContract())
	default:
		logger.Error("Could not deduce type of contract")
		return func() {}, fmt.Errorf("can't deduce if provider or consumer contract")
	}
}

// ContractNegotiationRequested represents the requested state.
type ContractNegotiationRequested struct {
	*Contract
	stateMachineDeps
}

// Recv gets called when a consumer receives a request message, it will verify the PIDs, and
// forcefully set the callback. After that it will set the status of the contract to OFFERED.
func (cn *ContractNegotiationRequested) Recv(
	ctx context.Context, message any,
) (context.Context, ContractNegotiationState, error) {
	ctx, logger := logging.InjectLabels(ctx, "recv_type", fmt.Sprintf("%T", cn))
	logger.Debug("Receiving message")
	var consumerPID, providerPID, callbackAddress string
	var targetState ContractState

	switch t := message.(type) {
	case shared.ContractOfferMessage:
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = t.CallbackAddress
		targetState = ContractStates.OFFERED
		if ppid, err := uuid.Parse(providerPID); err == nil && cn.GetProviderPID() == emptyUUID {
			cn.Contract.providerPID = ppid
		}
		ctx, logger = logging.InjectLabels(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
		)
		logger.Debug("Received message")
	case shared.ContractAgreementMessage:
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = t.CallbackAddress
		cn.Contract.agreement = t.Agreement
		targetState = ContractStates.AGREED
		ctx, logger = logging.InjectLabels(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
		)
	default:
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
	logger.Debug("Received message")
	return verifyAndTransform(ctx, cn, providerPID, consumerPID, callbackAddress, targetState)
}

// Send determines if an offer of agreemetn has to be sent.
func (cn *ContractNegotiationRequested) Send(ctx context.Context) (func(), error) {
	ctx, _ = logging.InjectLabels(ctx, "send_type", fmt.Sprintf("%T", cn))
	// Detect if this is a consumer initiated or provider initiated request.
	if cn.Contract.initial {
		cn.Contract.initial = false
		return sendContractOffer(ctx, cn.GetReconciler(), cn.GetContract())
	} else {
		return sendContractAgreement(ctx, cn.GetReconciler(), cn.GetContract(), cn.GetArchiver())
	}
}

type ContractNegotiationOffered struct {
	*Contract
	stateMachineDeps
}

// Recv gets called when a provider receives a request message. It will verify it and set the proper status for
// the next step.
func (cn *ContractNegotiationOffered) Recv(
	ctx context.Context, message any,
) (context.Context, ContractNegotiationState, error) {
	ctx, logger := logging.InjectLabels(ctx, "recv_type", fmt.Sprintf("%T", cn))
	logger.Debug("Receiving message")
	var consumerPID, providerPID, callbackAddress string
	var targetState ContractState

	switch t := message.(type) {
	case shared.ContractRequestMessage:
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = t.CallbackAddress
		targetState = ContractStates.REQUESTED
		if ppid, err := uuid.Parse(consumerPID); err == nil && cn.GetConsumerPID() == emptyUUID {
			cn.Contract.consumerPID = ppid
		}
		ctx, logger = logging.InjectLabels(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
		)
		logger.Debug("Received message")
	case shared.ContractNegotiationEventMessage:
		ctx, logger = logging.InjectLabels(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
			"event_type", t.EventType,
		)
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = cn.GetCallback().String()
		receivedStatus, err := ParseContractState(t.EventType)
		if err != nil {
			logger.Error("Event contained invalid status", "err", err)
			return ctx, nil, fmt.Errorf("event %s does not contain proper status: %w", t.EventType, err)
		}
		if receivedStatus != ContractStates.ACCEPTED {
			logger.Error("Event contained invalid status", "err", err)
			return ctx, nil, fmt.Errorf("invalid status: %s", receivedStatus)
		}
		targetState = receivedStatus
		logger.Debug("Received message")
	default:
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
	return verifyAndTransform(ctx, cn, providerPID, consumerPID, callbackAddress, targetState)
}

func (cn *ContractNegotiationOffered) Send(ctx context.Context) (func(), error) {
	ctx, _ = logging.InjectLabels(ctx, "send_type", fmt.Sprintf("%T", cn))
	// Detect if this is a consumer initiated or provider initiated request.
	if cn.Contract.initial {
		cn.Contract.initial = false
		return sendContractRequest(ctx, cn.GetReconciler(), cn.GetContract())
	} else {
		return sendContractEvent(
			ctx, cn.GetReconciler(), cn.GetContract(), cn.GetProviderPID(), ContractStates.ACCEPTED)
	}
}

type ContractNegotiationAccepted struct {
	*Contract
	stateMachineDeps
}

// Recv gets called on the consumer when the provider sends a contract agreement message.
func (cn *ContractNegotiationAccepted) Recv(
	ctx context.Context, message any,
) (context.Context, ContractNegotiationState, error) {
	ctx, logger := logging.InjectLabels(ctx, "recv_type", fmt.Sprintf("%T", cn))
	logger.Debug("Receiving message")
	m, ok := message.(shared.ContractAgreementMessage)
	if !ok {
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
	ctx, logger = logging.InjectLabels(ctx,
		"recv_msg_type", fmt.Sprintf("%T", m),
	)
	logger.Debug("Received message")
	cn.agreement = m.Agreement
	return verifyAndTransform(ctx, cn, m.ProviderPID, m.ConsumerPID, m.CallbackAddress, ContractStates.AGREED)
}

func (cn *ContractNegotiationAccepted) Send(ctx context.Context) (func(), error) {
	ctx, _ = logging.InjectLabels(ctx, "send_type", fmt.Sprintf("%T", cn))
	return sendContractAgreement(ctx, cn.GetReconciler(), cn.GetContract(), cn.GetArchiver())
}

type ContractNegotiationAgreed struct {
	*Contract
	stateMachineDeps
}

func (cn *ContractNegotiationAgreed) Recv(
	ctx context.Context, message any,
) (context.Context, ContractNegotiationState, error) {
	ctx, logger := logging.InjectLabels(ctx, "recv_type", fmt.Sprintf("%T", cn))
	logger.Info("Receiving me")
	m, ok := message.(shared.ContractAgreementVerificationMessage)
	if !ok {
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
	ctx, logger = logging.InjectLabels(ctx,
		"recv_msg_type", fmt.Sprintf("%T", m),
	)
	logger.Debug("Received message")
	return verifyAndTransform(ctx, cn, m.ProviderPID, m.ConsumerPID, cn.GetCallback().String(), ContractStates.VERIFIED)
}

func (cn *ContractNegotiationAgreed) Send(ctx context.Context) (func(), error) {
	ctx, _ = logging.InjectLabels(ctx, "send_type", fmt.Sprintf("%T", cn))
	return sendContractVerification(ctx, cn.GetReconciler(), cn.GetContract())
}

type ContractNegotiationVerified struct {
	*Contract
	stateMachineDeps
}

func (cn *ContractNegotiationVerified) Recv(
	ctx context.Context, message any,
) (context.Context, ContractNegotiationState, error) {
	ctx, logger := logging.InjectLabels(ctx, "recv_type", fmt.Sprintf("%T", cn))
	logger.Debug("Receiving message")
	m, ok := message.(shared.ContractNegotiationEventMessage)
	if !ok {
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
	ctx, logger = logging.InjectLabels(ctx,
		"recv_msg_type", fmt.Sprintf("%T", m),
		"event_type", m.EventType,
	)
	receivedStatus, err := ParseContractState(m.EventType)
	if err != nil {
		logger.Error("event does not contain the proper status", "err", err)
		return ctx, nil, fmt.Errorf("event %s does not contain proper status: %w", m.EventType, err)
	}
	if receivedStatus != ContractStates.FINALIZED {
		logger.Error("invalid status")
		return ctx, nil, fmt.Errorf("invalid status: %s", receivedStatus)
	}
	logger.Debug("Received message")
	return verifyAndTransform(
		ctx, cn, m.ProviderPID, m.ConsumerPID, cn.GetCallback().String(), ContractStates.FINALIZED)
}

func (cn *ContractNegotiationVerified) Send(ctx context.Context) (func(), error) {
	ctx, _ = logging.InjectLabels(ctx, "send_type", fmt.Sprintf("%T", cn))
	return sendContractEvent(
		ctx, cn.GetReconciler(), cn.GetContract(), cn.GetConsumerPID(), ContractStates.FINALIZED)
}

type ContractNegotiationFinalized struct {
	*Contract
	stateMachineDeps
}

func (cn *ContractNegotiationFinalized) Recv(
	ctx context.Context, message any,
) (context.Context, ContractNegotiationState, error) {
	return ctx, nil, fmt.Errorf("this is a final state")
}

func (cn *ContractNegotiationFinalized) Send(ctx context.Context) (func(), error) {
	return func() {}, nil
}

type ContractNegotiationTerminated struct {
	*Contract
	stateMachineDeps
}

func (cn *ContractNegotiationTerminated) Recv(
	ctx context.Context, message any,
) (context.Context, ContractNegotiationState, error) {
	return ctx, nil, fmt.Errorf("this is a final state")
}

func (cn *ContractNegotiationTerminated) Send(ctx context.Context) (func(), error) {
	// Nothing to do here.
	return func() {}, nil
}

func NewContract(
	ctx context.Context,
	store Archiver,
	provider providerv1.ProviderServiceClient,
	reconciler *Reconciler,
	providerPID, consumerPID uuid.UUID,
	state ContractState,
	offer odrl.Offer,
	callback, self *url.URL,
	role DataspaceRole,
) (context.Context, ContractNegotiationState, error) {
	contract := &Contract{
		providerPID: providerPID,
		consumerPID: consumerPID,
		state:       state,
		offer:       offer,
		callback:    callback,
		self:        self,
		role:        role,
	}
	var err error
	if role == DataspaceConsumer {
		err = store.PutConsumerContract(ctx, contract)
	} else {
		err = store.PutProviderContract(ctx, contract)
	}
	if err != nil {
		return ctx, nil, err
	}
	ctx, cn := GetContractNegotiation(ctx, store, contract, provider, reconciler)
	return ctx, cn, nil
}

func GetContractNegotiation(
	ctx context.Context,
	store Archiver,
	c *Contract,
	p providerv1.ProviderServiceClient,
	r *Reconciler,
) (context.Context, ContractNegotiationState) {
	var cns ContractNegotiationState
	deps := stateMachineDeps{a: store, p: p, r: r}
	switch c.GetState() {
	case ContractStates.INITIAL:
		cns = &ContractNegotiationInitial{Contract: c, stateMachineDeps: deps}
	case ContractStates.REQUESTED:
		cns = &ContractNegotiationRequested{Contract: c, stateMachineDeps: deps}
	case ContractStates.OFFERED:
		cns = &ContractNegotiationOffered{Contract: c, stateMachineDeps: deps}
	case ContractStates.AGREED:
		cns = &ContractNegotiationAgreed{Contract: c, stateMachineDeps: deps}
	case ContractStates.ACCEPTED:
		cns = &ContractNegotiationAccepted{Contract: c, stateMachineDeps: deps}
	case ContractStates.VERIFIED:
		cns = &ContractNegotiationVerified{Contract: c, stateMachineDeps: deps}
	case ContractStates.FINALIZED:
		cns = &ContractNegotiationFinalized{Contract: c, stateMachineDeps: deps}
	case ContractStates.TERMINATED:
		cns = &ContractNegotiationTerminated{Contract: c, stateMachineDeps: deps}
	default:
		panic("Invalid contract state.")
	}
	ctx, logger := logging.InjectLabels(ctx,
		"contract_consumerPID", cns.GetConsumerPID().String(),
		"contract_providerPID", cns.GetProviderPID().String(),
		"contract_state", cns.GetState().String(),
		"contract_role", cns.GetContract().role,
	)
	logger.Debug("Found contract")
	return ctx, cns
}

func verifyAndTransform(
	ctx context.Context,
	cn ContractNegotiationState,
	providerPID, consumerPID, callbackAddress string,
	targetState ContractState,
) (context.Context, ContractNegotiationState, error) {
	ctx, logger := logging.InjectLabels(ctx, "target_state", targetState)
	if cn.GetProviderPID().URN() != strings.ToLower(providerPID) {
		logger.Error(
			"given provider PID %s didn't match contract provider PID %s",
			"given", providerPID,
			"existing", cn.GetProviderPID().URN(),
		)
		return ctx, nil, fmt.Errorf(
			"given provider PID %s didn't match contract provider PID %s",
			providerPID,
			cn.GetProviderPID().URN(),
		)
	}
	if cn.GetConsumerPID().URN() != strings.ToLower(consumerPID) {
		logger.Error(
			"given consumer PID %s didn't match contract consumer PID %s",
			"given", consumerPID,
			"existing", cn.GetConsumerPID().URN(),
		)
		return ctx, nil, fmt.Errorf(
			"given consumer PID %s didn't match contract consumer PID %s",
			consumerPID,
			cn.GetConsumerPID().URN(),
		)
	}
	err := cn.SetCallback(callbackAddress)
	if err != nil {
		logger.Error("Invalid callback address", "err", err)
		return ctx, nil, fmt.Errorf("invalid callback address: %s", callbackAddress)
	}
	if err := cn.SetState(targetState); err != nil {
		logger.Error("Could not set state", "err", err)
		return ctx, nil, fmt.Errorf("could not set state: %w", err)
	}

	if cn.GetContract().role == DataspaceConsumer {
		err = cn.GetArchiver().PutConsumerContract(ctx, cn.GetContract())
	} else {
		err = cn.GetArchiver().PutProviderContract(ctx, cn.GetContract())
	}
	if err != nil {
		logger.Error("Could not set state", "err", err)
		return ctx, nil, fmt.Errorf("failed to save contract: %w", err)
	}
	ctx, cns := GetContractNegotiation(ctx, cn.GetArchiver(), cn.GetContract(), cn.GetProvider(), cn.GetReconciler())
	return ctx, cns, nil
}
