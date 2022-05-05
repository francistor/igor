package instrumentation

import (
	"igor/config"
	"igor/diamcodec"
)

type DiameterMetricKey struct {
	Peer string
	OH   string
	OR   string
	DH   string
	DR   string
	AP   string
	CM   string
}

func fillDiameterKey(peerName string, diameterMessage diamcodec.DiameterMessage) *DiameterMetricKey {
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

type DiameterRequestReceivedEvent struct {
	Key DiameterMetricKey
}

func PushDiameterRequestReceived(ci config.ConfigurationManager, peerName string, diameterMessage diamcodec.DiameterMessage) {
	//keyPtr := fillDiameterKey(peerName, diameterMessage)

	// ci.IChann <-
}

type DiameterAnswerReceivedEvent struct {
	Key DiameterMetricKey
}

func PushDiameterAnswerReceived(ci config.ConfigurationManager, peerName string, diameterMessage diamcodec.DiameterMessage) {
	//keyPtr := fillDiameterKey(peerName, diameterMessage)

	// ci.IChann <-
}

type DiameterRequestTimeoutEvent struct {
	Key DiameterMetricKey
}

func PushDiameterRequestTimeout(ci config.ConfigurationManager, peerName string, diameterMessage diamcodec.DiameterMessage) {
	//keyPtr := fillDiameterKey(peerName, diameterMessage)

	// ci.IChann <-
}

type DiameterRequestSentEvent struct {
	Key DiameterMetricKey
}

func PushDiameterRequestSent(ci config.ConfigurationManager, peerName string, diameterMessage diamcodec.DiameterMessage) {
	//keyPtr := fillDiameterKey(peerName, diameterMessage)

	// ci.IChann <-
}

type DiameterAnswerSentEvent struct {
	Key DiameterMetricKey
}

func PushDiameterAnswerSent(ci config.ConfigurationManager, peerName string, diameterMessage diamcodec.DiameterMessage) {
	//keyPtr := fillDiameterKey(peerName, diameterMessage)

	// ci.IChann <-
}

type DiameterAnswerDiscardedEvent struct {
	Key DiameterMetricKey
}

func PushDiameterAnswerDiscarded(ci config.ConfigurationManager, peerName string, diameterMessage diamcodec.DiameterMessage) {
	//keyPtr := fillDiameterKey(peerName, diameterMessage)

	// ci.IChann <-
}
