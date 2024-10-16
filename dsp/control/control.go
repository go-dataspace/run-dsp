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

package control

import (
	"context"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	dspv1alpha1 "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var dspaceContext = jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}})

type Server struct {
	dspv1alpha1.ClientServiceServer

	requester  shared.Requester
	store      statemachine.Archiver
	reconciler *statemachine.Reconciler
	provider   dspv1alpha1.ProviderServiceClient
	selfURL    *url.URL
}

func New(
	requester shared.Requester,
	store statemachine.Archiver,
	reconciler *statemachine.Reconciler,
	provider dspv1alpha1.ProviderServiceClient,
	selfURL *url.URL,
) *Server {
	return &Server{
		requester:  requester,
		store:      store,
		reconciler: reconciler,
		provider:   provider,
		selfURL:    selfURL,
	}
}

func (s *Server) getProviderURL(ctx context.Context, u string) (*url.URL, error) {
	pu, err := url.Parse(u)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid URL: %s", u)
	}
	pu.Path = path.Join(pu.Path, ".well-known", "dspace-version")
	resp, err := s.requester.SendHTTPRequest(ctx, "GET", pu, nil)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not fetch dataspace version url %s: %s", pu.String(), err)
	}

	wellKnown, err := shared.UnmarshalAndValidate(ctx, resp, shared.VersionResponse{})
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "Invalid version response: %s", err)
	}

	for _, v := range wellKnown.ProtocolVersions {
		if v.Version == constants.DSPVersion {
			du := shared.MustParseURL(u)
			du.Path = path.Join(du.Path, v.Path)
			return du, nil
		}
	}
	return nil, status.Errorf(
		codes.FailedPrecondition,
		"provider does not support dataspace protocol version %s", constants.DSPVersion,
	)
}

// Ping is a request to test if the provider is working, and to test the auth information.
func (s *Server) Ping(
	_ context.Context, _ *dspv1alpha1.ClientServicePingRequest,
) (*dspv1alpha1.ClientServicePingResponse, error) {
	return &dspv1alpha1.ClientServicePingResponse{}, nil
}

// Gets the catalogue based on the query parameters and the authorization header.
func (s *Server) GetProviderCatalogue(
	ctx context.Context, req *dspv1alpha1.GetProviderCatalogueRequest,
) (*dspv1alpha1.GetProviderCatalogueResponse, error) {
	providerURL, err := s.getProviderURL(ctx, req.ProviderUri)
	if err != nil {
		return nil, err
	}

	providerURL.Path = path.Join(providerURL.Path, "catalog", "request")
	resp, err := encodeRequestDecode[shared.CatalogRequestMessage, shared.CatalogAcknowledgement](
		ctx,
		s.requester,
		"POST",
		providerURL,
		shared.CatalogRequestMessage{
			Context: dspaceContext,
			Type:    "dspace:CatalogRequestMessage",
			Filter:  []any{},
		},
	)
	if err != nil {
		return nil, err
	}

	return &dspv1alpha1.GetProviderCatalogueResponse{
		Datasets: processCatalogue(resp.Datasets),
	}, nil
}

// Gets information about a single dataset.
func (s *Server) GetProviderDataset(
	ctx context.Context, req *dspv1alpha1.GetProviderDatasetRequest,
) (*dspv1alpha1.GetProviderDatasetResponse, error) {
	providerURL, err := s.getProviderURL(ctx, req.ProviderUrl)
	if err != nil {
		return nil, err
	}

	providerURL.Path = path.Join(providerURL.Path, "catalog", "datasets", req.DatasetId)
	resp, err := encodeRequestDecode[shared.DatasetRequestMessage, shared.Dataset](
		ctx,
		s.requester,
		"GET",
		providerURL,
		shared.DatasetRequestMessage{
			Context: dspaceContext,
			Type:    "dspace:DatasetRequestMessage",
			Dataset: shared.IDToURN(req.DatasetId),
		},
	)
	if err != nil {
		return nil, err
	}

	return &dspv1alpha1.GetProviderDatasetResponse{
		ProviderUrl: req.ProviderUrl,
		Dataset:     processDataset(resp),
	}, err
}

