// Copyright 2025 go-dataspace
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
	"encoding/json"
	"path"
	"slices"

	"github.com/google/uuid"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	contractopts "go-dataspace.eu/run-dsp/dsp/persistence/options/contract"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	"go-dataspace.eu/run-dsp/internal/authforwarder"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var localRequesterInfo = &dsrpc.RequesterInfo{
	AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
}

// VerifyConnection takes a token and verifies it's the same token it passed to the contract service.
func (s *Server) VerifyConnection(
	ctx context.Context,
	req *dsrpc.VerifyConnectionRequest,
) (*dsrpc.VerifyConnectionResponse, error) {
	recvToken := req.GetVerificationToken()
	if token, err := s.store.GetToken(ctx, "contract-token"); err != nil || token != recvToken {
		return nil, status.Errorf(codes.InvalidArgument, "could not verify code: %s", err)
	}
	return &dsrpc.VerifyConnectionResponse{}, nil
}

// ContractRequest sends a ContractRequestMessage.
func (s *Server) ContractRequest(
	ctx context.Context,
	req *dsrpc.ContractRequestRequest,
) (*dsrpc.ContractRequestResponse, error) {
	rawOffer := req.GetOffer()
	rawPID := req.GetPid()
	participantAddress := req.GetParticipantAddress()

	if rawPID == "" && participantAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "need to define pid or participant address")
	}

	if rawPID == "" {
		var offer odrl.Offer
		if err := json.Unmarshal([]byte(rawOffer), &offer); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "couldn't parse offer: %s", err)
		}

		providerURL, err := s.getProviderURL(ctx, participantAddress)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "could not reach provider: %s", err)
		}
		negotiation := contract.New(
			ctx,
			uuid.UUID{}, uuid.New(),
			contract.States.INITIAL,
			offer,
			providerURL,
			shared.MustParseURL(s.selfURL.String()),
			constants.DataspaceConsumer,
			req.GetAutoAccept() || s.contractService == nil,
			localRequesterInfo,
		)
		rawPID = negotiation.GetConsumerPID().String()
		if err := s.store.PutContract(ctx, negotiation); err != nil {
			return nil, status.Errorf(codes.Internal, "couldn't store contract negotiation: %s", err)
		}
	}

	return sendContractMessage[dsrpc.ContractRequestResponse](
		ctx,
		s,
		[]contract.State{contract.States.INITIAL, contract.States.OFFERED},
		rawPID,
		constants.DataspaceConsumer,
		true,
		req.GetAutoAccept(),
	)
}

func sendContractMessage[T any](
	ctx context.Context,
	s *Server,
	validFromStates []contract.State,
	rawPid string,
	role constants.DataspaceRole,
	initial, autoAccept bool,
) (*T, error) {
	var thing T
	pid, err := uuid.Parse(rawPid)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "couldn't parse UUID")
	}
	// Add local origin requester info to context since all control requests are from
	// a trusted source.
	ctx = authforwarder.SetRequesterInfo(ctx, localRequesterInfo)
	negotiation, err := s.store.GetContract(ctx, contractopts.WithRolePID(pid, role))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not find contract with pid %s: %s", pid, err)
	}
	if err := authforwarder.CheckRequesterInfo(ctx, negotiation.GetRequesterInfo()); err != nil {
		releaseErr := s.store.ReleaseContract(ctx, negotiation)
		if releaseErr != nil {
			ctxslog.Err(ctx, "problem when trying to release lock", releaseErr)
		}
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if initial {
		negotiation.SetInitial()
	}
	if autoAccept {
		negotiation.SetAutoAccept()
	}
	ctx = ctxslog.With(ctx, negotiation.GetLogFields("")...)
	ctxslog.Info(ctx, "Processing negotiation")
	if !slices.Contains(validFromStates, negotiation.GetState()) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid initial state: %s", negotiation.GetState())
	}

	contractTransition := statemachine.GetContractNegotiation(
		negotiation,
		s.provider,
		s.contractService,
		s.reconciler,
	)

	apply, err := contractTransition.Send(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Couldn't progress contract negotiation: %s", err)
	}
	if err := s.store.PutContract(ctx, negotiation); err != nil {
		return nil, status.Errorf(codes.Internal, "couldn't store contract negotiation: %s", err)
	}

	// The applyFunc for Send doesn't really return errors but the signature says for uniformity.
	_ = apply()
	return &thing, nil
}

