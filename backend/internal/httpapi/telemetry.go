package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var httpLatencyBucketsMS = [...]int64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

type runtimeSLI struct {
	mu        sync.Mutex
	requests  map[httpMetricKey]*httpMetric
	readiness map[readinessMetricKey]uint64
}

type httpMetricKey struct {
	Method string
	Route  string
	Status int
}

type httpMetric struct {
	Count        uint64
	DurationMS   float64
	BucketCounts [len(httpLatencyBucketsMS)]uint64
	InfCount     uint64
}

type readinessMetricKey struct {
	Result string
	Reason string
}

func newRuntimeSLI() *runtimeSLI {
	return &runtimeSLI{
		requests:  make(map[httpMetricKey]*httpMetric),
		readiness: make(map[readinessMetricKey]uint64),
	}
}

func (s *runtimeSLI) observeHTTP(method, route string, status int, duration time.Duration) {
	if s == nil {
		return
	}
	if route == "" {
		route = "UNMATCHED"
	}
	key := httpMetricKey{Method: method, Route: route, Status: status}
	durationMS := float64(duration) / float64(time.Millisecond)

	s.mu.Lock()
	defer s.mu.Unlock()
	metric := s.requests[key]
	if metric == nil {
		metric = &httpMetric{}
		s.requests[key] = metric
	}
	metric.Count++
	metric.DurationMS += durationMS
	metric.InfCount++
	for i, bucket := range httpLatencyBucketsMS {
		if durationMS <= float64(bucket) {
			metric.BucketCounts[i]++
		}
	}
}

func (s *runtimeSLI) observeReadiness(result, reason string) {
	if s == nil {
		return
	}
	if reason == "" {
		reason = "NONE"
	}
	s.mu.Lock()
	s.readiness[readinessMetricKey{Result: result, Reason: reason}]++
	s.mu.Unlock()
}

func metricEscape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

func (s *runtimeSLI) render() string {
	s.mu.Lock()
	requests := make(map[httpMetricKey]httpMetric, len(s.requests))
	for key, value := range s.requests {
		if value != nil {
			requests[key] = *value
		}
	}
	readiness := make(map[readinessMetricKey]uint64, len(s.readiness))
	for key, value := range s.readiness {
		readiness[key] = value
	}
	s.mu.Unlock()

	var out strings.Builder
	out.WriteString("# HELP apgic_http_requests_total HTTP requests observed by canonical route and status.\n")
	out.WriteString("# TYPE apgic_http_requests_total counter\n")
	out.WriteString("# HELP apgic_http_request_duration_milliseconds HTTP request latency histogram by canonical route and status.\n")
	out.WriteString("# TYPE apgic_http_request_duration_milliseconds histogram\n")

	httpKeys := make([]httpMetricKey, 0, len(requests))
	for key := range requests {
		httpKeys = append(httpKeys, key)
	}
	sort.Slice(httpKeys, func(i, j int) bool {
		if httpKeys[i].Route != httpKeys[j].Route {
			return httpKeys[i].Route < httpKeys[j].Route
		}
		if httpKeys[i].Method != httpKeys[j].Method {
			return httpKeys[i].Method < httpKeys[j].Method
		}
		return httpKeys[i].Status < httpKeys[j].Status
	})
	for _, key := range httpKeys {
		metric := requests[key]
		labels := fmt.Sprintf(
			"method=\"%s\",route=\"%s\",status=\"%d\"",
			metricEscape(key.Method), metricEscape(key.Route), key.Status,
		)
		fmt.Fprintf(&out, "apgic_http_requests_total{%s} %d\n", labels, metric.Count)
		for i, bucket := range httpLatencyBucketsMS {
			fmt.Fprintf(
				&out,
				"apgic_http_request_duration_milliseconds_bucket{%s,le=\"%d\"} %d\n",
				labels, bucket, metric.BucketCounts[i],
			)
		}
		fmt.Fprintf(
			&out,
			"apgic_http_request_duration_milliseconds_bucket{%s,le=\"+Inf\"} %d\n",
			labels, metric.InfCount,
		)
		fmt.Fprintf(&out, "apgic_http_request_duration_milliseconds_sum{%s} %.6f\n", labels, metric.DurationMS)
		fmt.Fprintf(&out, "apgic_http_request_duration_milliseconds_count{%s} %d\n", labels, metric.Count)
	}

	out.WriteString("# HELP apgic_readiness_checks_total Readiness decisions by result and dependency reason.\n")
	out.WriteString("# TYPE apgic_readiness_checks_total counter\n")
	readinessKeys := make([]readinessMetricKey, 0, len(readiness))
	for key := range readiness {
		readinessKeys = append(readinessKeys, key)
	}
	sort.Slice(readinessKeys, func(i, j int) bool {
		if readinessKeys[i].Result != readinessKeys[j].Result {
			return readinessKeys[i].Result < readinessKeys[j].Result
		}
		return readinessKeys[i].Reason < readinessKeys[j].Reason
	})
	for _, key := range readinessKeys {
		fmt.Fprintf(
			&out,
			"apgic_readiness_checks_total{result=\"%s\",reason=\"%s\"} %d\n",
			metricEscape(key.Result), metricEscape(key.Reason), readiness[key],
		)
	}
	return out.String()
}

type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusCapturingWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCapturingWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func observeRuntimeSLI(next http.Handler, sli *runtimeSLI) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		capture := &statusCapturingWriter{ResponseWriter: w}
		next.ServeHTTP(capture, r)
		status := capture.status
		if status == 0 {
			status = http.StatusOK
		}
		sli.observeHTTP(r.Method, r.Pattern, status, time.Since(started))
	})
}
