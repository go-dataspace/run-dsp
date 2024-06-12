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

	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
)

// temporary data functions.
func getCatalog() CatalogAcknowledgement {
	return CatalogAcknowledgement{
		Dataset: Dataset{
			Resource: Resource{
				ID:   "urn:uuid:3afeadd8-ed2d-569e-d634-8394a8836d57",
				Type: "dcat:Catalog",
				Keyword: []string{
					"traffic",
					"government",
				},
				Description: []Multilanguage{{
					Value:    "A catalog of data items",
					Language: "en",
				}},
			},
		},
		Context:  jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Datasets: []Dataset{getDataset()},
		Service: []DataService{{
			Resource: Resource{
				ID:   "urn:uuid:4aa2dcc8-4d2d-569e-d634-8394a8834d77",
				Type: "dcat:DataService",
			},
			EndpointDescription: "dspace:connector",
			EndpointURL:         "https://provider-a.com/connector",
		}},
		ParticipantID: "urn:example:DataProviderA",
	}
}

func getDataset() Dataset {
	return Dataset{
		Resource: Resource{
			ID:   "urn:uuid:3dd1add8-4d2d-569e-d634-8394a8836a88",
			Type: "dcat:Dataset",
			Keyword: []string{
				"traffic",
			},
			Description: []Multilanguage{{
				Value:    "Traffic data sample extract",
				Language: "en",
			}},
			Title: "Traffic Data",
		},
		HasPolicy: []odrl.Offer{{
			MessageOffer: odrl.MessageOffer{
				PolicyClass: odrl.PolicyClass{
					ID:         "urn:uuid:25d74620-8f65-427c-9fc3-04ad1e19dc50",
					ProviderID: "http://example.com/Provider",
					Profile:    []odrl.Reference{},
					Permission: []odrl.Permission{{
						Action: "odrl:use",
						Constraint: []odrl.Constraint{{
							RightOperand: "odrl:EU",
							LeftOperand:  "odrl:spatial",
							Operator:     "odrl:EQ",
						}},
					}},
					Prohibiton: []any{},
					Obligation: []odrl.Duty{},
				},
				Type: "odrl:Offer",
			},
		}},
		Distribution: []Distribution{{
			Type:   "dcat:Distribution",
			Format: "dspace:s3+push",
			AccessService: []DataService{{
				Resource: Resource{
					ID:   "urn:uuid:4aa2dcc8-4d2d-569e-d634-8394a8834d77",
					Type: "dcat:DataService",
				},
				EndpointURL: "https://provider-a.com/connector",
			}},
		}},
	}
}

func catalogRequestHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	body, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	catalogReq, err := unmarshalAndValidate(req.Context(), body, CatalogRequestMessage{})
	if err != nil {
		logger.Error("Non validating catalog request", "error", err)
		returnError(w, http.StatusBadRequest, "Request did not validate")
		return
	}
	logger.Debug("Got catalog request", "req", catalogReq)

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, getCatalog())
}

func datasetRequestHandler(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	logger := logging.Extract(req.Context())
	paramID := req.PathValue("id")
	if paramID == "" {
		returnError(w, http.StatusBadRequest, "No ID given in path")
		return
	}
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	datasetReq, err := unmarshalAndValidate(req.Context(), body, DatasetRequestMessage{})
	if err != nil {
		logger.Error("Non validating dataset request", "error", err)
		returnError(w, http.StatusBadRequest, "Request did not validate")
		return
	}
	logger.Debug("Got dataset request", "req", datasetReq)
	validateMarshalAndReturn(req.Context(), w, http.StatusOK, getDataset())
}
