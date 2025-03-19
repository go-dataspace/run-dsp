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

	"github.com/google/uuid"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/transfer"
	provider "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

var ErrNotImplemented = errors.New("not implemented")

type TransferRequester interface {
	GetProviderPID() uuid.UUID
	GetConsumerPID() uuid.UUID
	GetAgreementID() uuid.UUID
	GetTarget() string
	GetFormat() string
	GetCallback() *url.URL
	GetSelf() *url.URL
	GetState() transfer.State
	GetRole() constants.DataspaceRole
	SetState(state transfer.State) error
	GetTransferRequest() *transfer.Request
	GetPublishInfo() *provider.PublishInfo
	GetTransferDirection() transfer.Direction
	GetTransferProcess() shared.TransferProcess
}

type TransferRequestNegotiationState interface {
	TransferRequester
	Recv(ctx context.Context, message any) (TransferRequestNegotiationState, error)
	Send(ctx context.Context) (func(), error)
	GetProvider() provider.ProviderServiceClient
	GetReconciler() Reconciler
}

type TransferRequestNegotiationInitial struct {
	*transfer.Request
	stateMachineDeps
}

func (tr *TransferRequestNegotiationInitial) Recv(
	ctx context.Context, message any,
) (TransferRequestNegotiationState, error) {
	switch t := message.(type) {
	case shared.TransferRequestMessage:
		_, err := tr.GetProvider().GetDataset(ctx, &provider.GetDatasetRequest{
			DatasetId: tr.GetTarget(),
		})
		if err != nil {
			return nil, fmt.Errorf("could not find target: %w", err)
		}
		tr.SetProviderPID(uuid.New())
		return verifyAndTransformTransfer(
			tr, tr.GetProviderPID().URN(), t.ConsumerPID, transfer.States.REQUESTED)
	default:
		return nil, fmt.Errorf("invalid message type")
	}
}

func (tr *TransferRequestNegotiationInitial) Send(ctx context.Context) (func(), error) {
	return sendTransferRequest(ctx, tr)
}

type TransferRequestNegotiationRequested struct {
	*transfer.Request
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
			tr.SetProviderPID(u)
		}
		if tr.GetPublishInfo() == nil {
			var err error
			pi, err := dataAddressToPublishInfo(t.DataAddress)
			if err != nil {
				return nil, fmt.Errorf("invalid dataAddress supplied: %w", err)
			}
			tr.SetPublishInfo(pi)
		}
		return verifyAndTransformTransfer(tr, t.ProviderPID, t.ConsumerPID, transfer.States.STARTED)
	case shared.TransferTerminationMessage:
		return verifyAndTransformTransfer(tr, t.ProviderPID, t.ConsumerPID, transfer.States.TERMINATED)
	default:
		return nil, fmt.Errorf("invalid message type")
	}
}

func (tr *TransferRequestNegotiationRequested) Send(ctx context.Context) (func(), error) {
	switch tr.GetTransferDirection() {
	case transfer.DirectionPull:
		resp, err := tr.GetProvider().PublishDataset(ctx, &provider.PublishDatasetRequest{
			DatasetId: tr.GetTarget(),
			PublishId: tr.GetProviderPID().String(),
		})
		if err != nil {
			return func() {}, err
		}
		tr.SetPublishInfo(resp.PublishInfo)
	case transfer.DirectionPush:
		// TODO: Signal provider to start uploading dataset here.
		return func() {}, fmt.Errorf("push flow: %w", ErrNotImplemented)
	case transfer.DirectionUnknown:
		return func() {}, fmt.Errorf("unknown transfer direction")
	default:
		panic("unexpected statemachine.TransferDirection")
	}

	return sendTransferStart(ctx, tr)
}

type TransferRequestNegotiationStarted struct {
	*transfer.Request
	stateMachineDeps
}

func (tr *TransferRequestNegotiationStarted) Recv(
	ctx context.Context, message any,
) (TransferRequestNegotiationState, error) {
	switch t := message.(type) {
	case shared.TransferCompletionMessage:
		err := unpublishTransfer(ctx, tr)
		if err != nil {
			return nil, err
		}
		return verifyAndTransformTransfer(tr, t.ProviderPID, t.ConsumerPID, transfer.States.COMPLETED)
	case shared.TransferTerminationMessage:
		return verifyAndTransformTransfer(tr, t.ProviderPID, t.ConsumerPID, transfer.States.TERMINATED)
	default:
		return nil, fmt.Errorf("invalid message type")
	}
}

