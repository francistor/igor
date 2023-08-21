package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

//////////////////////////////////////////////////////////

type SessionQueryMetricKey struct {
	Path      string
	IndexName string
	ErrorCode string
}

type SessionQueryMetrics map[SessionQueryMetricKey]uint64

// Custom marshalling
// Produce {"key": {"endpoint": "<>", "indexName": "<>", "errorCode":"<>"}, "value": <>}
func (ssm SessionQueryMetrics) MarshalJSON() ([]byte, error) {

	// JSON object will have a property for the key and another one for the value
	type T struct {
		Key   SessionQueryMetricKey
		Value uint64
	}

	// The array of T to produce as JSON
	metrics := make([]T, 0)

	for m, v := range ssm {
		metrics = append(metrics, T{Key: m, Value: v})
	}

	return json.Marshal(metrics)
}

// Builder for Prometheus format export
func (sqm SessionQueryMetrics) genPrometheusMetric(metricName string, helpString string) string {
	var builder strings.Builder
	if len(sqm) > 0 {
		builder.WriteString(fmt.Sprintf("HELP %s %s\n", metricName, helpString))
		builder.WriteString(fmt.Sprintf("TYPE %s counter\n", metricName))
	}
	for k, v := range sqm {
		builder.WriteString(fmt.Sprintf("%s{path=\"%s\",indexname=\"%s\",errorcode=\"%s\"} %d\n",
			metricName, k.Path, k.IndexName, k.ErrorCode, v))
	}

	return builder.String()
}

type SessionQueryEvent struct {
	Key SessionQueryMetricKey
}

func IncrementSessionQueries(path string, indexname string, errorCode string) {
	MS.metricEventChan <- SessionQueryEvent{Key: SessionQueryMetricKey{Path: path, IndexName: indexname, ErrorCode: errorCode}}
}

//////////////////////////////////////////////////////////

type SessionUpdateMetricKey struct {
	Endpoint string
}

type SessionUpdateMetrics map[SessionUpdateMetricKey]uint64

// Custom marshalling
func (sum SessionUpdateMetrics) MarshalJSON() ([]byte, error) {

	// JSON object will have a property for the key and another one for the value
	type T struct {
		Key   SessionUpdateMetricKey
		Value uint64
	}

	// The array of T to produce as JSON
	metrics := make([]T, 0)

	for m, v := range sum {
		metrics = append(metrics, T{Key: m, Value: v})
	}

	return json.Marshal(metrics)
}

// Builder for Prometheus format export
func (sqm SessionUpdateMetrics) genPrometheusMetric(metricName string, helpString string) string {
	var builder strings.Builder
	if len(sqm) > 0 {
		builder.WriteString(fmt.Sprintf("HELP %s %s\n", metricName, helpString))
		builder.WriteString(fmt.Sprintf("TYPE %s counter\n", metricName))
	}
	for k, v := range sqm {
		builder.WriteString(fmt.Sprintf("%s{endpoint=\"%s\"} %d\n",
			metricName, k.Endpoint, v))
	}

	return builder.String()
}

type SessionUpdateEvent struct {
	Key SessionUpdateMetricKey
}

func IncrementSessionUpdateQueries(endpoint string) {
	MS.metricEventChan <- SessionUpdateEvent{Key: SessionUpdateMetricKey{Endpoint: endpoint}}
}
