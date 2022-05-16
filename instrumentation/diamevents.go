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
func DiameterMetricKeyFromMessage(peerName string, diameterMessage *diamcodec.DiameterMessage) *DiameterMetricKey {
	key := DiameterMetricKey{}
	key.Peer = peerName
	key.OH = diameterMessage.GetStringAVP("Origin-Host")
	key.OR = diameterMessage.GetStringAVP("Origin-Realm")
	key.DH = diameterMessage.GetStringAVP("Destination-Host")
	key.DR = diameterMessage.GetStringAVP("Destination-Realm")
	key.AP = diameterMessage.ApplicationName
	key.CM = diameterMessage.CommandName
	return &key
}

// Message sent to instrumentation server when a diameter request is received
type DiameterRequestReceivedEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is received
func PushDiameterRequestReceived(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- DiameterRequestReceivedEvent{Key: *DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter answer is received
type DiameterAnswerReceivedEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter answer is received
func PushDiameterAnswerReceived(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- DiameterAnswerReceivedEvent{Key: *DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter request timeout occurs
type DiameterRequestTimeoutEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request timeout occurs
func PushDiameterRequestTimeout(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- DiameterRequestTimeoutEvent{Key: *DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter request is sent
type DiameterRequestSentEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter request is sent
func PushDiameterRequestSent(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- DiameterRequestSentEvent{Key: *DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter answer is sent
type DiameterAnswerSentEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter answer is sent
func PushDiameterAnswerSent(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- DiameterAnswerSentEvent{Key: *DiameterMetricKeyFromMessage(peerName, diameterMessage)}
}

// Message sent to instrumentation server when a diameter answer is disccarded because the corresponding request was not found
type DiameterAnswerDiscardedEvent struct {
	Key DiameterMetricKey
}

// Helper function to send a message to the instrumentation server when a diameter answer is discarded
func PushDiameterAnswerDiscarded(peerName string, diameterMessage *diamcodec.DiameterMessage) {
	MS.InputChan <- DiameterAnswerDiscardedEvent{Key: *DiameterMetricKeyFromMessage(peerName, diameterMessage)}
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
