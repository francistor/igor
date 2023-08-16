package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

//////////////////////////////////////////////////////////

type HttpClientMetricKey struct {
	Endpoint  string
	ErrorCode string
}

type HttpClientMetrics map[HttpClientMetricKey]uint64

// Custom marshalling
func (hc HttpClientMetrics) MarshalJSON() ([]byte, error) {

	// JSON object will have a property for the key and another one for the value
	type T struct {
		Key   HttpClientMetricKey
		Value uint64
	}

	// The array of T to produce as JSON
	metrics := make([]T, 0)

	for m, v := range hc {
		metrics = append(metrics, T{Key: m, Value: v})
	}

	return json.Marshal(metrics)
}

// Builder for Prometheus format export
func (hcm HttpClientMetrics) genPrometheusMetric(metricName string, helpString string) string {
	var builder strings.Builder
	if len(hcm) > 0 {
		builder.WriteString(fmt.Sprintf("HELP %s %s\n", metricName, helpString))
		builder.WriteString(fmt.Sprintf("TYPE %s counter\n", metricName))
	}
	for k, v := range hcm {
		builder.WriteString(fmt.Sprintf("%s{endpoint=\"%s\",errorcode=\"%s\"} %d\n",
			metricName, k.Endpoint, k.ErrorCode, v))
	}

	return builder.String()
}

type HttpClientExchangeEvent struct {
	Key HttpClientMetricKey
}

func IncrementHttpClientExchange(endpoint string, errorCode string) {
	MS.metricEventChan <- HttpClientExchangeEvent{Key: HttpClientMetricKey{Endpoint: endpoint, ErrorCode: errorCode}}
}

//////////////////////////////////////////////////////////

type HttpHandlerMetricKey struct {
	Path      string
	ErrorCode string
}

type HttpHandlerMetrics map[HttpHandlerMetricKey]uint64

// Custom marshalling
func (hc HttpHandlerMetrics) MarshalJSON() ([]byte, error) {

	// JSON object will have a property for the key and another one for the value
	type T struct {
		Key   HttpHandlerMetricKey
		Value uint64
	}

	// The array of T to produce as JSON
	metrics := make([]T, 0)

	for m, v := range hc {
		metrics = append(metrics, T{Key: m, Value: v})
	}

	return json.Marshal(metrics)
}

// Builder for Prometheus format export
func (hhm HttpHandlerMetrics) genPrometheusMetric(metricName string, helpString string) string {
	var builder strings.Builder
	if len(hhm) > 0 {
		builder.WriteString(fmt.Sprintf("HELP %s %s\n", metricName, helpString))
		builder.WriteString(fmt.Sprintf("TYPE %s counter\n", metricName))
	}
	for k, v := range hhm {
		builder.WriteString(fmt.Sprintf("%s{path=\"%s\",errorcode=\"%s\"} %d\n",
			metricName, k.Path, k.ErrorCode, v))
	}

	return builder.String()
}

type HttpHandlerExchangeEvent struct {
	Key HttpHandlerMetricKey
}

func IncrementHttpHandlerExchange(errorCode string, path string) {
	MS.metricEventChan <- HttpHandlerExchangeEvent{Key: HttpHandlerMetricKey{ErrorCode: errorCode, Path: path}}
}

//////////////////////////////////////////////////////////

type HttpRouterMetricKey struct {
	Path      string
	ErrorCode string
}

type HttpRouterMetrics map[HttpRouterMetricKey]uint64

// Custom marshalling
func (hc HttpRouterMetrics) MarshalJSON() ([]byte, error) {

	// JSON object will have a property for the key and another one for the value
	type T struct {
		Key   HttpRouterMetricKey
		Value uint64
	}

	// The array of T to produce as JSON
	metrics := make([]T, 0)

	for m, v := range hc {
		metrics = append(metrics, T{Key: m, Value: v})
	}

	return json.Marshal(metrics)
}

// Builder for Prometheus format export
func (hrm HttpRouterMetrics) genPrometheusMetric(metricName string, helpString string) string {
	var builder strings.Builder
	if len(hrm) > 0 {
		builder.WriteString(fmt.Sprintf("HELP %s %s\n", metricName, helpString))
		builder.WriteString(fmt.Sprintf("TYPE %s counter\n", metricName))
	}
	for k, v := range hrm {
		builder.WriteString(fmt.Sprintf("%s{path=\"%s\",errorcode=\"%s\"} %d\n",
			metricName, k.Path, k.ErrorCode, v))
	}

	return builder.String()
}

type HttpRouterExchangeEvent struct {
	Key HttpRouterMetricKey
}

func IncrementHttpRouterExchange(errorCode string, path string) {
	// Strip the querystring to the path
	pos := strings.IndexRune(path, '?')
	if pos >= 0 {
		path = path[:pos]
	}
	MS.metricEventChan <- HttpRouterExchangeEvent{Key: HttpRouterMetricKey{ErrorCode: errorCode, Path: path}}
}
