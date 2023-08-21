package sessionserver

import "github.com/francistor/igor/core"

type SessionsResponse struct {
	Items [][]core.RadiusAVP
}
