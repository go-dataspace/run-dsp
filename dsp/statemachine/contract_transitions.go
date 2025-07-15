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
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/internal/authforwarder"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

var (
	emptyUUID   = uuid.UUID{}
	ErrNotFound = errors.New("not found")
)

type applyFunc func() error

type Contracter interface {
	GetProviderPID() uuid.UUID
	GetConsumerPID() uuid.UUID
	GetState() contract.State
	GetCallback() *url.URL
	SetCallback(u string) error
	GetSelf() *url.URL
	SetState(state contract.State) error
	GetContract() *contract.Negotiation
	GetOffer() odrl.Offer
	GetContractNegotiation() shared.ContractNegotiation
	AutoAccept() bool
	SetAutoAccept()
	GetLogFields(suffix string) []any
}

// ContractArchiver is an interface to retrieving and saving of contract negotiations.
// ContractPhase represents a contract in a certain state.
type ContractNegotiationState interface {
	Contracter
	Recv(ctx context.Context, message any) (context.Context, applyFunc, error)
	Send(ctx context.Context) (applyFunc, error)
	GetProvider() dsrpc.ProviderServiceClient
	GetReconciler() Reconciler
	GetContractService() dsrpc.ContractServiceClient
}

type stateMachineDeps struct {
	p dsrpc.ProviderServiceClient
	c dsrpc.ContractServiceClient
	r Reconciler
}

func (cd *stateMachineDeps) GetProvider() dsrpc.ProviderServiceClient        { return cd.p }
func (cd *stateMachineDeps) GetReconciler() Reconciler                       { return cd.r }
func (cd *stateMachineDeps) GetContractService() dsrpc.ContractServiceClient { return cd.c }

// ContractNegotiationInitial is an initial state for a contract that hasn't been actually
// been submitted yet.
type ContractNegotiationInitial struct {
	*contract.Negotiation
	stateMachineDeps
}

// Recv on the initial state gets called on both the provider and consumer, it's only called
// when a consumer receives an initial request message, or a provider receives an initial offer
// message. It will set the desired states but not generate the missing PID.
func (cn *ContractNegotiationInitial) Recv(
	ctx context.Context, message any,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx, "recv_type", fmt.Sprintf("%T", cn))
	ctxslog.Debug(ctx, "Receiving message")
	switch t := message.(type) {
	case shared.ContractRequestMessage:
		return cn.processContractRequest(ctx, t)
	case shared.ContractOfferMessage:
		return cn.processContractOffer(ctx, t)
	default:
		return ctx, nil, fmt.Errorf("Message type %s is not supported at this stage", t)
	}
}

func (cn *ContractNegotiationInitial) processContractOffer(
	ctx context.Context,
	t shared.ContractOfferMessage,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx,
		"recv_msg_type", fmt.Sprintf("%T", t),
		"dataset_target", cn.GetOffer().Target,
	)
	// This is the initial offer, we can assume all data is freshly made based on the offer.
	if err := cn.SetState(contract.States.OFFERED); err != nil {
		return ctx, nil, ctxslog.ReturnError(ctx, "could not set state", err)
	}
	cn.Negotiation.SetConsumerPID(uuid.New())
	cn.Negotiation.SetInitial()

	offer, err := json.Marshal(cn.GetOffer())
	if err != nil {
		return ctx, nil, ctxslog.ReturnError(ctx, "couldn't marshall offer", err)
	}

	ntfyFunc := func() error {
		_, err := cn.c.OfferReceived(ctx, &dsrpc.ContractServiceOfferReceivedRequest{
			Pid:           cn.GetConsumerPID().String(),
			Offer:         string(offer),
			RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
		})
		return err
	}
	if cn.AutoAccept() || cn.c == nil {
		ntfyFunc = func() error { return nil }
	}

	return ctx, ntfyFunc, nil
}

