package cdrwriter

import (
	"github.com/francistor/igor/core"
)

type Tick struct{}

type CDRFormatter interface {
	GetRadiusCDRString(rp *core.RadiusPacket) string
	GetDiameterCDRString(dm *core.DiameterMessage) string
}
