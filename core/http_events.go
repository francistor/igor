package core

import (
	"fmt"
	"strings"
)

//////////////////////////////////////////////////////////

type HttpClientMetricKey struct {
	Endpoint  string
	ErrorCode string
}

type HttpClientMetrics map[HttpClientMetricKey]uint64

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

func PushHttpClientExchange(endpoint string, errorCode string) {
	MS.metricEventChan <- HttpClientExchangeEvent{Key: HttpClientMetricKey{Endpoint: endpoint, ErrorCode: errorCode}}
}

//////////////////////////////////////////////////////////

type HttpHandlerMetricKey struct {
	Path      string
	ErrorCode string
}

type HttpHandlerMetrics map[HttpHandlerMetricKey]uint64

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

func PushHttpHandlerExchange(errorCode string, path string) {
	MS.metricEventChan <- HttpHandlerExchangeEvent{Key: HttpHandlerMetricKey{ErrorCode: errorCode, Path: path}}
}

//////////////////////////////////////////////////////////

type HttpRouterMetricKey struct {
	Path      string
	ErrorCode string
}

type HttpRouterMetrics map[HttpRouterMetricKey]uint64

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

func PushHttpRouterExchange(errorCode string, path string) {
	MS.metricEventChan <- HttpRouterExchangeEvent{Key: HttpRouterMetricKey{ErrorCode: errorCode, Path: path}}
}
