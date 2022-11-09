package cdrwriter

import (
	"github.com/francistor/igor/diamcodec"
	"github.com/francistor/igor/radiuscodec"
)

type CDRFormatter interface {
	GetRadiusCDRString(rp *radiuscodec.RadiusPacket) string
	GetDiameterCDRString(dm *diamcodec.DiameterMessage) string
}
