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
	Recv(ctx context.Context, message any) (ContractNegotiationState, error)
	Send(ctx context.Context) (func(), error)
	GetArchiver() Archiver
	GetProvider() providerv1.ProviderServiceClient
	GetRequester() Requester
}

type stateMachineDeps struct {
	a Archiver
	p providerv1.ProviderServiceClient
	r Requester
}

func (cd *stateMachineDeps) GetArchiver() Archiver                         { return cd.a }
func (cd *stateMachineDeps) GetProvider() providerv1.ProviderServiceClient { return cd.p }
func (cd *stateMachineDeps) GetRequester() Requester                       { return cd.r }

// ContractNegotiationInitial is an initial state for a contract that hasn't been actually
// been submitted yet.
type ContractNegotiationInitial struct {
	*Contract
	stateMachineDeps
}

// Recv on the initial state gets called on both the provider and consumer, it's only called
// when a consumer receives an initial request message, or a provider receives an initial offer
// message. It will set the desired states but not generate the missing PID.
func (cn *ContractNegotiationInitial) Recv(ctx context.Context, message any) (ContractNegotiationState, error) {
	switch t := message.(type) {
	case shared.ContractRequestMessage:
		target, err := uuid.Parse(cn.GetOffer().Target)
		if err != nil {
			return nil, fmt.Errorf("target is not a valid UUID: %w", err)
		}
		// This is the initial request, we can assume all data is freshly made based on the request.
		_, err = cn.GetProvider().GetDataset(ctx, &providerv1.GetDatasetRequest{
			DatasetId: target.String(),
		})
		if err != nil {
			return nil, fmt.Errorf("dataset %s: %w", cn.GetOffer().Target, ErrNotFound)
		}
		if err := cn.SetState(ContractStates.REQUESTED); err != nil {
			return nil, fmt.Errorf("could not set state: %w", err)
		}
		cn.Contract.providerPID = uuid.New()
		cn.Contract.initial = true
		if err := cn.a.PutProviderContract(ctx, cn.GetContract()); err != nil {
			return nil, fmt.Errorf("failed to save contract: %w", err)
		}
		return GetContractNegotiation(cn.a, cn.GetContract(), cn.GetProvider(), cn.GetRequester()), nil
	case shared.ContractOfferMessage:
		// This is the initial offer, we can assuem all data is freshly made based on the offer.
		if err := cn.SetState(ContractStates.OFFERED); err != nil {
			return nil, fmt.Errorf("could not set state: %w", err)
		}
		cn.Contract.consumerPID = uuid.New()
		cn.Contract.initial = true
		if err := cn.a.PutConsumerContract(ctx, cn.GetContract()); err != nil {
			return nil, fmt.Errorf("failed to save contract: %w", err)
		}
		return GetContractNegotiation(cn.a, cn.GetContract(), cn.GetProvider(), cn.GetRequester()), nil
	default:
		return nil, fmt.Errorf("Message type %s is not supported at this stage", t)
	}
}

