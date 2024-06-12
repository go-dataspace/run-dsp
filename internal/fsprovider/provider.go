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

type Adapter struct {
	path string
}

func New(path string) Adapter {
	return Adapter{path: path}
}

func (a Adapter) GetFileSet(ctx context.Context, citizenData *shared.CitizenData) (shared.Fileset, error) {
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
		return shared.Fileset{}, err
	}
	if !dirInfo.IsDir() {
		logger.Error("Path is not a directory")
		return shared.Fileset{}, fmt.Errorf("Path is not a directory: %s", userPath)
	}

	idPath := path.Join(userPath, ".id")
	_, err = os.Stat(idPath)
	if err != nil {
		if os.IsNotExist(err) {
			id := uuid.New()
			if err := os.WriteFile(idPath, []byte(id.String()), 0o600); err != nil {
				logger.Error("Could not write id file", "error", err)
				return shared.Fileset{}, err
			}
		} else {
			logger.Error("Could not stat ID file", "error", err)
			return shared.Fileset{}, err
		}
	}
	strId, err := os.ReadFile(idPath)
	if err != nil {
		logger.Error("Could not read id file", "error", err)
		return shared.Fileset{}, err
	}

	id := uuid.MustParse(string(strId))
	files, err := getFiles(logger, userPath)
	if err != nil {
		return shared.Fileset{}, err
	}
	return shared.Fileset{
		ID:          id,
		Title:       fmt.Sprintf("Hosted files"),
		Description: "Hosted files",
		Keywords:    []string{"dataloft", "medical"},
		Files:       files,
	}, err
}

func getFiles(logger *slog.Logger, dir string) ([]shared.File, error) {
	entries, err := os.ReadDir(dir)
	files := make([]shared.File, 0)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == ".id" {
			continue
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
		files = append(files, shared.File{
			Name:     entry.Name(),
			Modified: info.ModTime().Format(time.RFC3339Nano),
			Format:   mtype.String(),
		})
	}
	return files, nil
}