func (tr *TransferRequestNegotiationStarted) Send(ctx context.Context) (func(), error) {
	err := unpublishTransfer(ctx, tr)
	if err != nil {
		return func() {}, err
	}
	return sendTransferCompletion(ctx, tr)
}

func unpublishTransfer(ctx context.Context, tr TransferRequestNegotiationState) error {
	switch tr.GetTransferDirection() {
	case transfer.DirectionPull:
		if tr.GetRole() == constants.DataspaceProvider {
			_, err := tr.GetProvider().UnpublishDataset(ctx, &provider.UnpublishDatasetRequest{
				PublishId: tr.GetProviderPID().String(),
			})
			if err != nil {
				return err
			}
		}
	case transfer.DirectionPush:
		return fmt.Errorf("push flow: %w", ErrNotImplemented)
	case transfer.DirectionUnknown:
		return fmt.Errorf("unknown transfer direction")
	default:
		panic("unexpected statemachine.TransferDirection")
	}
	return nil
}

type TransferRequestNegotiationSuspended struct {
	*transfer.Request
	stateMachineDeps
}

type TransferRequestNegotiationCompleted struct {
	*transfer.Request
	stateMachineDeps
}

func (tr *TransferRequestNegotiationCompleted) Recv(
	ctx context.Context, message any,
) (TransferRequestNegotiationState, error) {
	return nil, fmt.Errorf("this is a final state")
}

func (tr *TransferRequestNegotiationCompleted) Send(ctx context.Context) (func(), error) {
	return func() {}, nil
}

type TransferRequestNegotiationTerminated struct {
	*transfer.Request
	stateMachineDeps
}

func (tr *TransferRequestNegotiationTerminated) Recv(
	ctx context.Context, message any,
) (TransferRequestNegotiationState, error) {
	return nil, fmt.Errorf("this is a final state")
}

func (tr *TransferRequestNegotiationTerminated) Send(ctx context.Context) (func(), error) {
	return func() {}, nil
}

func GetTransferRequestNegotiation(
	tr *transfer.Request, p provider.ProviderServiceClient, r Reconciler,
) TransferRequestNegotiationState {
	deps := stateMachineDeps{p: p, r: r}
	switch tr.GetState() {
	case transfer.States.INITIAL:
		return &TransferRequestNegotiationInitial{Request: tr, stateMachineDeps: deps}
	case transfer.States.REQUESTED:
		return &TransferRequestNegotiationRequested{Request: tr, stateMachineDeps: deps}
	case transfer.States.STARTED:
		return &TransferRequestNegotiationStarted{Request: tr, stateMachineDeps: deps}
	case transfer.States.COMPLETED:
		return &TransferRequestNegotiationCompleted{Request: tr, stateMachineDeps: deps}
	case transfer.States.TERMINATED:
		return &TransferRequestNegotiationTerminated{Request: tr, stateMachineDeps: deps}
	default:
		panic(fmt.Sprintf("No transition found for state %s", tr.GetState()))
	}
}

func dataAddressToPublishInfo(d shared.DataAddress) (*provider.PublishInfo, error) {
	p, err := makeEndpointPropertyMap(d.EndpointProperties)
	if err != nil {
		return nil, err
	}
	pi := &provider.PublishInfo{
		Url: d.Endpoint,
	}

	authType, ok := p["authType"]
	if !ok {
		return pi, nil
	}
	switch authType {
	case "bearer":
		pi.AuthenticationType = provider.AuthenticationType_AUTHENTICATION_TYPE_BEARER
		pi.Password = p["authorization"]
	case "basic":
		pi.AuthenticationType = provider.AuthenticationType_AUTHENTICATION_TYPE_BASIC
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
	tr TransferRequestNegotiationState,
	providerPID, consumerPID string,
	targetState transfer.State,
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
	return GetTransferRequestNegotiation(
		tr.GetTransferRequest(), tr.GetProvider(), tr.GetReconciler()), nil
}
