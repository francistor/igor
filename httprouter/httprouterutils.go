package httprouter

import (
	"encoding/json"
	"fmt"
	"igor/instrumentation"
	"igor/router"
	"net/http"
)

// Helper function to invoke an http router operation for diameter
func RouteDiameter(diameterRouter *router.DiameterRouter, client http.Client, path string, jsonRoutableDiameterRequest []byte) ([]byte, error) {

	// Unserialize to RoutableDiameterRequest
	rdr := router.RoutableDiameterRequest{}
	err := json.Unmarshal(jsonRoutableDiameterRequest, &rdr)
	if err != nil {
		instrumentation.PushHttpRouterExchange(UNSERIALIZATION_ERROR, path)
		return nil, fmt.Errorf("error unmarshaling request: %w", err)
	}
	err = rdr.ParseTimeout()
	if err != nil {
		instrumentation.PushHttpRouterExchange(UNSERIALIZATION_ERROR, path)
		return nil, fmt.Errorf("error unmarshaling request (timespec): %w", err)
	}

	// Send the request to the router
	answer, err := diameterRouter.RouteDiameterRequest(rdr.Message, rdr.Timeout)
	if err != nil {
		instrumentation.PushHttpRouterExchange(HANDLER_FUNCTION_ERROR, path)
		return nil, err
	}

	// Marshal to JSON
	jAnswer, err := json.Marshal(answer)
	if err != nil {
		instrumentation.PushHttpRouterExchange(SERIALIZATION_ERROR, path)
		return nil, err
	}

	instrumentation.PushHttpRouterExchange(SUCCESS, path)
	return jAnswer, nil

}

// Helper function to invoke an http router operation for radius
func RouteRadius(radiusRouter *router.RadiusRouter, client http.Client, path string, jsonRoutableRadiusRequest []byte) ([]byte, error) {

	// Unserialize to RoutableRadiusRequest
	rrr := router.RoutableRadiusRequest{}
	err := json.Unmarshal(jsonRoutableRadiusRequest, &rrr)
	if err != nil {
		instrumentation.PushHttpRouterExchange(UNSERIALIZATION_ERROR, path)
		return nil, fmt.Errorf("error unmarshaling request: %w", err)
	}
	err = rrr.ParseTimeout()
	if err != nil {
		instrumentation.PushHttpRouterExchange(UNSERIALIZATION_ERROR, path)
		return nil, fmt.Errorf("error unmarshaling request (timespec): %w", err)
	}

	// Send the request to the router
	answer, err := radiusRouter.RouteRadiusRequest(rrr.Destination, rrr.Packet, rrr.PerRequestTimeout, rrr.Tries, rrr.ServerTries, rrr.Secret)
	if err != nil {
		instrumentation.PushHttpRouterExchange(HANDLER_FUNCTION_ERROR, path)
		return nil, err
	}

	// Marshal to JSON
	jAnswer, err := json.Marshal(answer)
	if err != nil {
		instrumentation.PushHttpRouterExchange(SERIALIZATION_ERROR, path)
		return nil, err
	}

	instrumentation.PushHttpRouterExchange(SUCCESS, path)
	return jAnswer, nil
}
