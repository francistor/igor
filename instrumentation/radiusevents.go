package instrumentation

import (
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

// Radius Server

type RadiusServerRequestEvent struct {
	Key RadiusMetricKey
}

func PushRadiusServerRequest(endpoint string, Code string) {
	MS.InputChan <- RadiusServerRequestEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusServerResponseEvent struct {
	Key RadiusMetricKey
}

func PushRadiusServerResponse(endpoint string, Code string) {
	MS.InputChan <- RadiusServerResponseEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusServerDropEvent struct {
	Key RadiusMetricKey
}

func PushRadiusServerDrop(endpoint string, Code string) {
	MS.InputChan <- RadiusServerDropEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

// Radius Client

type RadiusClientRequestEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientRequest(endpoint string, Code string) {
	MS.InputChan <- RadiusClientRequestEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientResponse(endpoint string, Code string) {
	MS.InputChan <- RadiusClientResponseEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientTimeoutEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientTimeout(endpoint string, Code string) {
	MS.InputChan <- RadiusClientTimeoutEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseStalledEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientResponseStalled(endpoint string, Code string) {
	MS.InputChan <- RadiusClientResponseStalledEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
}

type RadiusClientResponseDropEvent struct {
	Key RadiusMetricKey
}

func PushRadiusClientResponseDrop(endpoint string, Code string) {
	MS.InputChan <- RadiusClientResponseDropEvent{Key: RadiusMetricKey{Endpoint: endpoint, Code: Code}}
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
	MS.InputChan <- RadiusServersTableUpdatedEvent{InstanceName: instanceName, Table: table}
}