func (cn *ContractNegotiationInitial) processContractRequest(
	ctx context.Context,
	t shared.ContractRequestMessage,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx,
		"recv_msg_type", fmt.Sprintf("%T", t),
		"dataset_target", cn.GetOffer().Target,
	)
	ctxslog.Debug(ctx, "Received message")

	target, err := shared.URNtoRawID(cn.GetOffer().Target)
	if err != nil {
		return ctx, nil, ctxslog.ReturnError(ctx, "can't parse URN", err)
	}

	// This is the initial request, we can assume all data is freshly made based on the request.
	permissions := cn.GetOffer().Permission
	if len(permissions) == 0 || permissions[0].Action != "odrl:copy" {
		_, err = cn.GetProvider().GetDataset(ctx, &dsrpc.GetDatasetRequest{
			DatasetId:     target,
			RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
		})
		if err != nil {
			// FIXME: Check here if the error is actually "not found".
			//        Here we also get the permission denied, which should terminate the transfer.
			return ctx, nil, ctxslog.ReturnError(ctx, "dataset not found",
				fmt.Errorf("dataset %s: %w", cn.GetOffer().Target, ErrNotFound),
				"dataset", cn.GetOffer().Target)
		}
	}
	if err := cn.SetState(contract.States.REQUESTED); err != nil {
		return ctx, nil, ctxslog.ReturnError(ctx, "could not set state", err)
	}
	cn.Negotiation.SetProviderPID(uuid.New())
	cn.Negotiation.SetInitial()
	offer, err := json.Marshal(cn.GetOffer())
	if err != nil {
		return ctx, nil, ctxslog.ReturnError(ctx, "couldn't marshall offer", err)
	}

	ntfyFunc := func() error {
		_, err := cn.c.RequestReceived(ctx, &dsrpc.ContractServiceRequestReceivedRequest{
			Pid:           cn.GetProviderPID().String(),
			Offer:         string(offer),
			RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
		})
		return err
	}
	if cn.AutoAccept() || cn.c == nil {
		ntfyFunc = func() error { return nil }
	}

	return ctx, ntfyFunc, nil
}

// Send progresses to the next state for the INITIAL state.
// This needs either the contract's consumer or provider PID set, but not both.
// If the provider PID is set, it will send out a contract offer to the callback.
// If the consumer PID is set, it will send out a contract request to the callback.
func (cn *ContractNegotiationInitial) Send(ctx context.Context) (applyFunc, error) { //nolint:cyclop
	ctx = ctxslog.With(ctx, "send_type", fmt.Sprintf("%T", cn))
	if (cn.GetConsumerPID() == emptyUUID && cn.GetProviderPID() == emptyUUID) ||
		(cn.GetConsumerPID() != emptyUUID && cn.GetProviderPID() != emptyUUID) {
		return func() error { return nil }, ctxslog.ReturnError(ctx, "can't deduce if provider or consumer contract",
			errors.New("can't deduce role in contract"))
	}

	switch {
	case cn.GetConsumerPID() != emptyUUID:
		return sendContractRequest(ctx, cn.GetReconciler(), cn.GetContract())
	case cn.GetProviderPID() != emptyUUID:
		targetID, err := shared.URNtoRawID(cn.GetOffer().Target)
		if err != nil {
			return func() error { return nil }, ctxslog.ReturnError(ctx, "invalid URN", err, "target", cn.GetOffer().Target)
		}

		permissions := cn.GetOffer().Permission
		if len(permissions) == 0 || permissions[0].Action != "odrl:copy" {
			_, err = cn.GetProvider().GetDataset(ctx, &dsrpc.GetDatasetRequest{
				DatasetId:     targetID,
				RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
			})
			if err != nil {
				return nil, ctxslog.ReturnError(ctx, "Dataset not found", ErrNotFound, "real_err", err)
			}
		}
		return sendContractOffer(ctx, cn.GetReconciler(), cn.GetContract())
	default:
		return func() error { return nil }, ctxslog.ReturnError(ctx,
			"can't deduce if provider or consumer contract",
			errors.New("can't deduce negotiation role"),
		)
	}
}

// ContractNegotiationRequested represents the requested state.
type ContractNegotiationRequested struct {
	*contract.Negotiation
	stateMachineDeps
}

