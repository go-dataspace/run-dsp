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
	"net/http"
	"time"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/logging"
	provider "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha2"
)

// CatalogError implements HTTPError for catalog requests.
type CatalogError struct {
	status  int
	dspCode string
	reason  string
	err     string
}

func (ce CatalogError) Error() string                       { return ce.err }
func (ce CatalogError) StatusCode() int                     { return ce.status }
func (ce CatalogError) ErrorType() string                   { return "dspace:CatalogError" }
func (ce CatalogError) DSPCode() string                     { return ce.dspCode }
func (ce CatalogError) Description() []shared.Multilanguage { return []shared.Multilanguage{} }
func (ce CatalogError) ProviderPID() string                 { return "" }
func (ce CatalogError) ConsumerPID() string                 { return "" }

func (ce CatalogError) Reason() []shared.Multilanguage {
	return []shared.Multilanguage{{Value: ce.reason, Language: "en"}}
}

func catalogError(err string, statusCode int, dspCode string, reason string) CatalogError {
	return CatalogError{
		status:  statusCode,
		dspCode: dspCode,
		reason:  reason,
		err:     err,
	}
}

func (dh *dspHandlers) getDataService() shared.DataService {
	id := dh.dataserviceID
	// If no ID found we just use a hardcoded one.
	if id == "" {
		id = "urn:uuid:09a761b1-8ab5-4d53-a57a-df52080298b4"
	}
	endpoint := dh.dataserviceEndpoint
	// Use an obvious wrong endpoint under example.org.
	if endpoint == "" {
		endpoint = "http://unknown.example.org"
	}
	return shared.DataService{
		Resource: shared.Resource{
			ID:   id,
			Type: "dcat:DataService",
		},
		EndpointURL: endpoint,
	}
}

func (ch *dspHandlers) catalogRequestHandler(w http.ResponseWriter, req *http.Request) error {
	logger := logging.Extract(req.Context())
	catalogReq, err := shared.DecodeValid[shared.CatalogRequestMessage](req)
	if err != nil {
		return catalogError("request did not validate", http.StatusBadRequest, "400", "Invalid request")
	}
	logger.Debug("Got catalog request", "req", catalogReq)
	// As the filter option is undefined, we will not fill anything
	resp, err := ch.provider.GetCatalogue(req.Context(), &provider.GetCatalogueRequest{})
	if err != nil {
		return grpcErrorHandler(err)
	}

	err = shared.EncodeValid(w, req, http.StatusOK, shared.CatalogAcknowledgement{
		Dataset: shared.Dataset{
			Resource: shared.Resource{
				ID:   "urn:uuid:3afeadd8-ed2d-569e-d634-8394a8836d57",
				Type: "dcat:Catalog",
				Keyword: []string{
					"dataspace",
					"run-dsp",
				},
			},
		},
		Context:  shared.GetDSPContext(),
		Datasets: processProviderCatalogue(resp.GetDatasets(), ch.getDataService()),
		Service:  []shared.DataService{ch.getDataService()},
	})
	if err != nil {
		logger.Error("failed to serve catalog after accepting", "err", err)
	}
	return nil
}

func (ch *dspHandlers) datasetRequestHandler(w http.ResponseWriter, req *http.Request) error {
	paramID := req.PathValue("id")
	if paramID == "" {
		return catalogError("no ID in path", http.StatusBadRequest, "400", "No ID given in path")
	}
	ctx, logger := logging.InjectLabels(req.Context(), "paramID", paramID)
	datasetReq, err := shared.DecodeValid[shared.DatasetRequestMessage](req)
	if err != nil {
		return catalogError(err.Error(), http.StatusBadRequest, "400", "Invalid dataset request")
	}
	logger.Debug("Got dataset request", "req", datasetReq)
	resp, err := ch.provider.GetDataset(ctx, &provider.GetDatasetRequest{
		DatasetId: paramID,
	})
	if err != nil {
		return grpcErrorHandler(err)
	}

	err = shared.EncodeValid(w, req, http.StatusOK, processProviderDataset(resp.GetDataset(), ch.getDataService()))
	if err != nil {
		logger.Error("failed to serve dataset", "err", err)
	}
	return nil
}

func processProviderDataset(pds *provider.Dataset, service shared.DataService) shared.Dataset {
	var checksum *shared.Checksum
	cs := pds.GetChecksum()
	if cs != nil {
		checksum = &shared.Checksum{
			Algorithm: cs.GetAlgorithm(),
			Value:     cs.GetValue(),
		}
	}
	ds := shared.Dataset{
		Resource: shared.Resource{
			ID:       shared.IDToURN(pds.GetId()),
			Type:     "dcat:Dataset",
			Title:    pds.GetTitle(),
			Issued:   pds.GetIssued().AsTime().Format(time.RFC3339),
			Modified: pds.GetModified().AsTime().Format(time.RFC3339),
			Keyword:  pds.GetKeywords(),
			Creator:  pds.GetCreator(),
		},
		Distribution: []shared.Distribution{{
			Type:           "dcat:Distribution",
			AccessService:  []shared.DataService{service},
			MediaType:      pds.GetMediaType(),
			License:        pds.GetLicense(),
			AccessRights:   pds.GetAccessRights(),
			Rights:         pds.GetRights(),
			ByteSize:       int(pds.GetByteSize()),
			CompressFormat: pds.GetCompressFormat(),
			PackageFormat:  pds.GetPackageFormat(),
			Checksum:       checksum,
		}},
	}

	for _, desc := range pds.GetDescription() {
		ds.Distribution[0].Description = append(ds.Distribution[0].Description, shared.Multilanguage{
			Value:    desc.GetValue(),
			Language: desc.GetLanguage(),
		})
	}

	if ck := pds.GetChecksum(); ck != nil {
		ds.Distribution[0].Checksum = &shared.Checksum{
			Algorithm: ck.GetAlgorithm(),
			Value:     ck.GetValue(),
		}
	}
	return ds
}

func processProviderCatalogue(gdc []*provider.Dataset, service shared.DataService) []shared.Dataset {
	datasets := make([]shared.Dataset, len(gdc))
	for i, f := range gdc {
		datasets[i] = processProviderDataset(f, service)
	}
	return datasets
}
