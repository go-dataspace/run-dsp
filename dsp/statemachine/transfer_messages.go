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
	"path"
	"strings"

	"github.com/google/uuid"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/transfer"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

func makeTransferRequestFunction(
	ctx context.Context,
	t *transfer.Request,
	cu *url.URL,
	reqBody []byte,
	destinationState transfer.State,
	reconciler Reconciler,
) func() {
	var id uuid.UUID
	if t.GetRole() == constants.DataspaceConsumer {
		id = t.GetConsumerPID()
	} else {
		id = t.GetProviderPID()
	}
	return func() {
		f := makeRequestFunction(
			ctx,
			cu,
			reqBody,
			id,
			t.GetRole(),
			destinationState.String(),
			ReconciliationTransferRequest,
			reconciler,
		)
		err := f()
		if err != nil {
			panic(err.Error())
		}
	}
}

func sendTransferRequest(ctx context.Context, tr *TransferRequestNegotiationInitial) (func(), error) {
	ctx = ctxslog.With(ctx, "operation", "sendTransferRequest")
	transferRequest := shared.TransferRequestMessage{
		Context:         shared.GetDSPContext(),
		Type:            "dspace:TransferRequestMessage",
		AgreementID:     tr.GetAgreementID().URN(),
		Format:          tr.GetFormat(),
		CallbackAddress: tr.GetSelf().String(),
		ConsumerPID:     tr.GetConsumerPID().URN(),
	}
	if tr.GetTransferDirection() == transfer.DirectionPush {
		ctxslog.Debug(ctx, "Push transfer request, trying to add dataddress")
		if tr.GetPublishInfo() == nil {
			return func() {}, errors.New("Push transfer request without publishinfo attempted")
		}
		transferRequest.DataAddress = publishInfoToDataAddress(tr.GetPublishInfo())
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, transferRequest)
	if err != nil {
		return func() {}, ctxslog.ReturnError(ctx, "could not process request", err)
	}

	cu := cloneURL(tr.GetCallback())
	cu.Path = path.Join(cu.Path, "transfers", "request")

	return makeTransferRequestFunction(
		ctx,
		tr.GetTransferRequest(),
		cu,
		reqBody,
		transfer.States.REQUESTED,
		tr.GetReconciler(),
	), nil
}

func sendTransferStart(ctx context.Context, tr *TransferRequestNegotiationRequested) (func(), error) {
	ctx = ctxslog.With(ctx, "operation", "sendTransferStarted")
	startRequest := shared.TransferStartMessage{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:TransferStartMessage",
		ProviderPID: tr.GetProviderPID().URN(),
		ConsumerPID: tr.GetConsumerPID().URN(),
		DataAddress: publishInfoToDataAddress(tr.GetPublishInfo()),
	}
	if tr.GetTransferDirection() == transfer.DirectionPull {
		ctxslog.Debug(ctx, "Pull transfer start, trying to add dataddress")
		if tr.GetPublishInfo() == nil {
			return func() {}, errors.New("Pull transfer start without publishinfo attempted")
		}
		startRequest.DataAddress = publishInfoToDataAddress(tr.GetPublishInfo())
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, startRequest)
	if err != nil {
		return func() {}, ctxslog.ReturnError(ctx, "could not process request", err)
	}

	pid := tr.GetConsumerPID().String()
	if tr.GetRole() == constants.DataspaceConsumer {
		pid = tr.GetProviderPID().String()
	}
	cu := cloneURL(tr.GetCallback())
	cu.Path = path.Join(cu.Path, "transfers", pid, "start")

	return makeTransferRequestFunction(
		ctx,
		tr.GetTransferRequest(),
		cu,
		reqBody,
		transfer.States.STARTED,
		tr.GetReconciler(),
	), nil
}

func sendTransferCompletion(ctx context.Context, tr *TransferRequestNegotiationStarted) (func(), error) {
	ctx = ctxslog.With(ctx, "operation", "sendTransferCompletion")
	startRequest := shared.TransferCompletionMessage{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:TransferCompletionMessage",
		ProviderPID: tr.GetProviderPID().URN(),
		ConsumerPID: tr.GetConsumerPID().URN(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, startRequest)
	if err != nil {
		return func() {}, ctxslog.ReturnError(ctx, "could not process request", err)
	}

	pid := tr.GetConsumerPID().String()
	if tr.GetRole() == constants.DataspaceConsumer {
		pid = tr.GetProviderPID().String()
	}

	cu := cloneURL(tr.GetCallback())
	cu.Path = path.Join(cu.Path, "transfers", pid, "completion")

	return makeTransferRequestFunction(
		ctx,
		tr.GetTransferRequest(),
		cu,
		reqBody,
		transfer.States.COMPLETED,
		tr.GetReconciler(),
	), nil
}

func publishInfoToDataAddress(pi *dsrpc.PublishInfo) *shared.DataAddress {
	da := &shared.DataAddress{
		Type:               "dspace:DataAddress",
		Endpoint:           pi.Url,
		EndpointProperties: []shared.EndpointProperty{},
	}
	if strings.HasPrefix(pi.Url, "https://") {
		da.EndpointType = "https://w3id.org/idsa/v4.1/HTTPS"
	}
	if strings.HasPrefix(pi.Url, "http://") {
		da.EndpointType = "https://w3id.org/idsa/v4.1/HTTP"
	}
	switch pi.AuthenticationType {
	case dsrpc.AuthenticationType_AUTHENTICATION_TYPE_BASIC:
		da.EndpointProperties = []shared.EndpointProperty{
			{
				Type:  "dspace:EndpointProperty",
				Name:  "username",
				Value: pi.Username,
			},
			{
				Type:  "dspace:EndpointProperty",
				Name:  "password",
				Value: pi.Password,
			},
			{
				Type:  "dspace:EndpointProperty",
				Name:  "authType",
				Value: "basic",
			},
		}
	case dsrpc.AuthenticationType_AUTHENTICATION_TYPE_BEARER:
		da.EndpointProperties = []shared.EndpointProperty{
			{
				Type:  "dspace:EndpointProperty",
				Name:  "authorization",
				Value: pi.Password,
			},
			{
				Type:  "dspace:EndpointProperty",
				Name:  "authType",
				Value: "bearer",
			},
		}
	case dsrpc.AuthenticationType_AUTHENTICATION_TYPE_UNSPECIFIED:
		da.EndpointProperties = []shared.EndpointProperty{}
	default:
		panic(fmt.Sprintf("unexpected providerv1.AuthenticationType: %#v", pi.AuthenticationType))
	}
	return da
}
