package httprouter

import (
	"context"
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/instrumentation"
	"igor/router"
	"io/ioutil"
	"net/http"
)

const (
	SERIALIZATION_ERROR    = "550"
	NETWORK_ERROR          = "551"
	HTTP_RESPONSE_ERROR    = "552"
	HANDLER_FUNCTION_ERROR = "553"
	UNSERIALIZATION_ERROR  = "554"

	SUCCESS = "200"
)

type HttpRouter struct {
	// Holds the configuration instance for this Handler
	ci *config.PolicyConfigurationManager

	// Holds the httpserver
	httpServer *http.Server

	// For signaling finalization
	doneChannel chan interface{}
}

// Creates a new DiameterHandler object
func NewHttpRouter(instanceName string, diameterRouter *router.DiameterRouter, radiusRouter *router.RadiusRouter) HttpRouter {

	// 	https://stackoverflow.com/questions/40786526/resetting-http-handlers-in-golang-for-unit-testing
	http.DefaultServeMux = new(http.ServeMux)

	ci := config.GetPolicyConfigInstance(instanceName)
	bindAddrPort := fmt.Sprintf("%s:%d", ci.HttpRouterConf().BindAddress, ci.HttpRouterConf().BindPort)
	config.GetLogger().Infof("HTTP Router listening in %s", bindAddrPort)

	h := HttpRouter{
		ci:          ci,
		httpServer:  &http.Server{Addr: bindAddrPort},
		doneChannel: make(chan interface{}, 1),
	}

	http.HandleFunc("/routeDiameterRequest", getDiameterRouteHandler(diameterRouter))
	http.HandleFunc("/routeRadiusRequest", getRadiusRouteHandler(radiusRouter))

	go h.Run()
	return h
}

// Execute the DiameterHandler. This function blocks. Should be executed
// in a goroutine.
func (dh *HttpRouter) Run() {

	dh.httpServer.ListenAndServeTLS(
		"/home/francisco/cert.pem",
		"/home/francisco/key.pem")

	close(dh.doneChannel)
}

// Gracefully shutdown
func (dh *HttpRouter) Close() {
	dh.httpServer.Shutdown(context.Background())
	<-dh.doneChannel
}

func getDiameterRouteHandler(diameterRouter *router.DiameterRouter) func(w http.ResponseWriter, req *http.Request) {

	return func(w http.ResponseWriter, req *http.Request) {
		logger := config.GetLogger()

		// Get the Routable Diameter Request
		jRequest, err := ioutil.ReadAll(req.Body)
		if err != nil {
			logger.Error("error reading request: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpRouterExchange(NETWORK_ERROR, req.RequestURI)
			return
		}
		var request router.RoutableDiameterRequest
		if err = json.Unmarshal(jRequest, &request); err != nil {
			logger.Error("error unmarshalling request: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpRouterExchange(SERIALIZATION_ERROR, req.RequestURI)
			return
		}

		// Generate the Diameter Answer, passing it to the router
		answer, err := diameterRouter.RouteDiameterRequest(request.Message, request.Timeout)
		if err != nil {
			logger.Errorf("error handling request: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpRouterExchange(HANDLER_FUNCTION_ERROR, req.RequestURI)
			return
		}
		jAnswer, err := json.Marshal(answer)
		if err != nil {
			logger.Errorf("error marshaling response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpRouterExchange(UNSERIALIZATION_ERROR, req.RequestURI)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(jAnswer)
		instrumentation.PushHttpRouterExchange(SUCCESS, req.RequestURI)
	}
}

func getRadiusRouteHandler(radiusRouter *router.RadiusRouter) func(w http.ResponseWriter, req *http.Request) {

	return func(w http.ResponseWriter, req *http.Request) {
		logger := config.GetLogger()

		// Get the Radius Request
		jRequest, err := ioutil.ReadAll(req.Body)
		if err != nil {
			logger.Error("error reading request: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpRouterExchange(NETWORK_ERROR, req.RequestURI)
			return
		}
		var request router.RoutableRadiusRequest
		if err = json.Unmarshal(jRequest, &request); err != nil {
			logger.Error("error unmarshalling request: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpRouterExchange(SERIALIZATION_ERROR, req.RequestURI)
			return
		}

		// Generate the Radius Answer, passing it to the router
		answer, err := radiusRouter.RouteRadiusRequest(request.Destination, request.Packet, request.PerRequestTimeout, request.Tries, request.ServerTries, request.Secret)
		if err != nil {
			logger.Errorf("error handling request: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpRouterExchange(HANDLER_FUNCTION_ERROR, req.RequestURI)
			return
		}
		jAnswer, err := json.Marshal(answer)
		if err != nil {
			logger.Errorf("error marshaling response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			instrumentation.PushHttpRouterExchange(UNSERIALIZATION_ERROR, req.RequestURI)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(jAnswer)
		instrumentation.PushHttpRouterExchange(SUCCESS, req.RequestURI)
	}
}