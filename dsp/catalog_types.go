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
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/odrl"
)

// CatalogRequestMessage is a message to request a catalog. Note that the filter format is defined
// as "implementation specific" and nothing else in the spec.
// TODO: define how we want to support filters.
type CatalogRequestMessage struct {
	Context jsonld.Context `json:"@context"`
	Type    string         `json:"@type" validate:"required,eq=dspace:CatalogRequestMessage"`
	Filter  []any          `json:"dspace:filter"`
}

// DatasetRequestMessage is a message to request a dataset.
type DatasetRequestMessage struct {
	Context jsonld.Context `json:"@context"`
	Type    string         `json:"@type" validate:"required,eq=dspace:DatasetRequestMessage"`
	Dataset string         `json:"dspace:dataset" validate:"required"`
}

// CatalogAcknowledgement is an acknowledgement for a catalog, containing a dataset.
type CatalogAcknowledgement struct {
	Dataset
	Context       jsonld.Context `json:"@context"`
	Type          string         `json:"@type" validate:"required,eq=dcat:Catalog"`
	Datasets      []Dataset      `json:"dcat:dataset" validate:"gte=1,dive"`
	Service       []DataService  `json:"dcat:service" validate:"gte=1,dive"`
	ParticipantID string         `json:"dspace:participantID,omitempty"`
	Homepage      string         `json:"foaf:homepage,omitempty"`
}

// CatalogError is a standardised error for catalog requests.
type CatalogError struct {
	Context jsonld.Context   `json:"@context"`
	Type    string           `json:"@type" validate:"required,eq=dspace:CatalogError"`
	Code    string           `json:"dspace:code"`
	Reason  []map[string]any `json:"dspace:reason"`
}

// Dataset is a DCAT dataset.
type Dataset struct {
	Resource
	HasPolicy    []odrl.Offer   `json:"odrl:hasPolicy" validate:"required,gte=1,dive"`
	Distribution []Distribution `json:"dcat:distribution" validate:"gte=1,dive"`
}

// Resource is a DCAT resource.
type Resource struct {
	Keyword     []string        `json:"dcat:keyword,omitempty"`
	Theme       []Reference     `json:"dcat:them,omitempty" validate:"gte=1,dive"`
	ConformsTo  string          `json:"dct:conformsTo,omitempty"`
	Creator     string          `json:"dct:creator,omitempty"`
	Description []Multilanguage `json:"dct:description,omitempty" validate:"dive"`
	Identifier  string          `json:"dct:identifier,omitempty"`
	Issued      string          `json:"dct:issued,omitempty"`
	Modified    string          `json:"dct:modified,omitempty"`
	Title       string          `json:"dct:title,omitempty"`
}

// Distribution is a DCAT distribution.
type Distribution struct {
	Title         string          `json:"dct:title,omitempty"`
	Description   []Multilanguage `json:"dct:description,omitempty" validate:"dive"`
	Issued        string          `json:"dct:issued,omitempty"`
	Modified      string          `json:"dct:modified,omitempty"`
	HasPolicy     []odrl.Offer    `json:"odrl:hasPolicy" validate:"gte=1,dive"`
	AccessService []DataService   `json:"dcat:accessService" validate:"required,gte=1,dive"`
}

// DataService is a DCAT dataservice.
type DataService struct {
	Resource
	EndpointDescription string    `json:"dcat:endpointDescription,omitempty"`
	EndpointURL         string    `json:"dcat:endpointURL,omitempty"`
	ServesDataset       []Dataset `json:"dcat:servesDataset,omitempty"`
}

// Reference is a DCAT reference.
type Reference struct {
	ID string `json:"@id" validate:"required"`
}
