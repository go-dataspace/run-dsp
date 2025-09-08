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

	"github.com/google/uuid"
	"go-dataspace.eu/ctxslog"
	dspconstants "go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/persistence"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	"go-dataspace.eu/run-dsp/dsp/transfer"
	"go-dataspace.eu/run-dsp/internal/authforwarder"
	"go-dataspace.eu/run-dsp/internal/constants"
	"go-dataspace.eu/run-dsp/jsonld"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var dspaceContext = jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}})

type Server struct {
	dsrpc.ControlServiceServer

	requester       shared.Requester
	store           persistence.StorageProvider
	reconciler      statemachine.Reconciler
	provider        dsrpc.ProviderServiceClient
	contractService dsrpc.ContractServiceClient
	selfURL         *url.URL
	tracer          trace.Tracer
}

func New(
	requester shared.Requester,
	store persistence.StorageProvider,
	reconciler statemachine.Reconciler,
	provider dsrpc.ProviderServiceClient,
	contractService dsrpc.ContractServiceClient,
	selfURL *url.URL,
) *Server {
	return &Server{
		requester:       requester,
		store:           store,
		reconciler:      reconciler,
		provider:        provider,
		contractService: contractService,
		selfURL:         selfURL,
		tracer:          otel.Tracer("codeberg.org/go-dataspace/run-dsp/dsp/control"),
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

// Gets the catalogue based on the query parameters and the authorization header.
func (s *Server) GetProviderCatalogue(
	ctx context.Context, req *dsrpc.GetProviderCatalogueRequest,
) (*dsrpc.GetProviderCatalogueResponse, error) {
	ctx, span := s.tracer.Start(ctx, "GetProviderCatalogue")
	defer span.End()
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

	return &dsrpc.GetProviderCatalogueResponse{
		Datasets: processCatalogue(resp.Datasets),
	}, nil
}

// Gets information about a single dataset.
func (s *Server) GetProviderDataset(
	ctx context.Context, req *dsrpc.GetProviderDatasetRequest,
) (*dsrpc.GetProviderDatasetResponse, error) {
	ctx, span := s.tracer.Start(ctx, "GetProviderDataset")
	defer span.End()
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

	return &dsrpc.GetProviderDatasetResponse{
		ProviderUrl: req.ProviderUrl,
		Dataset:     processDataset(resp),
	}, err
}

// Publishes a dataset.
//
//nolint:funlen,cyclop
func (s *Server) GetProviderDatasetDownloadInformation(
	ctx context.Context, req *dsrpc.GetProviderDatasetDownloadInformationRequest,
) (*dsrpc.GetProviderDatasetDownloadInformationResponse, error) {
	ctx, span := s.tracer.Start(ctx, "GetProviderDatasetDownloadInformation")
	defer span.End()
	providerURL, err := s.getProviderURL(ctx, req.GetProviderUrl())
	if err != nil {
		return nil, err
	}

	selfURL := shared.MustParseURL(s.selfURL.String())
	selfURL.Path = path.Join(selfURL.Path, "callback")
	negotiation := contract.New(
		ctx,
		uuid.UUID{}, uuid.New(),
		contract.States.INITIAL,
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
		dspconstants.DataspaceConsumer,
		s.contractService == nil,
		&dsrpc.RequesterInfo{
			AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
		},
	)
	// Store and retrieve contract negotiation so that it's saved and the locking works.
	if err := s.store.PutContract(ctx, negotiation); err != nil {
		return nil, status.Errorf(codes.Internal, "couldn't store contract negotiation: %s", err)
	}
	negotiation, err = s.store.GetContractRW(ctx, negotiation.GetLocalPID(), negotiation.GetRole())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "couldn't retrieve contract negotiation: %s", err)
	}

	ctx = ctxslog.With(ctx, negotiation.GetLogFields("_contract_negotiation")...)

	contractInit := statemachine.GetContractNegotiation(negotiation, s.provider, s.contractService, s.reconciler)

	apply, err := contractInit.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't generate inital contract request.")
	}
	if err := s.store.PutContract(ctx, negotiation); err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't store contract negotiation: %s", err)
	}
	ctxslog.Debug(ctx, "Beginning contract negotiation")
	// These don't really return errors, the err is for uniformity with the recv applyFunc
	_ = apply()

	negotiation, err = s.store.GetContractR(ctx, negotiation.GetLocalPID(), dspconstants.DataspaceConsumer)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal, "could not get consumer contract with PID %s: %s", negotiation.GetLocalPID(), err)
	}

	ctxslog.Info(ctx, "Starting to monitor contract")
	checks := 0

	for negotiation.GetState() != contract.States.FINALIZED {
		// Only log the status every 10 checks.
		if checks%10 == 0 {
			ctxslog.Debug(ctx, "Contract not finalized...", "checks", checks, "state", negotiation.GetState().String())
		}
		time.Sleep(1 * time.Second)
		negotiation, err = s.store.GetContractR(ctx, negotiation.GetLocalPID(), dspconstants.DataspaceConsumer)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal, "could not get consumer contract with PID %s: %s", negotiation.GetLocalPID(), err)
		}

		if negotiation.GetState() == contract.States.TERMINATED {
			return nil, status.Errorf(codes.Aborted, "contract negotiation terminated")
		}

		checks++
	}
	ctxslog.Info(ctx, "Contract finalized, continuing")
	transferConsumerPID := uuid.New()
	agreementID := uuid.MustParse(negotiation.GetAgreement().ID)
	agreement, err := s.store.GetAgreement(ctx, agreementID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get agreement with ID %s: %s", agreementID, err)
	}

	transferReq := transfer.New(
		ctx,
		transferConsumerPID,
		agreement,
		"HTTP_PULL",
		providerURL,
		selfURL,
		dspconstants.DataspaceConsumer,
		transfer.States.INITIAL,
		nil,
		&dsrpc.RequesterInfo{
			AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
		},
	)
	// Save and retrieve the transfer request to get the locks working properly.
	if err := s.store.PutTransfer(ctx, transferReq); err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't create transfer request: %s", err)
	}
	transferReq, err = s.store.GetTransferRW(ctx, transferReq.GetConsumerPID(), dspconstants.DataspaceConsumer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not retrieve transfer request: %s", err)
	}
	if err := authforwarder.CheckRequesterInfo(ctx, transferReq.GetRequesterInfo()); err != nil {
		releaseErr := s.store.ReleaseTransfer(ctx, transferReq)
		if releaseErr != nil {
			ctxslog.Error(ctx, "problem when trying to release lock", "err", releaseErr)
		}
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	transferInit := statemachine.GetTransferRequestNegotiation(transferReq, s.provider, s.reconciler)

	tApply, err := transferInit.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't generate inital initial request.")
	}
	if err := s.store.PutTransfer(ctx, transferReq); err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't create transfer request: %s", err)
	}

	ctxslog.Debug(ctx, "Beginning transfer request")
	tApply()

	tReq, err := s.store.GetTransferR(ctx, transferConsumerPID, dspconstants.DataspaceConsumer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get consumer transfer with PID %s: %s", transferConsumerPID, err)
	}
	if err := authforwarder.CheckRequesterInfo(ctx, tReq.GetRequesterInfo()); err != nil {
		return nil, err
	}

	ctx = ctxslog.With(ctx, tReq.GetLogFields("_transfer_requsts")...)

	ctxslog.Info(ctx, "Starting to monitor transfer request")
	for tReq.GetState() != transfer.States.STARTED {
		ctxslog.Debug(ctx, "Transfer not started", "current_state", tReq.GetState().String())
		time.Sleep(1 * time.Second)
		tReq, err = s.store.GetTransferR(ctx, transferConsumerPID, dspconstants.DataspaceConsumer)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal, "could not get consumer contract with PID %s: %s", transferConsumerPID, err,
			)
		}
		if err := authforwarder.CheckRequesterInfo(ctx, tReq.GetRequesterInfo()); err != nil {
			return nil, err
		}

		if tReq.GetState() == transfer.States.TERMINATED {
			return nil, status.Error(codes.Aborted, "transfer negotiation terminated")
		}
	}

	return &dsrpc.GetProviderDatasetDownloadInformationResponse{
		PublishInfo: tReq.GetPublishInfo(),
		TransferId:  tReq.GetConsumerPID().String(),
	}, nil
}

