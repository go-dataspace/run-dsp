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
	"slices"

	"github.com/go-dataspace/run-dsp/dsp/constants"
	"github.com/go-dataspace/run-dsp/dsp/contract"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	dspcontrol "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha2"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VerifyConnection takes a token and verifies it's the same token it passed to the contract service.
func (s *Server) VerifyConnection(
	ctx context.Context,
	req *dspcontrol.VerifyConnectionRequest,
) (*dspcontrol.VerifyConnectionResponse, error) {
	recvToken := req.GetVerificationToken()
	if token, err := s.store.GetToken(ctx, "contract-token"); err != nil || token != recvToken {
		return nil, status.Errorf(codes.InvalidArgument, "could not verify code: %s", err)
	}
	return &dspcontrol.VerifyConnectionResponse{}, nil
}

// ContractRequest sends a ContractRequestMessage.
func (s *Server) ContractRequest(
	ctx context.Context,
	req *dspcontrol.ContractRequestRequest,
) (*dspcontrol.ContractRequestResponse, error) {
	ctx, logger := logging.InjectLabels(ctx, "method", "ContactRequest")
	logger.Info("Called")

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
			uuid.UUID{}, uuid.New(),
			contract.States.INITIAL,
			offer,
			providerURL,
			shared.MustParseURL(s.selfURL.String()),
			constants.DataspaceConsumer,
		)
		rawPID = negotiation.GetConsumerPID().String()
		if err := s.store.PutContract(ctx, negotiation); err != nil {
			return nil, status.Errorf(codes.Internal, "couldn't store contract negotiation: %s", err)
		}
	}

	return sendContractMessage[dspcontrol.ContractRequestResponse](
		ctx,
		s,
		[]contract.State{contract.States.INITIAL, contract.States.OFFERED},
		rawPID,
		constants.DataspaceConsumer,
		true,
	)
}

func sendContractMessage[T any](
	ctx context.Context,
	s *Server,
	validFromStates []contract.State,
	rawPid string,
	role constants.DataspaceRole,
	initial bool,
) (*T, error) {
	var thing T
	pid, err := uuid.Parse(rawPid)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "couldn't parse UUID")
	}
	negotiation, err := s.store.GetContractRW(ctx, pid, role)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not find contract with pid %s: %s", pid, err)
	}
	if initial {
		negotiation.SetInitial()
	}
	ctx, logger := logging.InjectLabels(ctx,
		"negotiation_consumer_pid", negotiation.GetConsumerPID(),
		"negotiation_provider_pid", negotiation.GetProviderPID(),
		"state", negotiation.GetState(),
		"role", negotiation.GetRole(),
	)
	logger.Info("Processing negotiation")
	if !slices.Contains(validFromStates, negotiation.GetState()) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid initial state: %s", negotiation.GetState())
	}

	ctx, contractTransition := statemachine.GetContractNegotiation(
		ctx,
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
	apply()
	return &thing, nil
}

// ContractOffer sends a ContractOfferMessage.
func (s *Server) ContractOffer(
	ctx context.Context,
	req *dspcontrol.ContractOfferRequest,
) (*dspcontrol.ContractOfferResponse, error) {
	ctx, logger := logging.InjectLabels(ctx, "method", "ContractOffer")
	logger.Info("Called")

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
			uuid.New(), uuid.UUID{},
			contract.States.INITIAL,
			offer,
			providerURL,
			shared.MustParseURL(s.selfURL.String()),
			constants.DataspaceProvider,
		)
		rawPID = negotiation.GetProviderPID().String()
		if err := s.store.PutContract(ctx, negotiation); err != nil {
			return nil, status.Errorf(codes.Internal, "couldn't store contract negotiation: %s", err)
		}
	}

	return sendContractMessage[dspcontrol.ContractOfferResponse](
		ctx,
		s,
		[]contract.State{contract.States.INITIAL, contract.States.REQUESTED},
		rawPID,
		constants.DataspaceProvider,
		true,
	)
}

// ContractAccept sends an accepted event message.
func (s *Server) ContractAccept(
	ctx context.Context,
	req *dspcontrol.ContractAcceptRequest,
) (*dspcontrol.ContractAcceptResponse, error) {
	return sendContractMessage[dspcontrol.ContractAcceptResponse](
		ctx,
		s,
		[]contract.State{contract.States.OFFERED},
		req.GetPid(),
		constants.DataspaceConsumer,
		false,
	)
}

// ContractAgree sends a ContractAcceptedMessage.
func (s *Server) ContractAgree(
	ctx context.Context,
	req *dspcontrol.ContractAgreeRequest,
) (*dspcontrol.ContractAgreeResponse, error) {
	return sendContractMessage[dspcontrol.ContractAgreeResponse](
		ctx,
		s,
		[]contract.State{contract.States.REQUESTED, contract.States.ACCEPTED},
		req.GetPid(),
		constants.DataspaceProvider,
		false,
	)
}

// ContractVerify sends a ContractVerificationMessage.
func (s *Server) ContractVerify(
	ctx context.Context,
	req *dspcontrol.ContractVerifyRequest,
) (*dspcontrol.ContractVerifyResponse, error) {
	return sendContractMessage[dspcontrol.ContractVerifyResponse](
		ctx,
		s,
		[]contract.State{contract.States.AGREED},
		req.GetPid(),
		constants.DataspaceConsumer,
		false,
	)
}

// ContractFinalize sends a finalization event.
func (s *Server) ContractFinalize(
	ctx context.Context,
	req *dspcontrol.ContractFinalizeRequest,
) (*dspcontrol.ContractFinalizeResponse, error) {
	return sendContractMessage[dspcontrol.ContractFinalizeResponse](
		ctx,
		s,
		[]contract.State{contract.States.VERIFIED},
		req.GetPid(),
		constants.DataspaceProvider,
		false,
	)
}

// ContractTerminate sends a ContractTerminationMessage.
func (s *Server) ContractTerminate(
	_ context.Context,
	_ *dspcontrol.ContractTerminateRequest,
) (*dspcontrol.ContractTerminateResponse, error) {
	panic("not implemented") // TODO: Implement
}