// Publishes a dataset.
//
//nolint:funlen,cyclop
func (s *Server) GetProviderDatasetDownloadInformation(
	ctx context.Context, req *dspv1alpha1.GetProviderDatasetDownloadInformationRequest,
) (*dspv1alpha1.GetProviderDatasetDownloadInformationResponse, error) {
	providerURL, err := s.getProviderURL(ctx, req.GetProviderUrl())
	if err != nil {
		return nil, err
	}

	consumerPID := uuid.New()
	selfURL := shared.MustParseURL(s.selfURL.String())
	selfURL.Path = path.Join(selfURL.Path, "callback")
	ctx, contractInit, err := statemachine.NewContract(
		ctx,
		s.store, s.provider, s.reconciler,
		uuid.UUID{}, consumerPID,
		statemachine.ContractStates.INITIAL,
		odrl.Offer{
			MessageOffer: odrl.MessageOffer{
				PolicyClass: odrl.PolicyClass{
					AbstractPolicyRule: odrl.AbstractPolicyRule{},
					ID:                 uuid.New().URN(),
				},
				Type:   "odrl:Offer",
				Target: shared.IDToURN(req.GetDatasetId()),
			},
		},
		providerURL,
		selfURL,
		statemachine.DataspaceConsumer,
	)
	ctx, logger := logging.InjectLabels(ctx, "method", "GetProviderDownloadInformationRequest")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't create contract")
	}
	apply, err := contractInit.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't generate inital contract request.")
	}
	logger.Debug("Beginning contract negotiation")
	apply()

	contract, err := s.store.GetConsumerContract(ctx, consumerPID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get consumer contract with PID %s: %s", consumerPID, err)
	}

	logger.Info("Starting to monitor contract")
	for contract.GetState() != statemachine.ContractStates.FINALIZED {
		logger.Info("Contract not finalized", "state", contract.GetState().String())
		time.Sleep(1 * time.Second)
		contract, err = s.store.GetConsumerContract(ctx, consumerPID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not get consumer contract with PID %s: %s", consumerPID, err)
		}
	}
	logger.Info("Contract finalized, continuing")
	transferConsumerPID := uuid.New()
	agreementID := uuid.MustParse(contract.GetAgreement().ID)

	transferInit, err := statemachine.NewTransferRequest(
		ctx,
		s.store, s.provider, s.reconciler,
		transferConsumerPID,
		agreementID,
		"HTTP_PULL",
		providerURL,
		selfURL,
		statemachine.DataspaceConsumer,
		statemachine.TransferRequestStates.TRANSFERINITIAL,
		nil,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't create transfer request")
	}
	apply, err = transferInit.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't generate inital initial request.")
	}
	logger.Debug("Beginning transfer request")
	apply()

	tReq, err := s.store.GetConsumerTransfer(ctx, transferConsumerPID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get consumer transfer with PID %s: %s", transferConsumerPID, err)
	}

	logger.Info("Starting to monitor transfer request")
	for tReq.GetState() != statemachine.TransferRequestStates.STARTED {
		logger.Info("Transfer not started", "state", tReq.GetState().String())
		time.Sleep(1 * time.Second)
		tReq, err = s.store.GetConsumerTransfer(ctx, transferConsumerPID)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal, "could not get consumer contract with PID %s: %s", transferConsumerPID, err,
			)
		}
	}

	return &dspv1alpha1.GetProviderDatasetDownloadInformationResponse{
		PublishInfo: tReq.GetPublishInfo(),
		TransferId:  tReq.GetConsumerPID().String(),
	}, nil
}

