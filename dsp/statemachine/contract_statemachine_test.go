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

package statemachine_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/logging"
	mockprovider "github.com/go-dataspace/run-dsp/mocks/github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
	"github.com/go-dataspace/run-dsp/odrl"
	providerv1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRequester struct {
	ReceivedMethod string
	ReceivedURL    *url.URL
	ReceivedBody   []byte
	Response       []byte
}

func urlMustParse(u string) *url.URL {
	nu, err := url.Parse(u)
	if err != nil {
		panic("bad url")
	}
	return nu
}

func (mr *MockRequester) SendHTTPRequest(
	ctx context.Context, method string, url *url.URL, reqBody []byte,
) ([]byte, error) {
	mr.ReceivedMethod = method
	mr.ReceivedURL = url
	mr.ReceivedBody = reqBody
	return mr.Response, nil
}

func decode[T any](d []byte) (T, error) {
	var m T
	err := json.Unmarshal(d, &m)
	return m, err
}

const (
	reconcileWait = 1 * time.Second
)

var (
	target           = uuid.MustParse("68d3d534-06b9-4700-9890-915bc32ecb75")
	consumerPID      = uuid.MustParse("d6bc4c28-973b-4c2f-b63f-08076c4fc65e")
	providerPID      = uuid.MustParse("76e705bb-cd5a-49f3-99c2-cec1406c8e9e")
	providerCallback = urlMustParse("https://provider.dsp/")
	consumerCallback = urlMustParse("https://consumer.dsp/callback/")
	provInitCB       = urlMustParse("https://consumer.dsp/")

	publishURL = "https://example.org/publish-here.pdf"
	token      = "some-test-token"
)