// Requests information for pushing a dataset.
//
//nolint:funlen,cyclop
func (s *Server) GetProviderDatasetUploadInformation(
	ctx context.Context, req *dsrpc.GetProviderDatasetUploadInformationRequest,
) (*dsrpc.GetProviderDatasetUploadInformationResponse, error) {
	ctx, span := s.tracer.Start(ctx, "GetProviderDatasetUploadInformation")
	defer span.End()
	providerURL, err := s.getProviderURL(ctx, req.GetProviderUrl())
	if err != nil {
		return nil, err
	}

	selfURL := shared.MustParseURL(s.selfURL.String())
	negotiation := contract.New(
		ctx,
		uuid.New(), uuid.UUID{},
		contract.States.INITIAL,
		odrl.Offer{
			MessageOffer: odrl.MessageOffer{
				PolicyClass: odrl.PolicyClass{
					AbstractPolicyRule: odrl.AbstractPolicyRule{},
					ID:                 uuid.New().URN(),
					Permission: []odrl.Permission{
						{
							Action: "odrl:copy",
						},
					},
				},
				Type:   "odrl:Offer",
				Target: shared.IDToURN(req.GetDatasetId()),
			},
		},
		providerURL,
		selfURL,
		dspconstants.DataspaceProvider,
		s.contractService == nil,
		&dsrpc.RequesterInfo{
			AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
		},
	)
	// Store and retrieve contract negotiation so that it's saved and the locking works.
	if err := s.store.PutContract(ctx, negotiation); err != nil {
		return nil, status.Errorf(codes.Internal, "couldn't store contract negotiation: %s", err)
	}
	negotiation, err = s.store.GetContractRW(ctx, negotiation.GetLocalPID(), negotiation.GetRole())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "couldn't retrieve contract negotiation: %s", err)
	}

	ctx = ctxslog.With(ctx, negotiation.GetLogFields("_contract_negotiation")...)

	contractInit := statemachine.GetContractNegotiation(negotiation, s.provider, s.contractService, s.reconciler)

	apply, err := contractInit.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't generate inital contract request.")
	}
	if err := s.store.PutContract(ctx, negotiation); err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't store contract negotiation: %s", err)
	}
	ctxslog.Debug(ctx, "Beginning contract negotiation")
	// These don't really return errors, the err is for uniformity with the recv applyFunc
	_ = apply()

	negotiation, err = s.store.GetContractR(ctx, negotiation.GetLocalPID(), dspconstants.DataspaceProvider)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal, "could not get provider contract with PID %s: %s", negotiation.GetLocalPID(), err)
	}

	ctxslog.Info(ctx, "Starting to monitor contract")
	checks := 0
	for negotiation.GetState() != contract.States.FINALIZED {
		// Only log the status every 10 checks.
		if checks%10 == 0 {
			ctxslog.Info(ctx, "Contract not finalized", "current_state", negotiation.GetState().String())
		}
		time.Sleep(1 * time.Second)
		negotiation, err = s.store.GetContractR(ctx, negotiation.GetLocalPID(), dspconstants.DataspaceProvider)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal, "could not get provider contract with PID %s: %s", negotiation.GetLocalPID(), err)
		}
		checks++
	}
	ctxslog.Info(ctx, "Contract finalized, continuing")

	ctxslog.Info(ctx, "Starting to monitor transfer request")
	var tReq *transfer.Request
	for tReq == nil {
		transfers, err := s.store.GetTransfers(ctx)
		if err != nil {
			return nil, err
		}

		for _, t := range transfers {
			ctxslog.Debug(ctx, "checking if transfer has started", "transfer-agreement-id",
				t.GetAgreementID(), "negotiation-agreement-id", negotiation.GetAgreement().ID)

			// FIXME: For some reason we have two transfers requests in store for this single transfer.
			// - One in state INITIAL with no providerPID
			// - One in state STARTED, which is the one we want here.
			if t.GetState() == transfer.States.STARTED &&
				t.GetAgreementID().String() == strings.TrimPrefix(negotiation.GetAgreement().ID, "urn:uuid:") {
				tReq = t
				break
			}
		}

		if tReq != nil {
			ctxslog.Info(ctx, "Transfer requested", "state", tReq.GetState().String())
			break
		}

		ctxslog.Debug(ctx, "Transfer not yet requested")
		time.Sleep(1 * time.Second)
	}

	response := &dsrpc.GetProviderDatasetUploadInformationResponse{
		PublishInfo: tReq.GetPublishInfo(),
		TransferId:  tReq.GetProviderPID().String(),
	}
	return response, err
}