// Tells provider that we have finished our transfer.
func (s *Server) SignalTransferComplete(
	ctx context.Context, req *dspv1alpha1.SignalTransferCompleteRequest,
) (*dspv1alpha1.SignalTransferCompleteResponse, error) {
	id, err := uuid.Parse(req.GetTransferId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Transfer ID is not a valid UUID.")
	}
	trReq, err := s.store.GetConsumerTransfer(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no transfer found")
	}
	transferState := statemachine.GetTransferRequestNegotiation(s.store, trReq, s.provider, s.reconciler)
	apply, err := transferState.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "couldn't finish transfer: %s", err)
	}
	apply()
	for trReq.GetState() != statemachine.TransferRequestStates.COMPLETED {
		time.Sleep(1 * time.Second)
		trReq, err = s.store.GetConsumerTransfer(ctx, id)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not get consumer contract with PID %s: %s", id, err)
		}
	}

	return &dspv1alpha1.SignalTransferCompleteResponse{}, nil
}

// Tells provider to cancel file transfer.
func (s *Server) SignalTransferCancelled(
	_ context.Context, _ *dspv1alpha1.SignalTransferCancelledRequest,
) (*dspv1alpha1.SignalTransferCancelledResponse, error) {
	panic("not implemented") // TODO: Implement
}

// Tells provider to suspend file transfer.
func (s *Server) SignalTransferSuspend(
	_ context.Context, _ *dspv1alpha1.SignalTransferSuspendRequest,
) (*dspv1alpha1.SignalTransferSuspendResponse, error) {
	panic("not implemented") // TODO: Implement
}

// Tells provider to resume file transfer.
func (s *Server) SignalTransferResume(
	_ context.Context, _ *dspv1alpha1.SignalTransferResumeRequest,
) (*dspv1alpha1.SignalTransferResumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func processCatalogue(cat []shared.Dataset) []*dspv1alpha1.Dataset {
	rCat := make([]*dspv1alpha1.Dataset, len(cat))
	for i, d := range cat {
		rCat[i] = processDataset(d)
	}
	return rCat
}

func processDataset(dsp shared.Dataset) *dspv1alpha1.Dataset {
	desc := make([]*dspv1alpha1.Multilingual, len(dsp.Description))
	for i, v := range dsp.Description {
		desc[i] = &dspv1alpha1.Multilingual{
			Value:    v.Value,
			Language: v.Language,
		}
	}
	issued, err := time.Parse(time.RFC3339, dsp.Issued)
	if err != nil {
		issued = time.Unix(0, 0).UTC()
	}
	modified, err := time.Parse(time.RFC3339, dsp.Modified)
	if err != nil {
		modified = time.Unix(0, 0).UTC()
	}
	a := ""
	if len(dsp.Distribution) > 0 {
		a = dsp.Distribution[0].Format
	}
	return &dspv1alpha1.Dataset{
		Id:            strings.TrimPrefix(dsp.ID, "urn:uuid:%s"),
		Title:         dsp.Title,
		AccessMethods: a,
		Description:   desc,
		Keywords:      dsp.Keyword,
		Creator:       &dsp.Creator,
		Issued:        timestamppb.New(issued),
		Modified:      timestamppb.New(modified),
		Metadata:      map[string]string{},
	}
}

func encodeRequestDecode[B any, R any](
	ctx context.Context, r shared.Requester, method string, u *url.URL, body B,
) (R, error) {
	var resp R
	reqBody, err := shared.ValidateAndMarshal(ctx, body)
	if err != nil {
		return resp, status.Errorf(codes.Internal, "could not encode requst body: %s", err)
	}
	respBody, err := r.SendHTTPRequest(ctx, method, u, reqBody)
	if err != nil {
		return resp, status.Errorf(codes.Unavailable, "could not request %s: %s", u.String(), err)
	}
	resp, err = shared.UnmarshalAndValidate(ctx, respBody, resp)
	if err != nil {
		return resp, status.Errorf(codes.FailedPrecondition, "could not decode response: %s", err)
	}
	return resp, nil
}
