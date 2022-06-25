package instrumentation

import (
	"igor/diamcodec"
	"time"
)

// Used as key for diameter metrics, both in storage and as a way to specify queries,
// where the fields with non zero values will be used for aggregation
type DiameterMetricKey struct {
	Peer string
	OH   string
	OR   string
	DH   string
	DR   string
	AP   string
	CM   string
}

// Generates a DiameterMetricKey from a specified Diameter Message
func DiameterMetricKeyFromMessage(peerName string, diameterMessage *diamcodec.DiameterMessage) DiameterMetricKey {

	return DiameterMetricKey{
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

Router
	RouterRouteNotFound
	RouterHandlerError
*/

// Diameter Server

// Message sent to instrumentation server when a diameter request is received in a Peer
type PeerDiameterRequestReceivedEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is received
func PushPeerDiameterRequestReceived(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- PeerDiameterRequestReceivedEvent{Key: DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter answer is sent in a Peer
type PeerDiameterAnswerSentEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter answer is sent
func PushPeerDiameterAnswerSent(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- PeerDiameterAnswerSentEvent{Key: DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Diameter Client

// Message sent to instrumentation server when a diameter request is sent to a Peer
type PeerDiameterRequestSentEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is sent to a Peer
func PushPeerDiameterRequestSent(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- PeerDiameterRequestSentEvent{Key: DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter answer is received from a Peer
type PeerDiameterAnswerReceivedEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter answer is received from a Peer
func PushPeerDiameterAnswerReceived(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- PeerDiameterAnswerReceivedEvent{Key: DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter request timeout occurs
type PeerDiameterRequestTimeoutEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request timeout occurs
func PushPeerDiameterRequestTimeout(peerName string, key DiameterMetricKey) {
	MS.InputChan <- PeerDiameterRequestTimeoutEvent{Key: key}
}

// Message sent to instrumentation server when a diameter request timeout occurs
type PeerDiameterAnswerStalledEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request timeout occurs
func PushPeerDiameterAnswerStalled(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- PeerDiameterAnswerStalledEvent{Key: DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Router

// Message sent to instrumentation server when a diameter request has no route available
type RouterRouteNotFoundEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is discarded
func PushRouterRouteNotFound(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- RouterRouteNotFoundEvent{Key: DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when no diameter peer available
type RouterNoAvailablePeerEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is discarded
func PushRouterNoAvailablePeer(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- RouterNoAvailablePeerEvent{Key: DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

type RouterHandlerError struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when the handler produced an error
func PushRouterHandlerError(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- RouterHandlerError{Key: DiameterMetricKeyFromMessage(peerName, diameterMessage)}
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

func PushDiameterPeersStatus(instanceName string, table DiameterPeersTable) {
	MS.InputChan <- DiameterPeersTableUpdatedEvent{InstanceName: instanceName, Table: table}
}