// Recv gets called when a consumer receives a request message, it will verify the PIDs, and
// forcefully set the callback. After that it will set the status of the contract to OFFERED.
func (cn *ContractNegotiationRequested) Recv(
	ctx context.Context, message any,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx, "recv_type", fmt.Sprintf("%T", cn))
	ctxslog.Debug(ctx, "Receiving message")
	var consumerPID, providerPID, callbackAddress string
	var targetState contract.State
	var af applyFunc

	switch t := message.(type) {
	case shared.ContractOfferMessage:
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = t.CallbackAddress
		targetState = contract.States.OFFERED
		if ppid, err := uuid.Parse(providerPID); err == nil && cn.GetProviderPID() == emptyUUID {
			cn.Negotiation.SetProviderPID(ppid)
		}
		ctx = ctxslog.With(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
		)
		ctxslog.Debug(ctx, "Received message")
		offer, err := json.Marshal(cn.GetOffer())
		if err != nil {
			return ctx, nil, err
		}

		af = func() error {
			_, err := cn.c.OfferReceived(ctx, &dsrpc.ContractServiceOfferReceivedRequest{
				Pid:           cn.GetConsumerPID().String(),
				Offer:         string(offer),
				RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
			})
			return err
		}
	case shared.ContractAgreementMessage:
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = t.CallbackAddress
		cn.Negotiation.SetAgreement(&t.Agreement)
		targetState = contract.States.AGREED
		ctx = ctxslog.With(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
		)
		af = func() error {
			_, err := cn.c.AgreementReceived(ctx, &dsrpc.ContractServiceAgreementReceivedRequest{
				Pid:           cn.GetConsumerPID().String(),
				RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
			})
			return err
		}
	case shared.ContractNegotiationTerminationMessage:
		return processTermination(ctx, t, cn)
	default:
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
	if cn.AutoAccept() || cn.c == nil {
		af = func() error { return nil }
	}
	ctxslog.Debug(ctx, "Received message")
	return verifyAndTransform(ctx, cn, providerPID, consumerPID, callbackAddress, targetState, af)
}

// Send determines if an offer of agreemetn has to be sent.
func (cn *ContractNegotiationRequested) Send(ctx context.Context) (applyFunc, error) {
	ctx = ctxslog.With(ctx, "send_type", fmt.Sprintf("%T", cn))
	// Detect if this is a consumer initiated or provider initiated request.
	if cn.Negotiation.Initial() {
		cn.Negotiation.UnsetInitial()
		return sendContractOffer(ctx, cn.GetReconciler(), cn.GetContract())
	} else {
		return sendContractAgreement(ctx, cn.GetReconciler(), cn.GetContract())
	}
}

type ContractNegotiationOffered struct {
	*contract.Negotiation
	stateMachineDeps
}

