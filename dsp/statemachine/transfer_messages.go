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
	"path"
	"strings"

	"github.com/go-dataspace/run-dsp/dsp/constants"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/transfer"
	"github.com/go-dataspace/run-dsp/logging"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
	"github.com/google/uuid"
)

func makeTransferRequestFunction(
	ctx context.Context,
	t *transfer.Request,
	cu *url.URL,
	reqBody []byte,
	destinationState transfer.State,
	reconciler *Reconciler,
) func() {
	var id uuid.UUID
	if t.GetRole() == constants.DataspaceConsumer {
		id = t.GetConsumerPID()
	} else {
		id = t.GetProviderPID()
	}
	return makeRequestFunction(
		ctx,
		cu,
		reqBody,
		id,
		t.GetRole(),
		destinationState.String(),
		ReconciliationTransferRequest,
		reconciler,
	)
}

func sendTransferRequest(ctx context.Context, tr *TransferRequestNegotiationInitial) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendTransferRequest")
	transferRequest := shared.TransferRequestMessage{
		Context:         shared.GetDSPContext(),
		Type:            "dspace:TransferRequestMessage",
		AgreementID:     tr.GetAgreementID().URN(),
		Format:          tr.GetFormat(),
		CallbackAddress: tr.GetSelf().String(),
		ConsumerPID:     tr.GetConsumerPID().URN(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, transferRequest)
	if err != nil {
		logger.Error("Could not validate contract request", "err", err)
		return func() {}, fmt.Errorf("could not process request: %w", err)
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
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendTransferStarted")
	startRequest := shared.TransferStartMessage{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:TransferStartMessage",
		ProviderPID: tr.GetProviderPID().URN(),
		ConsumerPID: tr.GetConsumerPID().URN(),
		DataAddress: publishInfoToDataAddress(tr.GetPublishInfo()),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, startRequest)
	if err != nil {
		logger.Error("Could not validate start request", "err", err)
		return func() {}, fmt.Errorf("could not process request: %w", err)
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
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendTransferCompletion")
	startRequest := shared.TransferCompletionMessage{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:TransferCompletionMessage",
		ProviderPID: tr.GetProviderPID().URN(),
		ConsumerPID: tr.GetConsumerPID().URN(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, startRequest)
	if err != nil {
		logger.Error("Could not validate start request", "err", err)
		return func() {}, fmt.Errorf("could not process request: %w", err)
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

func publishInfoToDataAddress(pi *providerv1.PublishInfo) shared.DataAddress {
	da := shared.DataAddress{
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
	case providerv1.AuthenticationType_AUTHENTICATION_TYPE_BASIC:
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
	case providerv1.AuthenticationType_AUTHENTICATION_TYPE_BEARER:
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
	case providerv1.AuthenticationType_AUTHENTICATION_TYPE_UNSPECIFIED:
		da.EndpointProperties = []shared.EndpointProperty{}
	default:
		panic(fmt.Sprintf("unexpected providerv1.AuthenticationType: %#v", pi.AuthenticationType))
	}
	return da
}