// Send progresses to the next state for the INITIAL state.
// This needs either the contract's consumer or provider PID set, but not both.
// If the provider PID is set, it will send out a contract offer to the callback.
// If the consumer PID is set, it will send out a contract request to the callback.
func (cn *ContractNegotiationInitial) Send(ctx context.Context) (func(), error) { //nolint: cyclop
	if (cn.GetConsumerPID() == emptyUUID && cn.GetProviderPID() == emptyUUID) ||
		(cn.GetConsumerPID() != emptyUUID && cn.GetProviderPID() != emptyUUID) {
		return func() {}, fmt.Errorf("can't deduce if provider or consumer contract")
	}

	switch {
	case cn.GetConsumerPID() != emptyUUID:
		if err := cn.a.PutConsumerContract(ctx, cn.GetContract()); err != nil {
			return func() {}, fmt.Errorf("could not save contract: %w", err)
		}
		return sendContractRequest(ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver())
	case cn.GetProviderPID() != emptyUUID:
		u, err := uuid.Parse(cn.GetOffer().Target)
		if err != nil {
			return func() {}, fmt.Errorf("invalid UUID `%s`: %w", cn.GetOffer().Target, err)
		}
		_, err = cn.GetProvider().GetDataset(ctx, &providerv1.GetDatasetRequest{
			DatasetId: u.String(),
		})
		if err != nil {
			return nil, ErrNotFound
		}
		if err := cn.a.PutProviderContract(ctx, cn.GetContract()); err != nil {
			return func() {}, fmt.Errorf("could not save contract: %w", err)
		}
		return sendContractOffer(ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver())
	default:
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
func (cn *ContractNegotiationRequested) Recv(ctx context.Context, message any) (ContractNegotiationState, error) {
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
	case shared.ContractAgreementMessage:
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = t.CallbackAddress
		cn.Contract.agreement = t.Agreement
		targetState = ContractStates.AGREED
	default:
		return nil, fmt.Errorf("unsupported message type")
	}
	return verifyAndTransform(ctx, cn, providerPID, consumerPID, callbackAddress, targetState)
}

// Send determines if an offer of agreemetn has to be sent.
func (cn *ContractNegotiationRequested) Send(ctx context.Context) (func(), error) {
	// Detect if this is a consumer initiated or provider initiated request.
	if cn.Contract.initial {
		cn.Contract.initial = false
		return sendContractOffer(ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver())
	} else {
		return sendContractAgreement(ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver())
	}
}

type ContractNegotiationOffered struct {
	*Contract
	stateMachineDeps
}

// Recv gets called when a provider receives a request message. It will verify it and set the proper status for
// the next step.
func (cn *ContractNegotiationOffered) Recv(ctx context.Context, message any) (ContractNegotiationState, error) {
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
	case shared.ContractNegotiationEventMessage:
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = cn.GetCallback().String()
		receivedStatus, err := ParseContractState(t.EventType)
		if err != nil {
			return nil, fmt.Errorf("event %s does not contain proper status: %w", t.EventType, err)
		}
		if receivedStatus != ContractStates.ACCEPTED {
			return nil, fmt.Errorf("invalid status: %s", receivedStatus)
		}
		targetState = receivedStatus
	default:
		return nil, fmt.Errorf("unsupported message type")
	}
	return verifyAndTransform(ctx, cn, providerPID, consumerPID, callbackAddress, targetState)
}

func (cn *ContractNegotiationOffered) Send(ctx context.Context) (func(), error) {
	// Detect if this is a consumer initiated or provider initiated request.
	if cn.Contract.initial {
		cn.Contract.initial = false
		return sendContractRequest(ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver())
	} else {
		return sendContractEvent(
			ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver(), cn.GetProviderPID(), ContractStates.ACCEPTED)
	}
}

type ContractNegotiationAccepted struct {
	*Contract
	stateMachineDeps
}

// Recv gets called on the consumer when the provider sends a contract agreement message.
func (cn *ContractNegotiationAccepted) Recv(ctx context.Context, message any) (ContractNegotiationState, error) {
	m, ok := message.(shared.ContractAgreementMessage)
	if !ok {
		return nil, fmt.Errorf("unsupported message type")
	}
	cn.agreement = m.Agreement
	return verifyAndTransform(ctx, cn, m.ProviderPID, m.ConsumerPID, m.CallbackAddress, ContractStates.AGREED)
}

func (cn *ContractNegotiationAccepted) Send(ctx context.Context) (func(), error) {
	return sendContractAgreement(ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver())
}

type ContractNegotiationAgreed struct {
	*Contract
	stateMachineDeps
}

func (cn *ContractNegotiationAgreed) Recv(ctx context.Context, message any) (ContractNegotiationState, error) {
	m, ok := message.(shared.ContractAgreementVerificationMessage)
	if !ok {
		return nil, fmt.Errorf("unsupported message type")
	}
	return verifyAndTransform(ctx, cn, m.ProviderPID, m.ConsumerPID, cn.GetCallback().String(), ContractStates.VERIFIED)
}

func (cn *ContractNegotiationAgreed) Send(ctx context.Context) (func(), error) {
	return sendContractVerification(ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver())
}

type ContractNegotiationVerified struct {
	*Contract
	stateMachineDeps
}

func (cn *ContractNegotiationVerified) Recv(ctx context.Context, message any) (ContractNegotiationState, error) {
	m, ok := message.(shared.ContractNegotiationEventMessage)
	if !ok {
		return nil, fmt.Errorf("unsupported message type")
	}
	receivedStatus, err := ParseContractState(m.EventType)
	if err != nil {
		return nil, fmt.Errorf("event %s does not contain proper status: %w", m.EventType, err)
	}
	if receivedStatus != ContractStates.FINALIZED {
		return nil, fmt.Errorf("invalid status: %s", receivedStatus)
	}
	cns, err := verifyAndTransform(
		ctx, cn, m.ProviderPID, m.ConsumerPID, cn.GetCallback().String(), ContractStates.FINALIZED)
	if err != nil {
		return nil, err
	}
	if err := cn.GetArchiver().PutAgreement(ctx, &cn.Contract.agreement); err != nil {
		return nil, err
	}
	return cns, nil
}

