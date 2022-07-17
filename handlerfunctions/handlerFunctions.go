package handlerfunctions

import (
	"igor/diamcodec"
)

// The most basic handler ever. Returns an empty response to the received message
func EmptyHandler(request *diamcodec.DiameterMessage) (*diamcodec.DiameterMessage, error) {
	resp := diamcodec.NewDiameterAnswer(request)
	resp.Add("Result-Code", diamcodec.DIAMETER_SUCCESS)

	return resp, nil
}