// ContractOffer sends a ContractOfferMessage.
func (s *Server) ContractOffer(
	ctx context.Context,
	req *dsrpc.ContractOfferRequest,
) (*dsrpc.ContractOfferResponse, error) {
	rawOffer := req.GetOffer()
	rawPID := req.GetPid()
	participantAddress := req.GetParticipantAddress()

	if rawPID == "" && participantAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "need to define pid or participant address")
	}

	if rawPID == "" {
		var offer odrl.Offer
		if err := json.Unmarshal([]byte(rawOffer), &offer); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "couldn't parse offer: %s", err)
		}

		providerURL, err := s.getProviderURL(ctx, participantAddress)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "could not reach provider: %s", err)
		}
		negotiation := contract.New(
			ctx,
			uuid.New(), uuid.UUID{},
			contract.States.INITIAL,
			offer,
			providerURL,
			shared.MustParseURL(s.selfURL.String()),
			constants.DataspaceProvider,
			req.GetAutoAccept() || s.contractService == nil,
			localRequesterInfo,
		)

		rawPID = negotiation.GetProviderPID().String()
		if err := s.store.PutContract(ctx, negotiation); err != nil {
			return nil, status.Errorf(codes.Internal, "couldn't store contract negotiation: %s", err)
		}
	}

	return sendContractMessage[dsrpc.ContractOfferResponse](
		ctx,
		s,
		[]contract.State{contract.States.INITIAL, contract.States.REQUESTED},
		rawPID,
		constants.DataspaceProvider,
		true,
		req.GetAutoAccept(),
	)
}

// ContractAccept sends an accepted event message.
func (s *Server) ContractAccept(
	ctx context.Context,
	req *dsrpc.ContractAcceptRequest,
) (*dsrpc.ContractAcceptResponse, error) {
	return sendContractMessage[dsrpc.ContractAcceptResponse](
		ctx,
		s,
		[]contract.State{contract.States.OFFERED},
		req.GetPid(),
		constants.DataspaceConsumer,
		false,
		req.GetAutoAccept(),
	)
}

// ContractAgree sends a ContractAcceptedMessage.
func (s *Server) ContractAgree(
	ctx context.Context,
	req *dsrpc.ContractAgreeRequest,
) (*dsrpc.ContractAgreeResponse, error) {
	return sendContractMessage[dsrpc.ContractAgreeResponse](
		ctx,
		s,
		[]contract.State{contract.States.REQUESTED, contract.States.ACCEPTED},
		req.GetPid(),
		constants.DataspaceProvider,
		false,
		req.GetAutoAccept(),
	)
}

// ContractVerify sends a ContractVerificationMessage.
func (s *Server) ContractVerify(
	ctx context.Context,
	req *dsrpc.ContractVerifyRequest,
) (*dsrpc.ContractVerifyResponse, error) {
	return sendContractMessage[dsrpc.ContractVerifyResponse](
		ctx,
		s,
		[]contract.State{contract.States.AGREED},
		req.GetPid(),
		constants.DataspaceConsumer,
		false,
		req.GetAutoAccept(),
	)
}

// ContractFinalize sends a finalization event.
func (s *Server) ContractFinalize(
	ctx context.Context,
	req *dsrpc.ContractFinalizeRequest,
) (*dsrpc.ContractFinalizeResponse, error) {
	return sendContractMessage[dsrpc.ContractFinalizeResponse](
		ctx,
		s,
		[]contract.State{contract.States.VERIFIED},
		req.GetPid(),
		constants.DataspaceProvider,
		false,
		req.GetAutoAccept(),
	)
}

// ContractTerminate sends a ContractTerminationMessage.
func (s *Server) ContractTerminate(
	ctx context.Context,
	req *dsrpc.ContractTerminateRequest,
) (*dsrpc.ContractTerminateResponse, error) {
	pid, err := uuid.Parse(req.GetPid())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not parse PID: %s", err)
	}

	var negotiation *contract.Negotiation
	for _, role := range []constants.DataspaceRole{constants.DataspaceConsumer, constants.DataspaceProvider} {
		negotiation, err = s.store.GetContract(ctx, contractopts.WithRolePID(pid, role))
		if err == nil {
			ctxslog.Debug(ctx, "Contract found", "pid", negotiation.GetLocalPID().String(), "role", role)
			break
		}
	}
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not find contract with PID %s: %s", pid.String(), err)
	}

	reasons := make([]shared.Multilanguage, 0)
	for _, reason := range req.Reason {
		reasons = append(reasons, shared.Multilanguage{
			Language: "en", // hardcoded for now, going i18n will be its own PR.
			Value:    reason,
		})
	}
	negotiationTermination := shared.ContractNegotiationTerminationMessage{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:ContractNegotiationTerminationMessage",
		ProviderPID: negotiation.GetProviderPID().URN(),
		ConsumerPID: negotiation.GetConsumerPID().URN(),
		Code:        req.GetCode(),
		Reason:      reasons,
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, negotiationTermination)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not encode termination message: %s", err)
	}

	cu := shared.MustParseURL(negotiation.GetCallback().String())
	cu.Path = path.Join(cu.Path, "negotiations", negotiation.GetRemotePID().String(), "termination")
	s.reconciler.Add(statemachine.ReconciliationEntry{
		EntityID:    negotiation.GetLocalPID(),
		Type:        statemachine.ReconciliationContract,
		Role:        negotiation.GetRole(),
		TargetState: contract.States.TERMINATED.String(),
		Method:      "POST",
		URL:         cu,
		Body:        reqBody,
		Context:     ctx,
	})
	return &dsrpc.ContractTerminateResponse{}, nil
}
