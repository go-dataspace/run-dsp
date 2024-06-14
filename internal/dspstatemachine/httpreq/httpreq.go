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

// Package httpreq contains a convenience function for http requests.
package httpreq

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-dataspace/run-dsp/logging"
)

const (
	defaultRetryDuration = 2 * time.Minute
)

var client = &http.Client{}

// Request represents a http request with retrying capabilities.
type Request struct {
	Context             context.Context
	Method              string
	URL                 string
	Body                io.Reader
	RequestConfigure    func(*http.Request)
	StatusCodeCheck     func(status int) bool
	UpdateActivity      func()
	Cancel              func()
	PermanentErrorCheck func(status int, body []byte) bool
	Duration            time.Duration
}

type respBody struct {
	Response *http.Response
	Body     []byte
}

// RequestOption is a function that modifies a Request.
type RequestOption func(*Request)

func defaultRequest() Request {
	return Request{
		StatusCodeCheck: func(code int) bool { return code == http.StatusOK },
		Duration:        defaultRetryDuration,
	}
}

// New creates a new Request.
func New(ctx context.Context, method, url string, opts ...RequestOption) *Request {
	r := defaultRequest()
	r.Context = ctx
	r.Method = method
	r.URL = url
	for _, opt := range opts {
		opt(&r)
	}

	return &r
}

// WithBody sets the body of the request.
func WithBody(body io.Reader) RequestOption {
	return func(r *Request) {
		r.Body = body
	}
}

// WithRequestConfigure sets the request configuration function.
func WithRequestConfigure(configureFunc func(*http.Request)) RequestOption {
	return func(r *Request) {
		r.RequestConfigure = configureFunc
	}
}

// WithStatusCodeCheck sets the status code check function.
func WithStatusCodeCheck(checkFunc func(int) bool) RequestOption {
	return func(r *Request) {
		r.StatusCodeCheck = checkFunc
	}
}

// WithPermanentErrorCheck sets the permanent error check function.
// If the given function returns true, the request will error out and not retry.
func WithPermanentErrorCheck(checkFunc func(int, []byte) bool) RequestOption {
	return func(r *Request) {
		r.PermanentErrorCheck = checkFunc
	}
}

// WithUpdateActivity sets the update activity function.
func WithUpdateActivity(updateFunc func()) RequestOption {
	return func(r *Request) {
		r.UpdateActivity = updateFunc
	}
}

// WithCancel sets the cancel function.
func WithCancel(cancelFunc func()) RequestOption {
	return func(r *Request) {
		r.Cancel = cancelFunc
	}
}

// WithDuration sets how long to keep retrying the request.
func WithDuration(d time.Duration) RequestOption {
	return func(r *Request) {
		r.Duration = d
	}
}

// Do executes the request.
func (r *Request) Do() ([]byte, error) {
	logger := logging.Extract(r.Context).With("method", r.Method, "url", r.URL)

	reqFunc := func() (respBody, error) {
		req, err := http.NewRequest(r.Method, r.URL, r.Body)
		if err != nil {
			return respBody{}, err
		}

		if r.RequestConfigure != nil {
			r.RequestConfigure(req)
		}

		resp, err := client.Do(req)
		if err != nil {
			return respBody{}, err
		}
		defer resp.Body.Close()

		rBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return respBody{}, err
		}

		err = r.checkResponse(logger, resp.StatusCode, rBody)

		return respBody{Response: resp, Body: rBody}, err
	}
	errFunc := func(err error, d time.Duration) {
		logger.Error("Error, retrying", "delay", d, "error", err)
	}

	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = r.Duration
	bctx := backoff.WithContext(b, r.Context)
	bctx.Reset()
	respBody, err := backoff.RetryNotifyWithData(reqFunc, bctx, errFunc)
	if err != nil {
		if r.Cancel != nil {
			r.Cancel()
		}
		return nil, err
	}
	return respBody.Body, nil
}

func (r *Request) checkResponse(logger *slog.Logger, status int, body []byte) error {
	if r.PermanentErrorCheck != nil && r.PermanentErrorCheck(status, body) {
		logger.Debug("Permanent error check failed, not retrying",
			"status_code", status,
			"body", string(body),
		)
		return &backoff.PermanentError{Err: fmt.Errorf("permanent error: %d", status)}
	}

	if !r.StatusCodeCheck(status) {
		logger.Debug("Status code check failed, retrying",
			"status_code", status,
			"body", string(body),
		)
		return fmt.Errorf(
			"unexpected status code: %d", status,
		)
	}

	return nil
}