// Recv gets called when a provider receives a request message. It will verify it and set the proper status for
// the next step.
//
//nolint:funlen,cyclop
func (cn *ContractNegotiationOffered) Recv(
	ctx context.Context, message any,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx, "recv_type", fmt.Sprintf("%T", cn))
	ctxslog.Debug(ctx, "Receiving message")
	var consumerPID, providerPID, callbackAddress string
	var targetState contract.State
	var af applyFunc

	switch t := message.(type) {
	case shared.ContractRequestMessage:
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = t.CallbackAddress
		targetState = contract.States.REQUESTED
		if ppid, err := uuid.Parse(consumerPID); err == nil && cn.GetConsumerPID() == emptyUUID {
			cn.Negotiation.SetConsumerPID(ppid)
		}
		ctx = ctxslog.With(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
		)
		ctxslog.Debug(ctx, "Received message")
		offer, err := json.Marshal(cn.GetOffer())
		if err != nil {
			return ctx, nil, ctxslog.ReturnError(ctx, "couldn't marshall offer", err)
		}
		af = func() error {
			_, err := cn.c.RequestReceived(ctx, &dsrpc.ContractServiceRequestReceivedRequest{
				Pid:           cn.GetProviderPID().String(),
				Offer:         string(offer),
				RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
			})
			return err
		}
	case shared.ContractNegotiationEventMessage:
		ctx = ctxslog.With(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
			"event_type", t.EventType,
		)
		consumerPID = t.ConsumerPID
		providerPID = t.ProviderPID
		callbackAddress = cn.GetCallback().String()
		receivedStatus, err := contract.ParseState(t.EventType)
		if err != nil {
			return ctx, nil, ctxslog.ReturnError(ctx,
				"event does not contain proper status",
				err,
				"eventType", t.EventType,
			)
		}
		if receivedStatus != contract.States.ACCEPTED {
			return ctx, nil, ctxslog.ReturnError(ctx,
				"invalid status",
				fmt.Errorf("invalid status"),
				"received status", receivedStatus,
			)
		}
		targetState = receivedStatus
		ctxslog.Debug(ctx, "Received message")
		af = func() error {
			_, err := cn.c.AcceptedReceived(ctx, &dsrpc.ContractServiceAcceptedReceivedRequest{
				Pid:           cn.GetProviderPID().String(),
				RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
			})
			return err
		}
	case shared.ContractNegotiationTerminationMessage:
		return processTermination(ctx, t, cn)
	default:
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
	if cn.AutoAccept() {
		af = func() error { return nil }
	}
	return verifyAndTransform(ctx, cn, providerPID, consumerPID, callbackAddress, targetState, af)
}

func (cn *ContractNegotiationOffered) Send(ctx context.Context) (applyFunc, error) {
	ctx = ctxslog.With(ctx, "send_type", fmt.Sprintf("%T", cn))
	// Detect if this is a consumer initiated or provider initiated request.
	if cn.Negotiation.Initial() {
		cn.Negotiation.UnsetInitial()
		return sendContractRequest(ctx, cn.GetReconciler(), cn.GetContract())
	} else {
		return sendContractEvent(
			ctx, cn.GetReconciler(), cn.GetContract(), cn.GetProviderPID(), contract.States.ACCEPTED)
	}
}

type ContractNegotiationAccepted struct {
	*contract.Negotiation
	stateMachineDeps
}

// Recv gets called on the consumer when the provider sends a contract agreement message.
func (cn *ContractNegotiationAccepted) Recv(
	ctx context.Context, message any,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx, "recv_type", fmt.Sprintf("%T", cn))
	ctxslog.Debug(ctx, "Receiving message")
	switch t := message.(type) {
	case shared.ContractAgreementMessage:
		ctx = ctxslog.With(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
		)
		ctxslog.Debug(ctx, "Received message")
		cn.SetAgreement(&t.Agreement)
		af := func() error {
			_, err := cn.c.AgreementReceived(ctx, &dsrpc.ContractServiceAgreementReceivedRequest{
				Pid:           cn.GetConsumerPID().String(),
				RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
			})
			return err
		}
		if cn.AutoAccept() {
			af = func() error { return nil }
		}
		return verifyAndTransform(
			ctx,
			cn,
			t.ProviderPID,
			t.ConsumerPID,
			t.CallbackAddress,
			contract.States.AGREED,
			af,
		)
	case shared.ContractNegotiationTerminationMessage:
		return processTermination(ctx, t, cn)
	default:
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
}

func (cn *ContractNegotiationAccepted) Send(ctx context.Context) (applyFunc, error) {
	ctx = ctxslog.With(ctx, "send_type", fmt.Sprintf("%T", cn))
	return sendContractAgreement(ctx, cn.GetReconciler(), cn.GetContract())
}

type ContractNegotiationAgreed struct {
	*contract.Negotiation
	stateMachineDeps
}

