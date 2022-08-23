package cdrwriter

import "igor/radiuscodec"

type CDRFormatter interface {
	GetCDRString(rp *radiuscodec.RadiusPacket) string
}