// Tells provider that we have finished our transfer.
func (s *Server) SignalTransferComplete( //nolint:cyclop
	ctx context.Context, req *dsrpc.SignalTransferCompleteRequest,
) (*dsrpc.SignalTransferCompleteResponse, error) {
	ctx, span := s.tracer.Start(ctx, "SignalTransferComplete")
	defer span.End()
	id, err := uuid.Parse(req.GetTransferId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Transfer ID is not a valid UUID.")
	}

	trReq := &transfer.Request{}
	role := dspconstants.DataspaceConsumer
	for _, role = range []dspconstants.DataspaceRole{dspconstants.DataspaceConsumer, dspconstants.DataspaceProvider} {
		trReq, _ = s.store.GetTransferRW(ctx, id, role)
		if trReq != nil {
			if err := authforwarder.CheckRequesterInfo(ctx, trReq.GetRequesterInfo()); err != nil {
				releaseErr := s.store.ReleaseTransfer(ctx, trReq)
				if releaseErr != nil {
					ctxslog.Err(ctx, "problem when trying to release lock", releaseErr)
				}
				return nil, err
			}

			break
		}
	}
	if trReq == nil {
		return nil, status.Errorf(codes.NotFound, "no transfer found")
	}

	transferState := statemachine.GetTransferRequestNegotiation(trReq, s.provider, s.reconciler)
	apply, err := transferState.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "couldn't finish transfer: %s", err)
	}
	if err := s.store.PutTransfer(ctx, trReq); err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't create transfer request: %s", err)
	}
	apply()
	for trReq.GetState() != transfer.States.COMPLETED {
		time.Sleep(1 * time.Second)
		trReq, err = s.store.GetTransferR(ctx, id, role)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not get consumer contract with PID %s: %s", id, err)
		}
		if err := authforwarder.CheckRequesterInfo(ctx, trReq.GetRequesterInfo()); err != nil {
			return nil, err
		}
	}

	return &dsrpc.SignalTransferCompleteResponse{}, nil
}

