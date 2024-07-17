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

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/logging"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/provider/v1"
)

// TODO: dedup this and the other request function.
//
//nolint:dupl
func makeTransferRequestFunction(
	ctx context.Context,
	requester Requester,
	cu *url.URL,
	reqBody []byte,
	c *TransferRequest,
	a Archiver,
	destinationState TransferRequestState,
) func() {
	return func() {
		logger := logging.Extract(ctx)
		logger.Debug("Sending request")
		respBody, err := requester.SendHTTPRequest(ctx, "POST", cu, reqBody)
		if err != nil {
			logger.Error("Could not send request", "error", err)
			return
		}

		if len(respBody) > 0 {
			cneg, err := shared.UnmarshalAndValidate(ctx, respBody, shared.TransferProcess{})
			if err != nil {
				logger.Error("Could not parse response", "error", err)
				return
			}

			state, err := ParseTransferRequestState(cneg.State)
			if err != nil {
				logger.Error("Invalid state returned", "state", cneg.State)
				return
			}

			if state != destinationState {
				logger.Error("Invalid state returned", "state", state)
				return
			}
		}
		err = c.SetState(destinationState)
		if err != nil {
			logger.Error("Tried to set invalid state", "error", err)
			return
		}
		if c.role == TransferConsumer {
			err = a.PutConsumerTransfer(ctx, c)
		} else {
			err = a.PutProviderTransfer(ctx, c)
		}
		if err != nil {
			logger.Error("Could not store transfer request", "error", err)
			return
		}
	}
}

func sendTransferRequest(ctx context.Context, tr *TransferRequestNegotiationInitial) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendTransferRequest")
	transferRequest := shared.TransferRequestMessage{
		Context:         dspaceContext,
		Type:            "dspace:TransferRequestMessage",
		AgreementID:     tr.GetAgreementID().URN(),
		Format:          tr.GetFormat(),
		CallbackAddress: tr.GetSelf().String(),
		ConsumerPID:     tr.GetConsumerPID().URN(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, transferRequest)
	if err != nil {
		logger.Error("Could not validate contract request", "error", err)
		return func() {}, fmt.Errorf("could not process request: %w", err)
	}

	cu := cloneURL(tr.GetCallback())
	cu.Path = path.Join(cu.Path, "transfers", "request")

	return makeTransferRequestFunction(
		ctx, tr.GetRequester(), cu, reqBody, tr.GetTransferRequest(), tr.GetArchiver(),
		TransferRequestStates.TRANSFERREQUESTED,
	), nil
}

func sendTransferStart(ctx context.Context, tr *TransferRequestNegotiationRequested) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendTransferStarted")
	startRequest := shared.TransferStartMessage{
		Context:     dspaceContext,
		Type:        "dspace:TransferStartMessage",
		ProviderPID: tr.GetProviderPID().URN(),
		ConsumerPID: tr.GetConsumerPID().URN(),
		DataAddress: publishInfoToDataAddress(tr.GetPublishInfo()),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, startRequest)
	if err != nil {
		logger.Error("Could not validate start request", "error", err)
		return func() {}, fmt.Errorf("could not process request: %w", err)
	}

	pid := tr.GetConsumerPID().String()
	if tr.GetRole() == TransferConsumer {
		pid = tr.GetProviderPID().String()
	}
	cu := cloneURL(tr.GetCallback())
	cu.Path = path.Join(cu.Path, "transfers", pid, "start")

	return makeTransferRequestFunction(
		ctx, tr.GetRequester(), cu, reqBody, tr.GetTransferRequest(), tr.GetArchiver(),
		TransferRequestStates.STARTED,
	), nil
}

func sendTransferCompletion(ctx context.Context, tr *TransferRequestNegotiationStarted) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendTransferCompletion")
	startRequest := shared.TransferCompletionMessage{
		Context:     dspaceContext,
		Type:        "dspace:TransferCompletionMessage",
		ProviderPID: tr.GetProviderPID().URN(),
		ConsumerPID: tr.GetConsumerPID().URN(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, startRequest)
	if err != nil {
		logger.Error("Could not validate start request", "error", err)
		return func() {}, fmt.Errorf("could not process request: %w", err)
	}

	pid := tr.GetConsumerPID().String()
	if tr.GetRole() == TransferConsumer {
		pid = tr.GetProviderPID().String()
	}

	cu := cloneURL(tr.GetCallback())
	cu.Path = path.Join(cu.Path, "transfers", pid, "completion")

	return makeTransferRequestFunction(
		ctx, tr.GetRequester(), cu, reqBody, tr.GetTransferRequest(), tr.GetArchiver(),
		TransferRequestStates.COMPLETED,
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
		panic(fmt.Sprintf("unexpected providerv1.AuthenticationType: %#v", pi.AuthenticationType))
	default:
		panic(fmt.Sprintf("unexpected providerv1.AuthenticationType: %#v", pi.AuthenticationType))
	}
	return da
}
