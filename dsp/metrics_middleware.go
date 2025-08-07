// Copyright 2025 go-dataspace
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
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/urfave/negroni"
)

var buckets = prometheus.ExponentialBuckets(0.1, 1.5, 5)

var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Tracks the number of HTTP requests.",
		}, []string{"method", "code", "path", "host"},
	)
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Tracks the latencies for HTTP requests.",
			Buckets: buckets,
		},
		[]string{"method", "code", "path", "host"},
	)
	requestSize = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_request_size_bytes",
			Help: "Tracks the size of HTTP requests.",
		},
		[]string{"method", "code", "path", "host"},
	)
	responseSize = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_response_size_bytes",
			Help: "Tracks the size of HTTP responses.",
		},
		[]string{"method", "code", "path", "host"},
	)
)

func WrapHandlerWithMetrics(path string, handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := negroni.NewResponseWriter(w)
		handler.ServeHTTP(lrw, r)
		elapsed := time.Since(start)
		labelsValues := []string{
			fmt.Sprint(lrw.Status()),
			r.Method,
			path,
			r.Host,
		}
		requestsTotal.WithLabelValues(labelsValues...).Inc()
		requestDuration.WithLabelValues(labelsValues...).Observe(float64(elapsed.Seconds()))
		requestSize.WithLabelValues(labelsValues...).Observe(float64(r.ContentLength))
		responseSize.WithLabelValues(labelsValues...).Observe(float64(lrw.Size()))
	}
}