// TestStateMachinesConsumerInitPull tests a whole statemachine run, this will do the happy path,
// acting like the consumer initiated it. Once the contract state machine has successfully completed,
// it will do a pull transfer request.
// TODO: This is very unreadable, clean it up.
//
//nolint:funlen,maintidx
func TestStateMachineConsumerInitConsumerPull(t *testing.T) {
	t.Parallel()

	offer := odrl.Offer{
		MessageOffer: odrl.MessageOffer{
			PolicyClass: odrl.PolicyClass{
				AbstractPolicyRule: odrl.AbstractPolicyRule{},
				ID:                 uuid.New().URN(),
			},
			Type:   "odrl:Offer",
			Target: target.URN(),
		},
	}

	logger := logging.NewJSON("error", true)
	ctx := logging.Inject(context.Background(), logger)
	ctx, done := context.WithCancel(ctx)
	defer done()

	store := statemachine.NewMemoryArchiver()
	requester := &MockRequester{}

	mockProvider := mockprovider.NewMockProviderServiceClient(t)
	mockProvider.On("GetDataset", mock.Anything, &providerv1.GetDatasetRequest{
		DatasetId: target.String(),
	}).Return(&providerv1.GetDatasetResponse{
		Dataset: &providerv1.Dataset{},
	}, nil)

	reconciler := statemachine.NewReconciler(ctx, requester, store)
	reconciler.Run()

	ctx, consumerInit, err := statemachine.NewContract(
		ctx, store, mockProvider, reconciler, uuid.UUID{}, consumerPID,
		statemachine.ContractStates.INITIAL, offer, providerCallback, consumerCallback, statemachine.DataspaceConsumer)
	assert.Nil(t, err)
	assert.Nil(t, consumerInit.GetArchiver().PutConsumerContract(ctx, consumerInit.GetContract()))
	apply, err := consumerInit.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	consumerInitRes, err := store.GetConsumerContract(ctx, consumerPID)
	validateContract(t, err, consumerInitRes, statemachine.ContractStates.REQUESTED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t, providerCallback.String()+"negotiations/request", requester.ReceivedURL.String())

	reqMSG, err := decode[shared.ContractRequestMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, consumerPID.URN(), reqMSG.ConsumerPID)
	assert.Equal(t, consumerCallback.String(), reqMSG.CallbackAddress)
	assert.Equal(t, target.URN(), reqMSG.Offer.Target)

	ctx, providerInit, err := statemachine.NewContract(
		ctx, store, mockProvider, reconciler, uuid.UUID{}, uuid.MustParse(reqMSG.ConsumerPID),
		statemachine.ContractStates.INITIAL, odrl.Offer{MessageOffer: reqMSG.Offer},
		urlMustParse(reqMSG.CallbackAddress), providerCallback, statemachine.DataspaceProvider,
	)
	assert.Nil(t, err)

	ctx, nextProvider, err := providerInit.Recv(ctx, reqMSG)
	validateContract(t, err, nextProvider.GetContract(), statemachine.ContractStates.REQUESTED, false)

	gProviderPID := nextProvider.GetProviderPID()

	apply, err = nextProvider.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	providerReqRes, err := store.GetProviderContract(ctx, gProviderPID)
	validateContract(t, err, providerReqRes, statemachine.ContractStates.OFFERED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		consumerCallback.String()+"negotiations/"+consumerPID.String()+"/offers",
		requester.ReceivedURL.String())

	offerMSG, err := decode[shared.ContractOfferMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, consumerPID.URN(), offerMSG.ConsumerPID)
	assert.Equal(t, gProviderPID.URN(), offerMSG.ProviderPID)
	assert.Equal(t, providerCallback.String(), offerMSG.CallbackAddress)
	assert.Equal(t, target.URN(), offerMSG.Offer.Target)

	consContract, err := store.GetConsumerContract(ctx, uuid.MustParse(offerMSG.ConsumerPID))
	validateContract(t, err, consContract, statemachine.ContractStates.REQUESTED, false)

	ctx, cState := statemachine.GetContractNegotiation(ctx, store, consContract, mockProvider, reconciler)
	ctx, cNext, err := cState.Recv(ctx, offerMSG)
	assert.Nil(t, err)
	assert.Equal(t, statemachine.ContractStates.OFFERED, cNext.GetState())
	assert.Equal(t, providerCallback, cNext.GetCallback())
	assert.Equal(t, target.URN(), cNext.GetOffer().MessageOffer.Target)
	assert.Equal(t, consumerCallback, cNext.GetSelf())

	apply, err = cNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	cAcceptRes, err := store.GetConsumerContract(ctx, consumerPID)
	validateContract(t, err, cAcceptRes, statemachine.ContractStates.ACCEPTED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		providerCallback.String()+"negotiations/"+gProviderPID.String()+"/events",
		requester.ReceivedURL.String())

	acceptMessage, err := decode[shared.ContractNegotiationEventMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, consumerPID.URN(), acceptMessage.ConsumerPID)
	assert.Equal(t, gProviderPID.URN(), acceptMessage.ProviderPID)
	assert.Equal(t, "dspace:ACCEPTED", acceptMessage.EventType)

	provContract, err := store.GetProviderContract(ctx, uuid.MustParse(acceptMessage.ProviderPID))
	validateContract(t, err, provContract, statemachine.ContractStates.OFFERED, false)

	ctx, pState := statemachine.GetContractNegotiation(ctx, store, provContract, mockProvider, reconciler)
	ctx, pNext, err := pState.Recv(ctx, acceptMessage)
	validateContract(t, err, pNext.GetContract(), statemachine.ContractStates.ACCEPTED, false)

	apply, err = pNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	pAgreeRes, err := store.GetProviderContract(ctx, gProviderPID)
	validateContract(t, err, pAgreeRes, statemachine.ContractStates.AGREED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		consumerCallback.String()+"negotiations/"+consumerPID.String()+"/agreement",
		requester.ReceivedURL.String())

	agreeMSG, err := decode[shared.ContractAgreementMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, consumerPID.URN(), agreeMSG.ConsumerPID)
	assert.Equal(t, gProviderPID.URN(), agreeMSG.ProviderPID)
	assert.Equal(t, providerCallback.String(), agreeMSG.CallbackAddress)
	assert.Equal(t, target.URN(), agreeMSG.Agreement.Target)

	gAgreement := agreeMSG.Agreement

	cContract, err := store.GetConsumerContract(ctx, uuid.MustParse(acceptMessage.ConsumerPID))
	validateContract(t, err, cContract, statemachine.ContractStates.ACCEPTED, false)

	ctx, cState = statemachine.GetContractNegotiation(ctx, store, cContract, mockProvider, reconciler)
	ctx, cNext, err = cState.Recv(ctx, agreeMSG)
	validateContract(t, err, cNext.GetContract(), statemachine.ContractStates.AGREED, false)

	apply, err = cNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	cVerRes, err := store.GetConsumerContract(ctx, consumerPID)
	validateContract(t, err, cVerRes, statemachine.ContractStates.VERIFIED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		providerCallback.String()+"negotiations/"+gProviderPID.String()+"/agreement/verification",
		requester.ReceivedURL.String())

	verMSG, err := decode[shared.ContractAgreementVerificationMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, consumerPID.URN(), verMSG.ConsumerPID)
	assert.Equal(t, gProviderPID.URN(), verMSG.ProviderPID)

	pContract, err := store.GetProviderContract(ctx, uuid.MustParse(acceptMessage.ProviderPID))
	validateContract(t, err, pContract, statemachine.ContractStates.AGREED, false)

	ctx, pState = statemachine.GetContractNegotiation(ctx, store, pContract, mockProvider, reconciler)
	ctx, pNext, err = pState.Recv(ctx, verMSG)
	validateContract(t, err, pNext.GetContract(), statemachine.ContractStates.VERIFIED, false)

	apply, err = pNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	pFinRes, err := store.GetProviderContract(ctx, gProviderPID)
	validateContract(t, err, pFinRes, statemachine.ContractStates.FINALIZED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		consumerCallback.String()+"negotiations/"+consumerPID.String()+"/events",
		requester.ReceivedURL.String())

	finMSG, err := decode[shared.ContractNegotiationEventMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, consumerPID.URN(), finMSG.ConsumerPID)
	assert.Equal(t, gProviderPID.URN(), finMSG.ProviderPID)
	assert.Equal(t, "dspace:FINALIZED", finMSG.EventType)

	cContract, err = store.GetConsumerContract(ctx, uuid.MustParse(finMSG.ConsumerPID))
	validateContract(t, err, cContract, statemachine.ContractStates.VERIFIED, false)

	ctx, cState = statemachine.GetContractNegotiation(ctx, store, cContract, mockProvider, reconciler)
	ctx, cNext, err = cState.Recv(ctx, finMSG)
	validateContract(t, err, cNext.GetContract(), statemachine.ContractStates.FINALIZED, false)

	agreementID := uuid.MustParse(gAgreement.ID)

	trCPID := uuid.New()
	cTransInit, err := statemachine.NewTransferRequest(
		ctx, store, mockProvider, reconciler,
		trCPID, agreementID, "HTTP_PULL",
		providerCallback, consumerCallback, statemachine.DataspaceConsumer,
		statemachine.TransferRequestStates.TRANSFERINITIAL, nil,
	)
	assert.Nil(t, err)
	apply, err = cTransInit.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	cTransInitRes, err := store.GetConsumerTransfer(ctx, trCPID)
	validateTransfer(t, err, cTransInitRes, statemachine.TransferRequestStates.TRANSFERREQUESTED, agreementID)
	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t, providerCallback.String()+"transfers/request", requester.ReceivedURL.String())

	trReqMSG, err := decode[shared.TransferRequestMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, trCPID.URN(), trReqMSG.ConsumerPID)
	assert.Equal(t, agreementID.URN(), trReqMSG.AgreementID)
	assert.Equal(t, "HTTP_PULL", trReqMSG.Format)
	assert.Equal(t, consumerCallback.String(), trReqMSG.CallbackAddress)

	pTransInit, err := statemachine.NewTransferRequest(
		ctx, store, mockProvider, reconciler,
		uuid.MustParse(trReqMSG.ConsumerPID), uuid.MustParse(trReqMSG.AgreementID), trReqMSG.Format,
		urlMustParse(trReqMSG.CallbackAddress), providerCallback, statemachine.DataspaceProvider,
		statemachine.TransferRequestStates.TRANSFERINITIAL, nil,
	)
	assert.Nil(t, err)
	pTransNext, err := pTransInit.Recv(ctx, trReqMSG)
	validateTransfer(
		t, err, pTransNext.GetTransferRequest(), statemachine.TransferRequestStates.TRANSFERREQUESTED, agreementID)

	trProviderPID := pTransNext.GetProviderPID()
	mockProvider.On("PublishDataset", mock.Anything, &providerv1.PublishDatasetRequest{
		DatasetId: target.String(),
		PublishId: trProviderPID.String(),
	}).Return(&providerv1.PublishDatasetResponse{
		PublishInfo: &providerv1.PublishInfo{
			Url:                publishURL,
			AuthenticationType: providerv1.AuthenticationType_AUTHENTICATION_TYPE_BEARER,
			Username:           "",
			Password:           token,
		},
	}, nil)

	apply, err = pTransNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	pTransRes, err := store.GetProviderTransfer(ctx, trProviderPID)
	validateTransfer(t, err, pTransRes, statemachine.TransferRequestStates.STARTED, agreementID)
	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t, consumerCallback.String()+"transfers/"+trCPID.String()+"/start", requester.ReceivedURL.String())

	trStartMSG, err := decode[shared.TransferStartMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, trProviderPID.URN(), trStartMSG.ProviderPID)
	assert.Equal(t, trCPID.URN(), trStartMSG.ConsumerPID)
	assert.Equal(t, publishURL, trStartMSG.DataAddress.Endpoint)
	assert.Contains(t, trStartMSG.DataAddress.EndpointProperties, shared.EndpointProperty{
		Type:  "dspace:EndpointProperty",
		Name:  "authorization",
		Value: token,
	})
	assert.Contains(t, trStartMSG.DataAddress.EndpointProperties, shared.EndpointProperty{
		Type:  "dspace:EndpointProperty",
		Name:  "authType",
		Value: "bearer",
	})

	cTraReq, err := store.GetConsumerTransfer(ctx, uuid.MustParse(trStartMSG.ConsumerPID))
	validateTransfer(t, err, cTraReq, statemachine.TransferRequestStates.TRANSFERREQUESTED, agreementID)

	cTransInit = statemachine.GetTransferRequestNegotiation(store, cTraReq, mockProvider, reconciler)
	cTransNext, err := cTransInit.Recv(ctx, trStartMSG)
	validateTransfer(t, err, cTransNext.GetTransferRequest(), statemachine.TransferRequestStates.STARTED, agreementID)

	apply, err = cTransNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	cTransRes, err := store.GetConsumerTransfer(ctx, trCPID)
	validateTransfer(t, err, cTransRes, statemachine.TransferRequestStates.COMPLETED, agreementID)
	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(
		t,
		providerCallback.String()+"transfers/"+trProviderPID.String()+"/completion",
		requester.ReceivedURL.String(),
	)

	trCompletionMSG, err := decode[shared.TransferCompletionMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, trCPID.URN(), trCompletionMSG.ConsumerPID)
	assert.Equal(t, trProviderPID.URN(), trCompletionMSG.ProviderPID)

	mockProvider.On("UnpublishDataset", mock.Anything, &providerv1.UnpublishDatasetRequest{
		PublishId: trProviderPID.String(),
	}).Return(&providerv1.UnpublishDatasetResponse{
		Success: true,
	}, nil)

	pTransContractStarted, err := store.GetProviderTransfer(ctx, trProviderPID)
	validateTransfer(t, err, pTransContractStarted, statemachine.TransferRequestStates.STARTED, agreementID)

	pTransStarted := statemachine.GetTransferRequestNegotiation(store, pTransContractStarted, mockProvider, reconciler)
	pTransNext, err = pTransStarted.Recv(ctx, trCompletionMSG)
	validateTransfer(t, err, pTransNext.GetTransferRequest(), statemachine.TransferRequestStates.COMPLETED, agreementID)
}

