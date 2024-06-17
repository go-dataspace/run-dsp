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

package dspstatemachine

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
)

type httpTransferService struct {
	httpService
	TransferState DSPTransferStateStorage
}

func getHttpTransferService(ctx context.Context, transferState DSPTransferStateStorage) *httpTransferService {
	return &httpTransferService{
		httpService: httpService{
			Context: ctx,
		},
		TransferState: transferState,
	}
}

func (h *httpTransferService) ConsumerSendTransferRequest(ctx context.Context) error {
	logger := logging.Extract(ctx)
	logger.Debug("In ConsumerSendTransferRequest")

	transferRequest := shared.TransferRequestMessage{
		Context:         jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:            "dspace:TransferRequestMessage",
		AgreementID:     h.TransferState.AgreementID,
		Format:          "HTTP_PULL",
		CallbackAddress: h.TransferState.ConsumerCallbackAddress,
		ConsumerPID:     fmt.Sprintf("urn:uuid:%s", h.TransferState.ConsumerPID),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, transferRequest)
	if err != nil {
		return err
	}

	targetUrl := fmt.Sprintf("%s/transfers/request", h.TransferState.ProviderCallbackAddress)
	logger.Debug("Sending TransferRequest", "target_url", targetUrl, "transfer_request", transferRequest)
	responseBody, err := h.sendPostRequest(ctx, targetUrl, reqBody)
	if err != nil {
		return err
	}

	if len(responseBody) != 0 {
		transferProcess, err := shared.UnmarshalAndValidate(ctx, responseBody, shared.TransferProcess{})
		if err != nil {
			return err
		}

		logger.Debug("Got TransferProcess", "transfer_process", transferProcess)
		if transferProcess.State != "dspace:REQUESTED" {
			return errors.New("Invalid state returned")
		}
	}

	return nil
}

func (h *httpTransferService) ProviderSendTransferStartRequest(ctx context.Context) error {
	logger := logging.Extract(ctx)
	logger.Debug("In ProviderSendTransferStartRequest")

	transferStart := shared.TransferStartMessage{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:TransferStartMessage",
		ProviderPID: fmt.Sprintf("urn:uuid:%s", h.TransferState.ProviderPID),
		ConsumerPID: fmt.Sprintf("urn:uuid:%s", h.TransferState.ConsumerPID),
		DataAddress: shared.DataAddress{
			Type:         "dspace:DataAddress",
			EndpointType: "https://w3id.org/idsa/v4.1/HTTP",
			Endpoint:     h.TransferState.PublishInfo.URL,
			EndpointProperties: []shared.EndpointProperty{
				{
					Type:  "dspace:EndpointProperty",
					Name:  "authorization",
					Value: h.TransferState.PublishInfo.Token,
				},
				{
					Type:  "dspace:EndpointProperty",
					Name:  "authType",
					Value: "bearer",
				},
			},
		},
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, transferStart)
	if err != nil {
		return err
	}

	targetUrl := fmt.Sprintf("%s/transfers/%s/start", h.TransferState.ConsumerCallbackAddress, h.TransferState.ConsumerPID)
	logger.Debug("Sending TransferStart", "target_url", targetUrl, "transfer_start", transferStart)
	responseBody, err := h.sendPostRequest(ctx, targetUrl, reqBody)
	if err != nil {
		return err
	}

	if len(responseBody) != 0 {
		transferProcess, err := shared.UnmarshalAndValidate(ctx, responseBody, shared.TransferProcess{})
		if err != nil {
			return err
		}

		logger.Debug("Got TransferProcess", "transfer_process", transferProcess)
		if transferProcess.State != "dspace:STARTED" {
			return errors.New("Invalid state returned")
		}
	}

	return nil
}

//nolint:dupl
func (h *httpTransferService) ConsumerSendTransferCompleted(ctx context.Context) error {
	logger := logging.Extract(ctx)
	logger.Debug("In ConsumerSendTransferCompleted")

	transferCompletion := shared.TransferCompletionMessage{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:TransferCompletionMessage",
		ProviderPID: fmt.Sprintf("urn:uuid:%s", h.TransferState.ProviderPID),
		ConsumerPID: fmt.Sprintf("urn:uuid:%s", h.TransferState.ConsumerPID),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, transferCompletion)
	if err != nil {
		return err
	}

	targetUrl := fmt.Sprintf("%s/transfers/%s/completion",
		h.TransferState.ProviderCallbackAddress, h.TransferState.ProviderPID)
	logger.Debug("Sending TransferCompletionMessage", "target_url", targetUrl, "transfer_completion", transferCompletion)
	responseBody, err := h.sendPostRequest(ctx, targetUrl, reqBody)
	if err != nil {
		return err
	}

	if len(responseBody) != 0 {
		transferProcess, err := shared.UnmarshalAndValidate(ctx, responseBody, shared.TransferProcess{})
		if err != nil {
			return err
		}

		logger.Debug("Got TransferProcess", "transfer_process", transferProcess)
		if transferProcess.State != "dspace:COMPLETED" {
			return errors.New("Invalid state returned")
		}
	}

	return nil
}
