package handlerfunctions

import (
	"igor/diamcodec"
	"igor/radiuscodec"
)

// The most basic handler ever. Returns an empty response to the received message
func EmptyDiameterHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	resp := diamcodec.NewDiameterAnswer(request)
	resp.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)

	return resp, nil
}

// The most basic handler ever. Returns an empty response to the received message
func EmptyRadiusHandler(request *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {
	resp := radiuscodec.NewRadiusResponse(request, true)

	return resp, nil
}
