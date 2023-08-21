package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Used as key for radius metrics, both in storage and as a way to specify queries,
// where the fields with non zero values will be used for aggregation
type RadiusMetricKey struct {
	// <ipaddress>:<port> or <ipaddress>
	Endpoint string
	// Radius code
	Code string
}

type RadiusMetrics map[RadiusMetricKey]uint64

// Custom marshalling of RadiusMetrics
func (rm RadiusMetrics) MarshalJSON() ([]byte, error) {

	// JSON object will have a property for the key and another one for the value
	type T struct {
		Key   RadiusMetricKey
		Value uint64
	}

	// The array of T to produce as JSON
	metrics := make([]T, len(rm))

	for m, v := range rm {
		metrics = append(metrics, T{Key: m, Value: v})
	}

	return json.Marshal(metrics)
}

// Builder for Prometheus format export
func (rm RadiusMetrics) genPrometheusMetric(metricName string, helpString string) string {
	var builder strings.Builder
	if len(rm) > 0 {
		builder.WriteString(fmt.Sprintf("HELP %s %s\n", metricName, helpString))
		builder.WriteString(fmt.Sprintf("TYPE %s counter\n", metricName))
	}
	for k, v := range rm {
		builder.WriteString(fmt.Sprintf("%s{endpoint=\"%s\",code=\"%s\"} %d\n",
			metricName, k.Endpoint, k.Code, v))
	}

	return builder.String()
}

// Radius Server

type RadiusServerRequestEvent struct {
	Key RadiusMetricKey
}

func IncrementRadiusServerRequest(endpoint string, Code string) {
	MS.metricEventChan <- RadiusServerRequestEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusServerResponseEvent struct {
	Key RadiusMetricKey
}

func IncrementRadiusServerResponse(endpoint string, Code string) {
	MS.metricEventChan <- RadiusServerResponseEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusServerDropEvent struct {
	Key RadiusMetricKey
}

func IncrementRadiusServerDrop(endpoint string, Code string) {
	MS.metricEventChan <- RadiusServerDropEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

// Radius Client

type RadiusClientRequestEvent struct {
	Key RadiusMetricKey
}

func IncrementRadiusClientRequest(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientRequestEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseEvent struct {
	Key RadiusMetricKey
}

func IncrementRadiusClientResponse(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientResponseEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientTimeoutEvent struct {
	Key RadiusMetricKey
}

func IncrementRadiusClientTimeout(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientTimeoutEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseStalledEvent struct {
	Key RadiusMetricKey
}

func IncrementRadiusClientResponseStalled(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientResponseStalledEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseDropEvent struct {
	Key RadiusMetricKey
}

func IncrementRadiusClientResponseDrop(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientResponseDropEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

// Instrumentation of Diameter Peers table
type RadiusServerTableEntry struct {
	ServerName       string
	IsAvailable      bool
	UnavailableUntil time.Time
}

type RadiusServersTable []RadiusServerTableEntry

type RadiusServersTableUpdatedEvent struct {
	InstanceName string
	Table        RadiusServersTable
}

func IncrementRadiusServersTable(instanceName string, table RadiusServersTable) {
	MS.metricEventChan <- RadiusServersTableUpdatedEvent{InstanceName: instanceName, Table: table}
}
