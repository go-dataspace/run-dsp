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

package fsprovider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/gabriel-vasile/mimetype"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
)

type offers struct {
	u map[uuid.UUID]*fileOffer
	p map[string]*fileOffer
	sync.RWMutex
}

func (o *offers) GetByUUID(id uuid.UUID) *fileOffer {
	defer o.RUnlock()
	o.RLock()
	if offer, ok := o.u[id]; ok {
		return offer
	}
	return nil
}

func (o *offers) GetByPathIdentifier(pid string) *fileOffer {
	defer o.RUnlock()
	o.RLock()
	if offer, ok := o.p[pid]; ok {
		return offer
	}
	return nil
}

func (o *offers) Put(fo fileOffer) {
	defer o.Unlock()
	o.Lock()
	o.u[fo.ID] = &fo
	o.p[fo.PathIdentifier] = &fo
}

func (o *offers) Del(id uuid.UUID) {
	defer o.Unlock()
	o.Lock()
	if fo, ok := o.u[id]; ok {
		delete(o.p, fo.PathIdentifier)
		delete(o.u, id)
	}
}

type fileOffer struct {
	ID             uuid.UUID
	FilePath       string
	Token          string
	PathIdentifier string
}

// This function has been made with not that much thought, it should be secure enough, but
// deserves an audit.
func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Publisher handles all the file serving operations.
type Publisher struct {
	ctx    context.Context
	store  offers
	prefix string
}

func NewPublisher(ctx context.Context, prefix string) *Publisher {
	return &Publisher{
		ctx: ctx,
		store: offers{
			u: make(map[uuid.UUID]*fileOffer),
			p: make(map[string]*fileOffer),
		},
		prefix: prefix,
	}
}

// Mux returns a mux that is ready to serve the files.
func (p *Publisher) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{path...}", p.ServeHTTP)
	return mux
}

// Publish publishes the files. It will generate a random identifier and token, and will
// then serve that file for as long as it's not Unpublished.
// The file path will look like <prefix>/<identifier>/<file_name>.
// It will return the path and the token.
func (p *Publisher) Publish(id uuid.UUID, filePath string) (string, string, error) {
	token, err := generateRandomString(64)
	if err != nil {
		return "", "", err
	}
	identifier, err := generateRandomString(32)
	if err != nil {
		return "", "", err
	}
	p.store.Put(fileOffer{
		ID:             id,
		FilePath:       filePath,
		Token:          token,
		PathIdentifier: identifier,
	})

	return path.Join(p.prefix, identifier, path.Base(filePath)), token, err
}

func (p *Publisher) Unpublish(id uuid.UUID) {
	p.store.Del(id)
}

// ServeHTTP is the handle function to serve the files.
// It will look up file offer and serve it if the token was in the header.
// Note that it will always return a 404 to make guessing the path harder.
func (p *Publisher) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger := logging.Extract(req.Context())
	token := extractTokenFromRequest(req)
	if token == "" {
		logger.Info("No token received")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	reqPath := req.PathValue("path")
	pathParts := strings.Split(reqPath, "/")
	if len(pathParts) != 2 {
		logger.Info("Malformed path")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	fo := p.store.GetByPathIdentifier(pathParts[0])
	if fo == nil || path.Base(fo.FilePath) != pathParts[1] || fo.Token != token {
		logger.Info("Bad request")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	serveFile(w, logger.With("file", fo.FilePath), fo.FilePath)
}

// serveFile serves up the file, and as authentication has succeeded at this point
// we will return non-404 errors.
func serveFile(w http.ResponseWriter, logger *slog.Logger, fp string) {
	logger.Info("Serving file")
	finfo, err := os.Stat(fp)
	if err != nil {
		logger.Error("Could not stat file", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	mtype, err := mimetype.DetectFile(fp)
	if err != nil {
		logger.Error("Could not detect mimetype", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Del("Content-Type")
	w.Header().Add("Content-Type", mtype.String())
	w.Header().Add("Content-Length", strconv.Itoa(int(finfo.Size())))
	fh, err := os.Open(fp)
	if err != nil {
		logger.Error("Could not open file", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, fh)
	if err != nil {
		logger.Error("Could not serve file", "error", err)
		return
	}
	logger.Info("Finished serving file")
}

func extractTokenFromRequest(req *http.Request) string {
	headerVal := req.Header.Get("Authorization")
	parts := strings.Split(headerVal, " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}
