package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Used as key for diameter metrics, both in storage and as a way to specify queries,
// where the fields with non zero values will be used for aggregation
type PeerDiameterMetricKey struct {
	Peer string
	OH   string
	OR   string
	DH   string
	DR   string
	AP   string
	CM   string
}

type PeerDiameterMetrics map[PeerDiameterMetricKey]uint64

// Custom marshalling of PeerDiameterMetrics
func (pdm PeerDiameterMetrics) MarshalJSON() ([]byte, error) {

	// JSON object will have a property for the key and another one for the value
	type T struct {
		Key   PeerDiameterMetricKey
		Value uint64
	}

	// The array of T to produce as JSON
	metrics := make([]T, 0)

	for m, v := range pdm {
		metrics = append(metrics, T{Key: m, Value: v})
	}

	return json.Marshal(metrics)
}

// Builder for Prometheus format export
func (pdm PeerDiameterMetrics) genPrometheusMetric(metricName string, helpString string) string {
	var builder strings.Builder
	if len(pdm) > 0 {
		builder.WriteString(fmt.Sprintf("HELP %s %s\n", metricName, helpString))
		builder.WriteString(fmt.Sprintf("TYPE %s counter\n", metricName))
	}
	for k, v := range pdm {
		builder.WriteString(fmt.Sprintf("%s{peer=\"%s\",oh=\"%s\",or=\"%s\",dh=\"%s\",dr=\"%s\",ap=\"%s\",cm=\"%s\"} %d\n",
			metricName, k.Peer, k.OH, k.OR, k.DH, k.DR, k.AP, k.CM, v))
	}

	return builder.String()
}

// Generates a PeerDiameterMetric from a specified Diameter Message
func PeerDiameterMetricFromMessage(peerName string, diameterMessage *DiameterMessage) PeerDiameterMetricKey {

	return PeerDiameterMetricKey{
		Peer: peerName,
		OH:   diameterMessage.GetStringAVP("Origin-Host"),
		OR:   diameterMessage.GetStringAVP("Origin-Realm"),
		DH:   diameterMessage.GetStringAVP("Destination-Host"),
		DR:   diameterMessage.GetStringAVP("Destination-Realm"),
		AP:   diameterMessage.ApplicationName,
		CM:   diameterMessage.CommandName,
	}
}

/*
Diameter Server
	PeerRequestReceived
	PeerAnswerSent

Diameter Client
	PeerRequestSent
	PeerAnswerReceived
	PeerRequestTimeout
	PeerAnswerStalled

Router
	RouterRouteNotFound
	RouterHandlerError
*/

// Diameter Server

// Message sent to instrumentation server when a diameter request is received in a Peer
type PeerDiameterRequestReceivedEvent struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is received
func IncrementPeerDiameterRequestReceived(peerName string, diameterMessage *DiameterMessage) {
	MS.metricEventChan <- PeerDiameterRequestReceivedEvent{Key: PeerDiameterMetricFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter answer is sent in a Peer
type PeerDiameterAnswerSentEvent struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter answer is sent
func IncrementPeerDiameterAnswerSent(peerName string, diameterMessage *DiameterMessage) {
	MS.metricEventChan <- PeerDiameterAnswerSentEvent{Key: PeerDiameterMetricFromMessage(peerName, diameterMessage)}
}

// Diameter Client

// Message sent to instrumentation server when a diameter request is sent to a Peer
type PeerDiameterRequestSentEvent struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is sent to a Peer
func IncrementPeerDiameterRequestSent(peerName string, diameterMessage *DiameterMessage) {
	MS.metricEventChan <- PeerDiameterRequestSentEvent{Key: PeerDiameterMetricFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter answer is received from a Peer
type PeerDiameterAnswerReceivedEvent struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter answer is received from a Peer
func IncrementPeerDiameterAnswerReceived(peerName string, diameterMessage *DiameterMessage) {
	MS.metricEventChan <- PeerDiameterAnswerReceivedEvent{Key: PeerDiameterMetricFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter request timeout occurs
type PeerDiameterRequestTimeoutEvent struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request timeout occurs
func IncrementPeerDiameterRequestTimeout(key PeerDiameterMetricKey) {
	MS.metricEventChan <- PeerDiameterRequestTimeoutEvent{Key: key}
}

// Message sent to instrumentation server when a diameter request timeout occurs
type PeerDiameterAnswerStalledEvent struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request timeout occurs
func IncrementPeerDiameterAnswerStalled(peerName string, diameterMessage *DiameterMessage) {
	MS.metricEventChan <- PeerDiameterAnswerStalledEvent{Key: PeerDiameterMetricFromMessage(peerName, diameterMessage)}
}

// Router

// Message sent to instrumentation server when a diameter request has no route available
type RouterRouteNotFoundEvent struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is discarded
func IncrementRouterRouteNotFound(peerName string, diameterMessage *DiameterMessage) {
	MS.metricEventChan <- RouterRouteNotFoundEvent{Key: PeerDiameterMetricFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when no diameter peer available
type RouterNoAvailablePeerEvent struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is discarded
func IncrementRouterNoAvailablePeer(peerName string, diameterMessage *DiameterMessage) {
	MS.metricEventChan <- RouterNoAvailablePeerEvent{Key: PeerDiameterMetricFromMessage(peerName, diameterMessage)}
}

type RouterHandlerError struct {
	Key PeerDiameterMetricKey
}

// Helper function to send a message to the instrumentation server when the handler produced an error
func IncrementRouterHandlerError(peerName string, diameterMessage *DiameterMessage) {
	MS.metricEventChan <- RouterHandlerError{Key: PeerDiameterMetricFromMessage(peerName, diameterMessage)}
}

// Instrumentation of Diameter Peers table
type DiameterPeersTableEntry struct {
	DiameterHost     string
	IPAddress        string
	ConnectionPolicy string
	IsEngaged        bool
	LastStatusChange time.Time
	LastError        error
}

type DiameterPeersTable []DiameterPeersTableEntry

type DiameterPeersTableUpdatedEvent struct {
	InstanceName string
	Table        DiameterPeersTable
}

func IncrementDiameterPeersStatus(instanceName string, table DiameterPeersTable) {
	MS.metricEventChan <- DiameterPeersTableUpdatedEvent{InstanceName: instanceName, Table: table}
}
