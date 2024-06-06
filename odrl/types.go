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

// Package odrl contains ODRL code
package odrl

//nolint:lll
// This is for now a partial port of this JSON schema:
// https://international-data-spaces-association.github.io/ids-specification/2024-1/negotiation/message/schema/contract-schema.json

// Offer is an ODRL offer.
type Offer struct {
	MessageOffer
}

// MessageOffer is an ODRL MessageOffer.
type MessageOffer struct {
	PolicyClass
	Type string `json:"@type" validate:"required,eq=odrl:Offer"`
}

// PolicyClass is an ODRL PolicyClass.
type PolicyClass struct {
	AbstractPolicyRule
	ID         string       `json:"@id" verification:"required"`
	Profile    []Reference  `json:"odrl:profile,omitempty" verification:"dive"`
	Permission []Permission `json:"odrl:permission,omitempty" verification:"gte=1,dive"`
	Obligation []Duty       `json:"odrl:obligation,omitempty" verification:"gte=1,dive"`
}

// AbstractPolicyRule defines an ODRL abstract policy rule.
type AbstractPolicyRule struct {
	Assigner string `json:"assigner,omitempty"`
	Assignee string `json:"assignee,omitempty"`
}

// Reference is a reference.
type Reference struct {
	ID string `json:"@id" verification:"required"`
}

// Permission is a permisson entry.
type Permission struct {
	AbstractPolicyRule
	Action     string       `json:"action" verification:"required"` // TODO custom verifier.
	Constraint []Constraint `json:"constraint,omitempty" verification:"gte=1,dive"`
	Duty       Duty         `json:"duty,omitempty" verification:"dive"`
}

// Duty is an ODRL duty.
type Duty struct {
	AbstractPolicyRule
	ID         string       `json:"@id"`
	Action     string       `json:"action" verification:"required"` // TODO: custom verifier.
	Constraint []Constraint `json:"constraint,omitempty" verification:"gte=1,dive"`
}

// Constraint is an ODRL constraint.
type Constraint struct {
	RightOperand          string // TODO: implement custom verifier
	RightOperandReference Reference
	LeftOperand           string // TODO: implement custom verifier.
	Operator              string // TODO: implment custom verifier.
}