func (cn *ContractNegotiationAgreed) Recv(
	ctx context.Context, message any,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx, "recv_type", fmt.Sprintf("%T", cn))
	ctxslog.Debug(ctx, "Receiving message")
	switch t := message.(type) {
	case shared.ContractAgreementVerificationMessage:
		ctx = ctxslog.With(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
		)
		af := func() error {
			_, err := cn.c.VerificationReceived(ctx, &dsrpc.ContractServiceVerificationReceivedRequest{
				Pid:           cn.GetProviderPID().String(),
				RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
			})
			return err
		}
		if cn.AutoAccept() {
			af = func() error { return nil }
		}
		return verifyAndTransform(
			ctx,
			cn,
			t.ProviderPID,
			t.ConsumerPID,
			cn.GetCallback().String(),
			contract.States.VERIFIED,
			af,
		)
	case shared.ContractNegotiationTerminationMessage:
		return processTermination(ctx, t, cn)
	default:
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
}

func (cn *ContractNegotiationAgreed) Send(ctx context.Context) (applyFunc, error) {
	ctx = ctxslog.With(ctx, "send_type", fmt.Sprintf("%T", cn))
	return sendContractVerification(ctx, cn.GetReconciler(), cn.GetContract())
}

type ContractNegotiationVerified struct {
	*contract.Negotiation
	stateMachineDeps
}

func (cn *ContractNegotiationVerified) Recv(
	ctx context.Context, message any,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx, "recv_type", fmt.Sprintf("%T", cn))
	ctxslog.Debug(ctx, "Receiving message")
	switch t := message.(type) {
	case shared.ContractNegotiationEventMessage:
		ctx = ctxslog.With(ctx,
			"recv_msg_type", fmt.Sprintf("%T", t),
			"event_type", t.EventType,
		)
		receivedStatus, err := contract.ParseState(t.EventType)
		if err != nil {
			return ctx, nil, ctxslog.ReturnError(ctx,
				"event does not contain proper status",
				err,
				"eventType", t.EventType,
			)
		}
		if receivedStatus != contract.States.FINALIZED {
			return ctx, nil, ctxslog.ReturnError(ctx,
				"invalid status",
				fmt.Errorf("invalid status received"),
				"receivedStatus", receivedStatus,
			)
		}
		ctxslog.Debug(ctx, "Received message")
		af := func() error {
			offer, err := shared.ValidateAndMarshal(ctx, cn.Negotiation.GetOffer())
			if err != nil {
				return err
			}
			_, err = cn.c.FinalizationReceived(ctx, &dsrpc.ContractServiceFinalizationReceivedRequest{
				Pid:           cn.GetLocalPID().String(),
				RequesterInfo: authforwarder.ExtractRequesterInfo(ctx),
				Offer:         string(offer),
			})
			return err
		}
		if cn.AutoAccept() {
			af = func() error { return nil }
		}
		return verifyAndTransform(
			ctx,
			cn,
			t.ProviderPID,
			t.ConsumerPID,
			cn.GetCallback().String(),
			contract.States.FINALIZED,
			af,
		)
	case shared.ContractNegotiationTerminationMessage:
		return processTermination(ctx, t, cn)
	default:
		return ctx, nil, fmt.Errorf("unsupported message type")
	}
}

func (cn *ContractNegotiationVerified) Send(ctx context.Context) (applyFunc, error) {
	ctx = ctxslog.With(ctx, "send_type", fmt.Sprintf("%T", cn))
	return sendContractEvent(
		ctx, cn.GetReconciler(), cn.GetContract(), cn.GetConsumerPID(), contract.States.FINALIZED)
}

type ContractNegotiationFinalized struct {
	*contract.Negotiation
	stateMachineDeps
}

func (cn *ContractNegotiationFinalized) Recv(
	ctx context.Context, message any,
) (context.Context, applyFunc, error) {
	return ctx, nil, fmt.Errorf("this is a final state")
}

func (cn *ContractNegotiationFinalized) Send(ctx context.Context) (applyFunc, error) {
	return func() error { return nil }, nil
}

type ContractNegotiationTerminated struct {
	*contract.Negotiation
	stateMachineDeps
}

func (cn *ContractNegotiationTerminated) Recv(
	ctx context.Context, message any,
) (context.Context, applyFunc, error) {
	return ctx, nil, fmt.Errorf("this is a final state")
}