// TestContractStateMachineConsumerInit tests a whole contract statemachine run, this will do the happy path,
// acting like the consumer initiated it.
//
//nolint:funlen
func TestContractStateMachineProviderInit(t *testing.T) {
	t.Parallel()

	offer := odrl.Offer{
		MessageOffer: odrl.MessageOffer{
			PolicyClass: odrl.PolicyClass{
				AbstractPolicyRule: odrl.AbstractPolicyRule{},
				ID:                 uuid.New().URN(),
			},
			Type:   "odrl:Offer",
			Target: target.URN(),
		},
	}

	logger := logging.NewJSON("error", true)
	ctx := logging.Inject(context.Background(), logger)
	store := statemachine.NewMemoryArchiver()
	requester := &MockRequester{}

	mockProvider := mockprovider.NewMockProviderServiceClient(t)
	mockProvider.On("GetDataset", mock.Anything, &providerv1.GetDatasetRequest{
		DatasetId: target.String(),
	}).Return(&providerv1.GetDatasetResponse{
		Dataset: &providerv1.Dataset{},
	}, nil)

	reconciler := statemachine.NewReconciler(ctx, requester, store)
	reconciler.Run()

	ctx, pInit, err := statemachine.NewContract(
		ctx, store, mockProvider, reconciler, providerPID, uuid.UUID{},
		statemachine.ContractStates.INITIAL, offer, provInitCB, providerCallback, statemachine.DataspaceProvider)
	assert.Nil(t, err)
	apply, err := pInit.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	pInitC, err := store.GetProviderContract(ctx, providerPID)
	validateContract(t, err, pInitC, statemachine.ContractStates.OFFERED, true)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t, provInitCB.String()+"negotiations/offers", requester.ReceivedURL.String())

	offMSG, err := decode[shared.ContractOfferMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, providerPID.URN(), offMSG.ProviderPID)
	assert.Equal(t, providerCallback.String(), offMSG.CallbackAddress)
	assert.Equal(t, target.URN(), offMSG.Offer.Target)

	ctx, cInit, err := statemachine.NewContract(
		ctx, store, mockProvider, reconciler, uuid.MustParse(offMSG.ProviderPID), uuid.UUID{},
		statemachine.ContractStates.INITIAL, odrl.Offer{MessageOffer: offMSG.Offer},
		urlMustParse(offMSG.CallbackAddress), consumerCallback, statemachine.DataspaceConsumer,
	)
	assert.Nil(t, err)

	ctx, nextProvider, err := cInit.Recv(ctx, offMSG)
	validateContract(t, err, nextProvider.GetContract(), statemachine.ContractStates.OFFERED, false)

	gConsumerPID := nextProvider.GetConsumerPID()

	apply, err = nextProvider.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	cReqRes, err := store.GetConsumerContract(ctx, gConsumerPID)
	validateContract(t, err, cReqRes, statemachine.ContractStates.REQUESTED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		providerCallback.String()+"negotiations/"+providerPID.String()+"/request",
		requester.ReceivedURL.String())

	reqMSG, err := decode[shared.ContractRequestMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, gConsumerPID.URN(), reqMSG.ConsumerPID)
	assert.Equal(t, providerPID.URN(), reqMSG.ProviderPID)
	assert.Equal(t, consumerCallback.String(), reqMSG.CallbackAddress)
	assert.Equal(t, target.URN(), reqMSG.Offer.Target)

	pReqContract, err := store.GetProviderContract(ctx, uuid.MustParse(reqMSG.ProviderPID))
	validateContract(t, err, pReqContract, statemachine.ContractStates.OFFERED, true)

	ctx, pOffState := statemachine.GetContractNegotiation(ctx, store, pReqContract, mockProvider, reconciler)
	ctx, pOffNext, err := pOffState.Recv(ctx, reqMSG)
	validateContract(t, err, pOffNext.GetContract(), statemachine.ContractStates.REQUESTED, false)

	apply, err = pOffNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	pAgreedRes, err := store.GetProviderContract(ctx, providerPID)
	validateContract(t, err, pAgreedRes, statemachine.ContractStates.AGREED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		consumerCallback.String()+"negotiations/"+gConsumerPID.String()+"/agreement",
		requester.ReceivedURL.String())

	agreeMSG, err := decode[shared.ContractAgreementMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, gConsumerPID.URN(), agreeMSG.ConsumerPID)
	assert.Equal(t, providerPID.URN(), agreeMSG.ProviderPID)
	assert.Equal(t, providerCallback.String(), agreeMSG.CallbackAddress)
	assert.Equal(t, target.URN(), agreeMSG.Agreement.Target)

	cContract, err := store.GetConsumerContract(ctx, uuid.MustParse(agreeMSG.ConsumerPID))
	validateContract(t, err, cContract, statemachine.ContractStates.REQUESTED, false)

	ctx, pOffState = statemachine.GetContractNegotiation(ctx, store, cContract, mockProvider, reconciler)
	ctx, pOffNext, err = pOffState.Recv(ctx, agreeMSG)
	validateContract(t, err, pOffNext.GetContract(), statemachine.ContractStates.AGREED, false)

	apply, err = pOffNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	cVerRes, err := store.GetConsumerContract(ctx, gConsumerPID)
	validateContract(t, err, cVerRes, statemachine.ContractStates.VERIFIED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		providerCallback.String()+"negotiations/"+providerPID.String()+"/agreement/verification",
		requester.ReceivedURL.String())

	verMSG, err := decode[shared.ContractAgreementVerificationMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, gConsumerPID.URN(), verMSG.ConsumerPID)
	assert.Equal(t, providerPID.URN(), verMSG.ProviderPID)

	pContract, err := store.GetProviderContract(ctx, uuid.MustParse(verMSG.ProviderPID))
	validateContract(t, err, pContract, statemachine.ContractStates.AGREED, false)

	ctx, pState := statemachine.GetContractNegotiation(ctx, store, pContract, mockProvider, reconciler)
	ctx, pNext, err := pState.Recv(ctx, verMSG)
	validateContract(t, err, pNext.GetContract(), statemachine.ContractStates.VERIFIED, false)

	apply, err = pNext.Send(ctx)
	assert.Nil(t, err)
	apply()
	time.Sleep(reconcileWait)

	pFinRes, err := store.GetProviderContract(ctx, providerPID)
	validateContract(t, err, pFinRes, statemachine.ContractStates.FINALIZED, false)

	assert.Equal(t, "POST", requester.ReceivedMethod)
	assert.Equal(t,
		consumerCallback.String()+"negotiations/"+gConsumerPID.String()+"/events",
		requester.ReceivedURL.String())

	finMSG, err := decode[shared.ContractNegotiationEventMessage](requester.ReceivedBody)
	assert.Nil(t, err)
	assert.Equal(t, gConsumerPID.URN(), finMSG.ConsumerPID)
	assert.Equal(t, providerPID.URN(), finMSG.ProviderPID)
	assert.Equal(t, "dspace:FINALIZED", finMSG.EventType)

	cContract, err = store.GetConsumerContract(ctx, uuid.MustParse(finMSG.ConsumerPID))
	validateContract(t, err, cContract, statemachine.ContractStates.VERIFIED, false)

	ctx, pOffState = statemachine.GetContractNegotiation(ctx, store, cContract, mockProvider, reconciler)
	_, pOffNext, err = pOffState.Recv(ctx, finMSG)
	validateContract(t, err, pOffNext.GetContract(), statemachine.ContractStates.FINALIZED, false)
}

