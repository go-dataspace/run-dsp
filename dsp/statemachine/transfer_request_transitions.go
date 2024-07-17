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
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/provider/v1"
	"github.com/google/uuid"
)

var ErrNotImplemented = errors.New("not implemented")

type TransferRequester interface {
	GetProviderPID() uuid.UUID
	GetConsumerPID() uuid.UUID
	GetAgreementID() uuid.UUID
	GetTarget() uuid.UUID
	GetFormat() string
	GetCallback() *url.URL
	GetSelf() *url.URL
	GetState() TransferRequestState
	GetRole() TransferRole
	SetState(state TransferRequestState) error
	GetTransferRequest() *TransferRequest
	GetPublishInfo() *providerv1.PublishInfo
	GetTransferDirection() TransferDirection
}

type TransferRequestNegotiationState interface {
	TransferRequester
	Recv(ctx context.Context, message any) (TransferRequestNegotiationState, error)
	Send(ctx context.Context) (func(), error)
	GetArchiver() Archiver
	GetProvider() providerv1.ProviderServiceClient
	GetRequester() Requester
}

type TransferRequestNegotiationInitial struct {
	*TransferRequest
	stateMachineDeps
}

func (tr *TransferRequestNegotiationInitial) Recv(
	ctx context.Context, message any,
) (TransferRequestNegotiationState, error) {
	switch t := message.(type) {
	case shared.TransferRequestMessage:
		_, err := tr.GetProvider().GetDataset(ctx, &providerv1.GetDatasetRequest{
			DatasetId: tr.GetTarget().String(),
		})
		if err != nil {
			return nil, fmt.Errorf("could not find target: %w", err)
		}
		tr.providerPID = uuid.New()
		return verifyAndTransformTransfer(
			ctx, tr, tr.providerPID.URN(), t.ConsumerPID, TransferRequestStates.TRANSFERREQUESTED)
	default:
		return nil, fmt.Errorf("invalid message type")
	}
}

func (tr *TransferRequestNegotiationInitial) Send(ctx context.Context) (func(), error) {
	return sendTransferRequest(ctx, tr)
}

type TransferRequestNegotiationRequested struct {
	*TransferRequest
	stateMachineDeps
}

func (tr *TransferRequestNegotiationRequested) Recv(
	ctx context.Context, message any,
) (TransferRequestNegotiationState, error) {
	switch t := message.(type) {
	case shared.TransferStartMessage:
		if tr.GetProviderPID() == emptyUUID {
			u, err := uuid.Parse(t.ProviderPID)
			if err != nil {
				return nil, fmt.Errorf("invalid UUID for provider PID: %w", err)
			}
			tr.providerPID = u
		}
		if tr.publishInfo == nil {
			var err error
			tr.publishInfo, err = dataAddressToPublishInfo(t.DataAddress)
			if err != nil {
				return nil, fmt.Errorf("invalid dataAddress supplied: %w", err)
			}
		}
		return verifyAndTransformTransfer(ctx, tr, t.ProviderPID, t.ConsumerPID, TransferRequestStates.STARTED)
	default:
		return nil, fmt.Errorf("invalid message type")
	}
}

func (tr *TransferRequestNegotiationRequested) Send(ctx context.Context) (func(), error) {
	switch tr.GetTransferDirection() {
	case DirectionPull:
		resp, err := tr.GetProvider().PublishDataset(ctx, &providerv1.PublishDatasetRequest{
			DatasetId: tr.GetTarget().String(),
			PublishId: tr.GetProviderPID().String(),
		})
		if err != nil {
			return func() {}, err
		}
		tr.publishInfo = resp.PublishInfo
	case DirectionPush:
		// TODO: Signal provider to start uploading dataset here.
		return func() {}, fmt.Errorf("push flow: %w", ErrNotImplemented)
	case DirectionUnknown:
		return func() {}, fmt.Errorf("unknown transfer direction")
	default:
		panic("unexpected statemachine.TransferDirection")
	}

	return sendTransferStart(ctx, tr)
}

type TransferRequestNegotiationStarted struct {
	*TransferRequest
	stateMachineDeps
}

func (tr *TransferRequestNegotiationStarted) Recv(
	ctx context.Context, message any,
) (TransferRequestNegotiationState, error) {
	switch t := message.(type) {
	case shared.TransferCompletionMessage:
		return verifyAndTransformTransfer(ctx, tr, t.ProviderPID, t.ConsumerPID, TransferRequestStates.COMPLETED)
	default:
		return nil, fmt.Errorf("invalid message type")
	}
}

func (tr *TransferRequestNegotiationStarted) Send(ctx context.Context) (func(), error) {
	switch tr.GetTransferDirection() {
	case DirectionPull:
		_, err := tr.GetProvider().UnpublishDataset(ctx, &providerv1.UnpublishDatasetRequest{
			PublishId: tr.GetProviderPID().String(),
		})
		if err != nil {
			return func() {}, err
		}
	case DirectionPush:
		// TODO: Signal provider to start uploading dataset here.
		return func() {}, fmt.Errorf("push flow: %w", ErrNotImplemented)
	case DirectionUnknown:
		return func() {}, fmt.Errorf("unknown transfer direction")
	default:
		panic("unexpected statemachine.TransferDirection")
	}
	return sendTransferCompletion(ctx, tr)
}