// Tells provider to cancel file transfer.
func (s *Server) SignalTransferCancelled(
	_ context.Context, _ *dsrpc.SignalTransferCancelledRequest,
) (*dsrpc.SignalTransferCancelledResponse, error) {
	panic("not implemented") // TODO: Implement
}

// Tells provider to suspend file transfer.
func (s *Server) SignalTransferSuspend(
	_ context.Context, _ *dsrpc.SignalTransferSuspendRequest,
) (*dsrpc.SignalTransferSuspendResponse, error) {
	panic("not implemented") // TODO: Implement
}

// Tells provider to resume file transfer.
func (s *Server) SignalTransferResume(
	_ context.Context, _ *dsrpc.SignalTransferResumeRequest,
) (*dsrpc.SignalTransferResumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (s *Server) InitiatePushTransfer(ctx context.Context, r *dsrpc.InitiatePushTransferRequest) (
	*dsrpc.InitiatePushTransferResponse, error,
) {
	ctx, span := s.tracer.Start(ctx, "InitiatePushTransfer")
	defer span.End()
	selfURL := shared.MustParseURL(s.selfURL.String())
	selfURL.Path = path.Join(selfURL.Path, "callback")
	transferConsumerPID := uuid.New()

	// Add requester info to context
	ctx = authforwarder.SetRequesterInfo(ctx, r.RequesterInfo)
	contractID := uuid.MustParse(r.Pid)
	negotiation, err := s.store.GetContractR(ctx, contractID, dspconstants.DataspaceConsumer)
	if err != nil {
		return nil, err
	}
	response, err := s.provider.ReceiveDataset(ctx, &dsrpc.ReceiveDatasetRequest{
		DatasetId:     negotiation.GetOffer().Target, // FIXME: This could be bad and potentially overwrite things.
		RequesterInfo: r.RequesterInfo,
	})
	if err != nil {
		return nil, err
	}
	transferReq := transfer.New(
		ctx,
		transferConsumerPID,
		negotiation.GetAgreement(),
		"HTTP_PUSH",
		negotiation.GetCallback(),
		selfURL,
		dspconstants.DataspaceConsumer,
		transfer.States.INITIAL,
		response.PublishInfo,
		&dsrpc.RequesterInfo{
			AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
		},
	)
	// Save and retrieve the transfer request to get the locks working properly.
	if err := s.store.PutTransfer(ctx, transferReq); err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't create transfer request: %s", err)
	}
	transferReq, err = s.store.GetTransferRW(ctx, transferReq.GetConsumerPID(), dspconstants.DataspaceConsumer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not retrieve transfer request: %s", err)
	}
	if err := authforwarder.CheckRequesterInfo(ctx, transferReq.GetRequesterInfo()); err != nil {
		releaseErr := s.store.ReleaseTransfer(ctx, transferReq)
		if releaseErr != nil {
			ctxslog.Err(ctx, "problem when trying to release lock", releaseErr)
		}
		return nil, err
	}

	transferInit := statemachine.GetTransferRequestNegotiation(transferReq, s.provider, s.reconciler)
	tApply, err := transferInit.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't generate inital initial request.")
	}
	if err := s.store.PutTransfer(ctx, transferReq); err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't create transfer request: %s", err)
	}

	tApply()
	return &dsrpc.InitiatePushTransferResponse{}, nil
}