//nolint:funlen
func TestTermination(t *testing.T) {
	t.Parallel()

	offer := odrl.Offer{
		MessageOffer: odrl.MessageOffer{
			PolicyClass: odrl.PolicyClass{
				AbstractPolicyRule: odrl.AbstractPolicyRule{},
				ID:                 uuid.New().URN(),
			},
			Type:   "odrl:Offer",
			Target: target.URN(),
		},
	}

	logger := logging.NewJSON("error", true)
	ctx := logging.Inject(context.Background(), logger)
	ctx, done := context.WithCancel(ctx)
	defer done()

	store := statemachine.NewMemoryArchiver()
	requester := &MockRequester{}

	mockProvider := mockprovider.NewMockProviderServiceClient(t)

	reconciler := statemachine.NewReconciler(ctx, requester, store)
	reconciler.Run()

	for _, role := range []statemachine.DataspaceRole{
		statemachine.DataspaceConsumer,
		statemachine.DataspaceProvider,
	} {
		for _, state := range []statemachine.ContractState{
			statemachine.ContractStates.REQUESTED,
			statemachine.ContractStates.OFFERED,
			statemachine.ContractStates.ACCEPTED,
			statemachine.ContractStates.AGREED,
			statemachine.ContractStates.VERIFIED,
		} {
			consumerPID := uuid.New()
			providerPID := uuid.New()
			ctx, consumerInit, err := statemachine.NewContract(
				ctx, store, mockProvider, reconciler, providerPID, consumerPID,
				state, offer, providerCallback, consumerCallback, role)
			assert.Nil(t, err)
			msg := shared.ContractNegotiationTerminationMessage{
				Context:     shared.GetDSPContext(),
				Type:        "dspace:ContractNegotiationTerminationMessage",
				ProviderPID: providerPID.URN(),
				ConsumerPID: consumerPID.URN(),
				Code:        "meh",
				Reason: []shared.Multilanguage{
					{
						Language: "en",
						Value:    "test",
					},
				},
			}
			ctx, next, err := consumerInit.Recv(ctx, msg)
			assert.IsType(t, &statemachine.ContractNegotiationTerminated{}, next)
			assert.Nil(t, err)
			_, err = next.Send(ctx)
			assert.Nil(t, err)
			var contract *statemachine.Contract
			switch role {
			case statemachine.DataspaceProvider:
				contract, err = store.GetProviderContract(ctx, providerPID)
			case statemachine.DataspaceConsumer:
				contract, err = store.GetConsumerContract(ctx, consumerPID)
			}
			assert.Nil(t, err)
			assert.Equal(t, statemachine.ContractStates.TERMINATED, contract.GetState())
		}
	}
}

