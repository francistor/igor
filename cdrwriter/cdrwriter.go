package cdrwriter

import (
	"igor/diamcodec"
	"igor/radiuscodec"
)

type CDRFormatter interface {
	GetRadiusCDRString(rp *radiuscodec.RadiusPacket) string
	GetDiameterCDRString(dm *diamcodec.DiameterMessage) string
}
