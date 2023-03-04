package core

import (
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

		fmt.Println("***************************")
		fmt.Println(k.Code)
		fmt.Println(fmt.Sprintf("-%s-", k.Code))
		fmt.Println("***************************")
	}

	return builder.String()
}

// Radius Server

type RadiusServerRequestEvent struct {
	Key RadiusMetricKey
}

func PushRadiusServerRequest(endpoint string, Code string) {
	MS.metricEventChan <- RadiusServerRequestEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusServerResponseEvent struct {
	Key RadiusMetricKey
}

func PushRadiusServerResponse(endpoint string, Code string) {
	MS.metricEventChan <- RadiusServerResponseEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusServerDropEvent struct {
	Key RadiusMetricKey
}

func PushRadiusServerDrop(endpoint string, Code string) {
	MS.metricEventChan <- RadiusServerDropEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

// Radius Client

type RadiusClientRequestEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientRequest(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientRequestEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientResponse(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientResponseEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientTimeoutEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientTimeout(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientTimeoutEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseStalledEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientResponseStalled(endpoint string, Code string) {
	MS.metricEventChan <- RadiusClientResponseStalledEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseDropEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientResponseDrop(endpoint string, Code string) {
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

func PushRadiusServersTable(instanceName string, table RadiusServersTable) {
	MS.metricEventChan <- RadiusServersTableUpdatedEvent{InstanceName: instanceName, Table: table}
}