type TransferRequestNegotiationSuspended struct {
	*TransferRequest
	stateMachineDeps
}

type TransferRequestNegotiationCompleted struct {
	*TransferRequest
	stateMachineDeps
}

type TransferRequestNegotiationTerminated struct {
	*TransferRequest
	stateMachineDeps
}

func NewTransferRequest(
	ctx context.Context,
	store Archiver,
	provider providerv1.ProviderServiceClient,
	requester Requester,
	consumerPID, agreementID uuid.UUID,
	format string,
	callback, self *url.URL,
	role TransferRole,
	state TransferRequestState,
	publishInfo *providerv1.PublishInfo,
) (TransferRequestNegotiationState, error) {
	agreement, err := store.GetAgreement(ctx, agreementID)
	if err != nil {
		return nil, fmt.Errorf("no agreement found")
	}
	traReq := &TransferRequest{
		state:             state,
		consumerPID:       consumerPID,
		agreementID:       agreementID,
		target:            uuid.MustParse(agreement.Target),
		format:            format,
		callback:          callback,
		self:              self,
		role:              role,
		publishInfo:       publishInfo,
		transferDirection: DirectionPush,
	}
	if publishInfo == nil {
		traReq.transferDirection = DirectionPull
	}
	return GetTransferRequestNegotiation(store, traReq, provider, requester), nil
}

func GetTransferRequestNegotiation(
	a Archiver, tr *TransferRequest, p providerv1.ProviderServiceClient, r Requester,
) TransferRequestNegotiationState {
	deps := stateMachineDeps{a: a, p: p, r: r}
	switch tr.GetState() {
	case TransferRequestStates.TRANSFERINITIAL:
		return &TransferRequestNegotiationInitial{TransferRequest: tr, stateMachineDeps: deps}
	case TransferRequestStates.TRANSFERREQUESTED:
		return &TransferRequestNegotiationRequested{TransferRequest: tr, stateMachineDeps: deps}
	case TransferRequestStates.STARTED:
		return &TransferRequestNegotiationStarted{TransferRequest: tr, stateMachineDeps: deps}
	default:
		panic(fmt.Sprintf("No transition found for state %s", tr.GetState()))
	}
}

func dataAddressToPublishInfo(d shared.DataAddress) (*providerv1.PublishInfo, error) {
	p, err := makeEndpointPropertyMap(d.EndpointProperties)
	if err != nil {
		return nil, err
	}
	pi := &providerv1.PublishInfo{
		Url: d.Endpoint,
	}
	authType, ok := p["authType"]
	if !ok {
		return nil, fmt.Errorf("no authtype defined")
	}
	switch authType {
	case "bearer":
		pi.AuthenticationType = providerv1.AuthenticationType_AUTHENTICATION_TYPE_BEARER
		pi.Password = p["authorization"]
	case "basic":
		pi.AuthenticationType = providerv1.AuthenticationType_AUTHENTICATION_TYPE_BASIC
		pi.Username = p["username"]
		pi.Password = p["password"]
	default:
		return nil, fmt.Errorf("unsupported authentication type: %s", authType)
	}

	return pi, nil
}

func makeEndpointPropertyMap(p []shared.EndpointProperty) (map[string]string, error) {
	m := make(map[string]string)
	for _, e := range p {
		if e.Type != "dspace:EndpointProperty" {
			return nil, fmt.Errorf("invalid endpoint property")
		}
		m[e.Name] = e.Value
	}
	return m, nil
}

func verifyAndTransformTransfer(
	ctx context.Context,
	tr TransferRequestNegotiationState,
	providerPID, consumerPID string,
	targetState TransferRequestState,
) (TransferRequestNegotiationState, error) {
	if tr.GetProviderPID().URN() != strings.ToLower(providerPID) {
		return nil, fmt.Errorf(
			"given provider pid %s does not match transfer provider pid %s",
			providerPID,
			tr.GetProviderPID().URN(),
		)
	}
	if tr.GetConsumerPID().URN() != strings.ToLower(consumerPID) {
		return nil, fmt.Errorf(
			"given consumer pid %s does not match transfer consumer pid %s",
			providerPID,
			tr.GetProviderPID().URN(),
		)
	}
	if err := tr.SetState(targetState); err != nil {
		return nil, fmt.Errorf("could not set state: %w", err)
	}
	var err error
	if tr.GetRole() == TransferConsumer {
		err = tr.GetArchiver().PutConsumerTransfer(ctx, tr.GetTransferRequest())
	} else {
		err = tr.GetArchiver().PutProviderTransfer(ctx, tr.GetTransferRequest())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to save contract: %w", err)
	}
	return GetTransferRequestNegotiation(
		tr.GetArchiver(), tr.GetTransferRequest(), tr.GetProvider(), tr.GetRequester()), nil
}
