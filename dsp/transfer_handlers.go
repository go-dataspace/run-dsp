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

package dsp

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/internal/dspstatemachine"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/provider/v1"
	"github.com/google/uuid"
)

func getTransferProcessReq() shared.TransferProcess {
	return shared.TransferProcess{
		Context:     jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Type:        "dspace:TransferProcess",
		ProviderPID: "urn:uuid:a343fcbf-99fc-4ce8-8e9b-148c97605aab",
		ConsumerPID: "urn:uuid:32541fe6-c580-409e-85a8-8a9a32fbe833",
		State:       "dspace:REQUESTED",
	}
}

func (dh *dspHandlers) providerTransferProcessHandler(w http.ResponseWriter, req *http.Request) {
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, getTransferProcessReq())
}

func (dh *dspHandlers) providerTransferRequestHandler(w http.ResponseWriter, req *http.Request) {
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	transferReq, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferRequestMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	parts := strings.Split(transferReq.ConsumerPID, ":")
	uuidPart := parts[len(parts)-1]
	consumerPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Consumer PID is not a UUID")
		return
	}

	stateStorage := dspstatemachine.GetStateStorage(req.Context())
	agreement, err := stateStorage.FindAgreement(req.Context(), transferReq.AgreementID)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Consumer PID is not a UUID")
		return
	}

	providerPID := uuid.New()
	parts = strings.Split(agreement.Target, ":")
	uuidPart = parts[len(parts)-1]
	fileID := uuid.MustParse(uuidPart)

	resp, err := dh.provider.PublishDataset(req.Context(), &providerv1.PublishDatasetRequest{
		DatasetId: fileID.String(),
		PublishId: providerPID.String(),
	})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Consumer PID is not a UUID")
		return
	}
	publishInfo := processProviderPublishInfo(resp.PublishInfo)

	transferArgs := dspstatemachine.TransferArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:         dspstatemachine.Provider,
			ConsumerProcessId:       consumerPID,
			ProviderProcessId:       providerPID,
			ConsumerCallbackAddress: transferReq.CallbackAddress,
		},
		AgreementID:  transferReq.AgreementID,
		PublishInfo:  publishInfo,
		StateStorage: stateStorage,
	}

	transferProcess, err := dspstatemachine.ProviderCheckTransferRequestMessage(req.Context(), transferArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
		return
	}
	validateMarshalAndReturn(req.Context(), w, http.StatusCreated, transferProcess)
	go sendUUID(req.Context(), transferArgs.ProviderProcessId, dspstatemachine.Transfer)
}

func processProviderPublishInfo(ppi *providerv1.PublishInfo) shared.PublishInfo {
	return shared.PublishInfo{
		URL:   ppi.GetUrl(),
		Token: ppi.GetPassword(),
	}
}

func (dh *dspHandlers) providerTransferStartHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	start, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferStartMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer start", "start", start)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func (dh *dspHandlers) providerTransferCompletionHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPIDString := req.PathValue("providerPID")
	if providerPIDString == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	providerPID, err := uuid.Parse(providerPIDString)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Provider PID is not a UUID")
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	completion, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferCompletionMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer completion", "completion", completion)

	transferArgs := dspstatemachine.TransferArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:   dspstatemachine.Provider,
			ProviderProcessId: providerPID,
		},
		StateStorage: dspstatemachine.GetStateStorage(req.Context()),
	}

	_, err = dh.provider.UnpublishDataset(req.Context(), &providerv1.UnpublishDatasetRequest{
		PublishId: providerPID.String(),
	})
	if err != nil {
		returnError(w, http.StatusInternalServerError, "could not unpublish file")
		return
	}

	transferProcess, err := dspstatemachine.ProviderCheckTransferCompletionMessage(req.Context(), transferArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
		return
	}
	validateMarshalAndReturn(req.Context(), w, http.StatusCreated, transferProcess)
}

func (dh *dspHandlers) providerTransferTerminationHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	termination, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferTerminationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer termination", "termination", termination)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func (dh *dspHandlers) providerTransferSuspensionHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("providerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	suspension, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferSuspensionMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer suspension", "suspension", suspension)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func (dh *dspHandlers) consumerTransferStartHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPIDString := req.PathValue("consumerPID")
	if consumerPIDString == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	consumerPID, err := uuid.Parse(consumerPIDString)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Consumer PID is not a UUID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	start, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferStartMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer start from provider", "suspension", start)

	parts := strings.Split(start.ProviderPID, ":")
	uuidPart := parts[len(parts)-1]
	providerPID, err := uuid.Parse(uuidPart)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Provider PID is not a UUID")
		return
	}
	transferArgs := dspstatemachine.TransferArgs{
		BaseArgs: dspstatemachine.BaseArgs{
			ParticipantRole:   dspstatemachine.Provider,
			ConsumerProcessId: consumerPID,
			ProviderProcessId: providerPID,
		},
		StateStorage: dspstatemachine.GetStateStorage(req.Context()),
	}

	transferProcess, err := dspstatemachine.ConsumerCheckTransferStartMessage(req.Context(), transferArgs)
	if err != nil {
		returnError(w, http.StatusInternalServerError, err.Error())
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusCreated, transferProcess)
	// go sendUUID(req.Context(), transferArgs.ConsumerProcessId, dspstatemachine.Transfer)
}

func (dh *dspHandlers) consumerTransferCompletionHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	providerPID := req.PathValue("consumerPID")
	if providerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing provider PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	completion, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferCompletionMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer completion", "completion", completion)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func (dh *dspHandlers) consumerTransferTerminationHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("providerPID")
	if consumerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	termination, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferTerminationMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer termination", "termination", termination)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}

func (dh *dspHandlers) consumerTransferSuspensionHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	consumerPID := req.PathValue("providerPID")
	if consumerPID == "" {
		returnError(w, http.StatusBadRequest, "Missing consumer PID")
		return
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	suspension, err := shared.UnmarshalAndValidate(req.Context(), reqBody, shared.TransferSuspensionMessage{})
	if err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	logger.Debug("Got transfer suspension", "suspension", suspension)

	// If all goes well, we just return a 200
	w.WriteHeader(http.StatusOK)
}