func processCatalogue(cat []shared.Dataset) []*dsrpc.Dataset {
	rCat := make([]*dsrpc.Dataset, len(cat))
	for i, d := range cat {
		rCat[i] = processDataset(d)
	}
	return rCat
}

func processDataset(dsp shared.Dataset) *dsrpc.Dataset {
	issued, err := time.Parse(time.RFC3339, dsp.Issued)
	if err != nil {
		issued = time.Unix(0, 0).UTC()
	}
	modified, err := time.Parse(time.RFC3339, dsp.Modified)
	if err != nil {
		modified = time.Unix(0, 0).UTC()
	}
	a := ""
	mediaType := ""
	var desc []*dsrpc.Multilingual
	var size int64
	var checksum *dsrpc.Checksum
	if len(dsp.Distribution) > 0 {
		d := dsp.Distribution[0]
		desc = make([]*dsrpc.Multilingual, len(d.Description))
		for i, v := range d.Description {
			desc[i] = &dsrpc.Multilingual{
				Value:    v.Value,
				Language: v.Language,
			}
		}
		a = d.Format
		mediaType = d.MediaType
		size = int64(d.ByteSize)
		if d.Checksum != nil {
			checksum = &dsrpc.Checksum{
				Algorithm: d.Checksum.Algorithm,
				Value:     d.Checksum.Value,
			}
		}
	}
	return &dsrpc.Dataset{
		Id:            strings.TrimPrefix(dsp.ID, "urn:uuid:%s"),
		Title:         dsp.Title,
		AccessMethods: a,
		Description:   desc,
		Keywords:      dsp.Keyword,
		Creator:       &dsp.Creator,
		Issued:        timestamppb.New(issued),
		Modified:      timestamppb.New(modified),
		Metadata:      map[string]string{},
		MediaType:     mediaType,
		ByteSize:      size,
		Checksum:      checksum,
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