func validateContract(
	t *testing.T, err error, c *statemachine.Contract, state statemachine.ContractState, provInit bool,
) {
	t.Helper()
	assert.Nil(t, err)
	assert.Equal(t, state, c.GetState())
	if c.GetRole() == statemachine.DataspaceConsumer {
		assert.Equal(t, providerCallback, c.GetCallback())
		assert.Equal(t, consumerCallback, c.GetSelf())
	} else {
		if provInit {
			assert.Equal(t, provInitCB, c.GetCallback())
		} else {
			assert.Equal(t, consumerCallback, c.GetCallback())
			assert.Equal(t, providerCallback, c.GetSelf())
		}
	}
	assert.Equal(t, target.URN(), c.GetOffer().MessageOffer.Target)
}

func validateTransfer(
	t *testing.T, err error, c *statemachine.TransferRequest, state statemachine.TransferRequestState,
	agreementID uuid.UUID,
) {
	t.Helper()
	assert.Nil(t, err)
	assert.Equal(t, state, c.GetState())
	assert.Equal(t, agreementID, c.GetAgreementID())
	if c.GetRole() == statemachine.DataspaceConsumer {
		assert.Equal(t, providerCallback, c.GetCallback())
		assert.Equal(t, consumerCallback, c.GetSelf())
	} else {
		assert.Equal(t, consumerCallback, c.GetCallback())
		assert.Equal(t, providerCallback, c.GetSelf())
	}

	assert.Equal(t, statemachine.DirectionPull, c.GetTransferDirection())

	if c.GetPublishInfo() != nil {
		assert.Equal(t, publishURL, c.GetPublishInfo().Url)
		assert.Equal(t, providerv1.AuthenticationType_AUTHENTICATION_TYPE_BEARER, c.GetPublishInfo().AuthenticationType)
		assert.Equal(t, "", c.GetPublishInfo().Username)
		assert.Equal(t, token, c.GetPublishInfo().Password)
	}
}