func (cn *ContractNegotiationTerminated) Send(ctx context.Context) (applyFunc, error) {
	// Nothing to do here.
	return func() error { return nil }, nil
}

func GetContractNegotiation(
	c *contract.Negotiation,
	p dsrpc.ProviderServiceClient,
	cs dsrpc.ContractServiceClient,
	r Reconciler,
) ContractNegotiationState {
	var cns ContractNegotiationState
	deps := stateMachineDeps{p: p, c: cs, r: r}
	switch c.GetState() {
	case contract.States.INITIAL:
		cns = &ContractNegotiationInitial{Negotiation: c, stateMachineDeps: deps}
	case contract.States.REQUESTED:
		cns = &ContractNegotiationRequested{Negotiation: c, stateMachineDeps: deps}
	case contract.States.OFFERED:
		cns = &ContractNegotiationOffered{Negotiation: c, stateMachineDeps: deps}
	case contract.States.AGREED:
		cns = &ContractNegotiationAgreed{Negotiation: c, stateMachineDeps: deps}
	case contract.States.ACCEPTED:
		cns = &ContractNegotiationAccepted{Negotiation: c, stateMachineDeps: deps}
	case contract.States.VERIFIED:
		cns = &ContractNegotiationVerified{Negotiation: c, stateMachineDeps: deps}
	case contract.States.FINALIZED:
		cns = &ContractNegotiationFinalized{Negotiation: c, stateMachineDeps: deps}
	case contract.States.TERMINATED:
		cns = &ContractNegotiationTerminated{Negotiation: c, stateMachineDeps: deps}
	default:
		panic("Invalid contract state.")
	}
	return cns
}

func verifyAndTransform(
	ctx context.Context,
	cn ContractNegotiationState,
	providerPID, consumerPID, callbackAddress string,
	targetState contract.State,
	af applyFunc,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx, "target_state", targetState)
	if cn.GetProviderPID().URN() != strings.ToLower(providerPID) {
		return ctx, nil, ctxslog.ReturnError(ctx,
			"given provider PID didn't match contract provider PID",
			fmt.Errorf("given provider PID didn't match contract provider PID"),
			"given", providerPID,
			"existing", cn.GetProviderPID().URN(),
		)
	}
	if cn.GetConsumerPID().URN() != strings.ToLower(consumerPID) {
		return ctx, nil, ctxslog.ReturnError(ctx,
			"given consumer PID didn't match contract consumer PID",
			fmt.Errorf("given consumer PID didn't match contract negotation consumer PID"),
			"given", consumerPID,
			"existing", cn.GetConsumerPID().URN(),
		)
	}
	err := cn.SetCallback(callbackAddress)
	if err != nil {
		return ctx, nil, ctxslog.ReturnError(ctx, "invalid callback address", err, "callbackAddress", callbackAddress)
	}
	if err := cn.SetState(targetState); err != nil {
		return ctx, nil, ctxslog.ReturnError(ctx, "could not set state", err)
	}

	return ctx, af, nil
}

func processTermination(
	ctx context.Context, t shared.ContractNegotiationTerminationMessage, cn ContractNegotiationState,
) (context.Context, applyFunc, error) {
	ctx = ctxslog.With(ctx, "termination_code", t.Code)

	af := func() error {
		pid := cn.GetConsumerPID().String()
		if cn.GetContract().GetRole() == constants.DataspaceProvider {
			pid = cn.GetProviderPID().String()
		}
		reason := ""
		if len(t.Reason) > 0 {
			reason = t.Reason[0].Value
		}
		_, err := cn.GetContractService().TerminationReceived(ctx, &dsrpc.ContractServiceTerminationReceivedRequest{
			Pid:    pid,
			Code:   t.Code,
			Reason: []string{reason},
		})
		return err
	}
	if cn.AutoAccept() || cn.GetContractService() == nil {
		af = func() error { return nil }
	}
	return verifyAndTransform(
		ctx,
		cn,
		t.ProviderPID,
		t.ConsumerPID,
		cn.GetCallback().String(),
		contract.States.TERMINATED,
		af,
	)
}
