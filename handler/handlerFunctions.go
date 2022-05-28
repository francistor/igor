package handler

import (
	"igor/diamcodec"
)

// The most basic handler ever. Returns an empty response to the received message
func EmptyHandler(request diamcodec.DiameterMessage) (diamcodec.DiameterMessage, error) {
	return diamcodec.NewDefaultDiameterAnswer(&request), nil
}
