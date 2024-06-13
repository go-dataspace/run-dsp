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

// Package fsprovider is a provider for dataspace files.
// THIS IS MEANT FOR TESTING PURPOSES AND IS SUPER INSECURE
package fsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
)

const idDB = ".ids.json"

var ErrNotFound = errors.New("File not found")

type Adapter struct {
	path string
}

func New(path string) Adapter {
	return Adapter{path: path}
}

func (a Adapter) GetFileSet(ctx context.Context, citizenData *shared.CitizenData) ([]*shared.File, error) {
	logger := logging.Extract(ctx)
	userPath := path.Join(
		a.path,
		strconv.Itoa(citizenData.BirthDate.Year()),
		strconv.Itoa(int(citizenData.BirthDate.Month())),
		strconv.Itoa(citizenData.BirthDate.Day()),
		citizenData.LastName,
		citizenData.FirstName,
	)
	logger = logger.With("path", userPath)
	logger.Debug("Checking path for info")
	dirInfo, err := os.Stat(userPath)
	if err != nil {
		logger.Error("Couldn't stat path", "error", err)
		return nil, err
	}
	if !dirInfo.IsDir() {
		logger.Error("Path is not a directory")
		return nil, fmt.Errorf("Path is not a directory: %s", userPath)
	}

	files, err := getFiles(logger, userPath)
	if err != nil {
		return nil, err
	}
	return files, err
}

// GetFile gets a file by ID. Now done by just checking if it's in the returned files. Super
// inefficient.
func (a Adapter) GetFile(ctx context.Context, citizenData *shared.CitizenData, id uuid.UUID) (*shared.File, error) {
	files, err := a.GetFileSet(ctx, citizenData)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.ID == id {
			return f, nil
		}
	}
	return nil, ErrNotFound
}

func getIDs(logger *slog.Logger, userPath string) (map[string]uuid.UUID, error) {
	idPath := path.Join(userPath, idDB)
	_, err := os.Stat(idPath)
	ids := make(map[string]uuid.UUID)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Error("Could not stat ID file", "error", err)
			return nil, err
		}
	} else {
		idData, err := os.ReadFile(idPath)
		if err != nil {
			logger.Error("Could not read idFile", "error", err)
			return nil, err
		}
		err = json.Unmarshal(idData, &ids)
		if err != nil {
			logger.Error("Could not unmarshal idFile", "error", err)
			return nil, err
		}
	}
	return ids, nil
}

func saveIDs(logger *slog.Logger, dir string, ids map[string]uuid.UUID) error {
	j, err := json.Marshal(ids)
	if err != nil {
		logger.Error("Couldn't marshal IDs", "error", err)
		return err
	}
	idPath := path.Join(dir, idDB)
	err = os.WriteFile(idPath, j, 0o600)
	if err != nil {
		logger.Error("Couldn't write idDB", "error", err)
		return err
	}
	return nil
}

func getFiles(logger *slog.Logger, dir string) ([]*shared.File, error) {
	ids, err := getIDs(logger, dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	files := make([]*shared.File, 0)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == idDB {
			continue
		}
		id := uuid.New()
		if existing, ok := ids[entry.Name()]; ok {
			id = existing
		}
		info, err := entry.Info()
		if err != nil {
			logger.Error("Couldn't get file info", "file", entry.Name(), "error", err)
			return nil, err
		}
		mtype, err := mimetype.DetectFile(path.Join(dir, entry.Name()))
		if err != nil {
			logger.Error("Couldn't get file mimetype", "file", entry.Name(), "error", err)
			return nil, err
		}
		files = append(files, &shared.File{
			ID:       id,
			Name:     entry.Name(),
			Modified: info.ModTime().Format(time.RFC3339Nano),
			Format:   mtype.String(),
		})
		ids[entry.Name()] = id
	}

	err = saveIDs(logger, dir, ids)
	if err != nil {
		return nil, err
	}

	return files, nil
}
