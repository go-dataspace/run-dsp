// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package dspstatemachine

import "context"

type providerContractTasksService interface {
	SendContractOffer(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendContractAgreement(ctx context.Context, args ContractArgs) (ContractNegotiationMessageType, error)
	SendNegotiationFinalized(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendContractNegotiationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendTerminationMessage(ctx context.Context, args ContractArgs) (
		ContractNegotiationMessageType, error)
	SendErrorMessage(ctx context.Context, args ContractArgs) error
}