func (cn *ContractNegotiationVerified) Send(ctx context.Context) (func(), error) {
	if err := cn.GetArchiver().PutAgreement(ctx, &cn.GetContract().agreement); err != nil {
		return nil, err
	}
	return sendContractEvent(
		ctx, cn.GetRequester(), cn.GetContract(), cn.GetArchiver(), cn.GetConsumerPID(), ContractStates.FINALIZED)
}

type ContractNegotiationFinalized struct {
	*Contract
	stateMachineDeps
}

func (cn *ContractNegotiationFinalized) Recv(ctx context.Context, message any) (ContractNegotiationState, error) {
	return nil, fmt.Errorf("this is a final state")
}

func (cn *ContractNegotiationFinalized) Send(ctx context.Context) (func(), error) {
	return func() {}, fmt.Errorf("can't progress from %s", cn.GetState())
}

type ContractNegotiationTerminated struct {
	*Contract
	stateMachineDeps
}

func (cn *ContractNegotiationTerminated) Recv(ctx context.Context, message any) (ContractNegotiationState, error) {
	return nil, fmt.Errorf("this is a final state")
}

func (cn *ContractNegotiationTerminated) Send(ctx context.Context) (func(), error) {
	return func() {}, fmt.Errorf("can't progress from %s", cn.GetState())
}

func NewContract(
	ctx context.Context,
	store Archiver,
	provider providerv1.ProviderServiceClient,
	requester Requester,
	providerPID, consumerPID uuid.UUID,
	state ContractState,
	offer odrl.Offer,
	callback, self *url.URL,
	role ContractRole,
) ContractNegotiationState {
	contract := &Contract{
		providerPID: providerPID,
		consumerPID: consumerPID,
		state:       state,
		offer:       offer,
		callback:    callback,
		self:        self,
		role:        role,
	}
	return GetContractNegotiation(store, contract, provider, requester)
}

func GetContractNegotiation(
	store Archiver,
	c *Contract,
	p providerv1.ProviderServiceClient,
	r Requester,
) ContractNegotiationState {
	deps := stateMachineDeps{a: store, p: p, r: r}
	switch c.GetState() {
	case ContractStates.INITIAL:
		return &ContractNegotiationInitial{Contract: c, stateMachineDeps: deps}
	case ContractStates.REQUESTED:
		return &ContractNegotiationRequested{Contract: c, stateMachineDeps: deps}
	case ContractStates.OFFERED:
		return &ContractNegotiationOffered{Contract: c, stateMachineDeps: deps}
	case ContractStates.AGREED:
		return &ContractNegotiationAgreed{Contract: c, stateMachineDeps: deps}
	case ContractStates.ACCEPTED:
		return &ContractNegotiationAccepted{Contract: c, stateMachineDeps: deps}
	case ContractStates.VERIFIED:
		return &ContractNegotiationVerified{Contract: c, stateMachineDeps: deps}
	case ContractStates.FINALIZED:
		return &ContractNegotiationFinalized{Contract: c, stateMachineDeps: deps}
	case ContractStates.TERMINATED:
		return &ContractNegotiationTerminated{Contract: c, stateMachineDeps: deps}
	default:
		panic("Invalid contract state.")
	}
}

func verifyAndTransform(
	ctx context.Context,
	cn ContractNegotiationState,
	providerPID, consumerPID, callbackAddress string,
	targetState ContractState,
) (ContractNegotiationState, error) {
	if cn.GetProviderPID().URN() != strings.ToLower(providerPID) {
		return nil, fmt.Errorf(
			"given provider PID %s didn't match contract provider PID %s",
			providerPID,
			cn.GetProviderPID().URN(),
		)
	}
	if cn.GetConsumerPID().URN() != strings.ToLower(consumerPID) {
		return nil, fmt.Errorf(
			"given consumer PID %s didn't match contract consumer PID %s",
			consumerPID,
			cn.GetConsumerPID().URN(),
		)
	}
	err := cn.SetCallback(callbackAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid callback address: %s", callbackAddress)
	}
	if err := cn.SetState(targetState); err != nil {
		return nil, fmt.Errorf("could not set state: %w", err)
	}

	if cn.GetContract().role == ContractConsumer {
		err = cn.GetArchiver().PutConsumerContract(ctx, cn.GetContract())
	} else {
		err = cn.GetArchiver().PutProviderContract(ctx, cn.GetContract())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to save contract: %w", err)
	}
	return GetContractNegotiation(cn.GetArchiver(), cn.GetContract(), cn.GetProvider(), cn.GetRequester()), nil
}
