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
	"fmt"
	"io"
	"net/http"

	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/internal/auth"
	"github.com/go-dataspace/run-dsp/internal/constants"
	"github.com/go-dataspace/run-dsp/jsonld"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
)

var dataService = shared.DataService{
	Resource: shared.Resource{
		ID:   "urn:uuid:7acb5d82-33b0-47c0-a22b-2fc470c8e3cb",
		Type: "dcat:DataService",
	},
	EndpointURL: "https://insert-url-here.dsp/",
}

func fileToDataset(file *shared.File, service shared.DataService) shared.Dataset {
	ds := shared.Dataset{
		Resource: shared.Resource{
			ID:         fmt.Sprintf("urn:uuid:%s", file.ID.String()),
			Type:       "dcat:Dataset",
			Title:      file.Name,
			Modified:   file.Modified,
			ConformsTo: file.Format,
		},
		Distribution: []shared.Distribution{{
			Type:          "dcat:Distribution",
			Format:        "dspace:https+push",
			AccessService: []shared.DataService{service},
		}},
	}
	return ds
}

func filesToDatasets(files []*shared.File, service shared.DataService) []shared.Dataset {
	datasets := make([]shared.Dataset, len(files))
	for i, f := range files {
		datasets[i] = fileToDataset(f, service)
	}
	return datasets
}

func (ch *dspHandlers) catalogRequestHandler(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	body, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	catalogReq, err := shared.UnmarshalAndValidate(req.Context(), body, shared.CatalogRequestMessage{})
	if err != nil {
		logger.Error("Non validating catalog request", "error", err)
		returnError(w, http.StatusBadRequest, "Request did not validate")
		return
	}
	logger.Debug("Got catalog request", "req", catalogReq)
	ui := auth.ExtractUserInfo(req.Context())
	// As there's no filter option yet, we don't need anything from the catalog request.
	fileSet, err := ch.provider.GetFileSet(req.Context(), &shared.CitizenData{
		FirstName: ui.FirstName,
		LastName:  ui.Lastname,
		BirthDate: ui.BirthDate,
	})
	if err != nil {
		returnError(w, http.StatusInternalServerError, "could not get catalog")
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, shared.CatalogAcknowledgement{
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
		Context:  jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}}),
		Datasets: filesToDatasets(fileSet, dataService),
		Service:  []shared.DataService{dataService},
	})
}

func (ch *dspHandlers) datasetRequestHandler(w http.ResponseWriter, req *http.Request) {
	paramID := req.PathValue("id")
	if paramID == "" {
		returnError(w, http.StatusBadRequest, "No ID given in path")
		return
	}
	ctx, logger := logging.InjectLabels(req.Context(), "paramID", paramID)
	id, err := uuid.Parse(paramID)
	if err != nil {
		logger.Error("Misformed uuid in path", "error", err)
		returnError(w, http.StatusBadRequest, "Invalid ID")
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Could not read body")
		return
	}
	datasetReq, err := shared.UnmarshalAndValidate(ctx, body, shared.DatasetRequestMessage{})
	if err != nil {
		logger.Error("Non validating dataset request", "error", err)
		returnError(w, http.StatusBadRequest, "Request did not validate")
		return
	}
	logger.Debug("Got dataset request", "req", datasetReq)
	ui := auth.ExtractUserInfo(ctx)
	// As there's no filter option yet, we don't need anything from the catalog request.
	file, err := ch.provider.GetFile(ctx, &shared.CitizenData{
		FirstName: ui.FirstName,
		LastName:  ui.Lastname,
		BirthDate: ui.BirthDate,
	}, id)
	if err != nil {
		returnError(w, http.StatusInternalServerError, "could not get dataset")
		return
	}

	validateMarshalAndReturn(req.Context(), w, http.StatusOK, fileToDataset(file, dataService))
}
