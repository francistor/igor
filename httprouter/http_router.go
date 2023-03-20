package httprouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/francistor/igor/core"
	"github.com/francistor/igor/router"
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
	ci *core.PolicyConfigurationManager

	// Holds the httpserver
	httpServer *http.Server

	// For signaling finalization
	doneChannel chan interface{}
}

// Creates a new HttpRouter object
func NewHttpRouter(instanceName string, diameterRouter *router.DiameterRouter, radiusRouter *router.RadiusRouter) *HttpRouter {

	mux := new(http.ServeMux)
	if diameterRouter != nil {
		mux.HandleFunc("/routeDiameterRequest", getDiameterRouteHandler(diameterRouter))
	}
	if radiusRouter != nil {
		mux.HandleFunc("/routeRadiusRequest", getRadiusRouteHandler(radiusRouter))
	}

	ci := core.GetPolicyConfigInstance(instanceName)
	bindAddrPort := fmt.Sprintf("%s:%d", ci.HttpRouterConf().BindAddress, ci.HttpRouterConf().BindPort)
	core.GetLogger().Infof("HTTP Router listening in %s", bindAddrPort)

	h := HttpRouter{
		ci: ci,
		httpServer: &http.Server{
			Addr:              bindAddrPort,
			Handler:           mux,
			IdleTimeout:       1 * time.Minute,
			ReadHeaderTimeout: 5 * time.Second,
		},
		doneChannel: make(chan interface{}, 1),
	}

	go h.Run()
	return &h
}

// Execute the DiameterHandler. This function blocks. Should be executed
// in a goroutine.
func (dh *HttpRouter) Run() {

	// Make sure certificates exist in the current directory
	certFile, keyFile := core.EnsureCertificates()

	var err error
	if dh.ci.HttpRouterConf().UsePlainHttp {
		err = dh.httpServer.ListenAndServe()
	} else {
		err = dh.httpServer.ListenAndServeTLS(certFile, keyFile)
	}

	if !errors.Is(err, http.ErrServerClosed) {
		panic("error starting http handler  " + err.Error())
	}

	close(dh.doneChannel)
}

// Gracefully shutdown
func (dh *HttpRouter) Close() {
	dh.httpServer.Shutdown(context.Background())
	<-dh.doneChannel
}

func getDiameterRouteHandler(diameterRouter *router.DiameterRouter) func(w http.ResponseWriter, req *http.Request) {

	return func(w http.ResponseWriter, req *http.Request) {
		logger := core.GetLogger()

		// Get the Routable Diameter Request
		var jRequest []byte
		jRequestRaw, err := io.ReadAll(req.Body)
		if err != nil {
			logger.Error("error reading request: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(NETWORK_ERROR, req.RequestURI)
			return
		}

		// Execute the template if exists
		if len(req.URL.Query()) > 0 {
			// Apply template with query parameters if defined
			tmpl, err := template.New("request_template").Parse(string(jRequestRaw))
			if err != nil {
				logger.Errorf("error un-templating request: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
				core.PushHttpRouterExchange(UNSERIALIZATION_ERROR, req.RequestURI)
				return
			}

			// Get only one value for each parameter
			var parametersSet map[string]string = make(map[string]string)
			for k, v := range req.URL.Query() {
				parametersSet[k] = v[0]
			}

			// Apply the template
			var tmplRes strings.Builder
			if err := tmpl.Execute(&tmplRes, parametersSet); err != nil {
				logger.Errorf("error un-templating request: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
				core.PushHttpRouterExchange(UNSERIALIZATION_ERROR, req.RequestURI)
				return
			}

			jRequest = []byte(tmplRes.String())

		} else {
			jRequest = jRequestRaw
		}

		var request router.RoutableDiameterRequest
		if err = json.Unmarshal(jRequest, &request); err != nil {
			logger.Errorf("error unmarshalling request: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(UNSERIALIZATION_ERROR, req.RequestURI)
			return
		}
		request.Message.Tidy()

		// Fill the timeout
		if err = request.ParseTimeout(); err != nil {
			logger.Error("error parsing Timeoutspec: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(SERIALIZATION_ERROR, req.RequestURI)
			return
		}

		// Generate the Diameter Answer, passing it to the router
		answer, err := diameterRouter.RouteDiameterRequest(request.Message, request.Timeout)
		if err != nil {
			logger.Errorf("error handling request: %s", err)
			w.WriteHeader(http.StatusGatewayTimeout)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(HANDLER_FUNCTION_ERROR, req.RequestURI)
			return
		}
		jAnswer, err := json.Marshal(answer)
		if err != nil {
			logger.Errorf("error marshaling response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(SERIALIZATION_ERROR, req.RequestURI)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jAnswer)
		core.PushHttpRouterExchange(SUCCESS, req.RequestURI)
	}
}

func getRadiusRouteHandler(radiusRouter *router.RadiusRouter) func(w http.ResponseWriter, req *http.Request) {

	return func(w http.ResponseWriter, req *http.Request) {
		logger := core.GetLogger()

		// Get the Radius Request
		var jRequest []byte
		jRequestRaw, err := io.ReadAll(req.Body)
		if err != nil {
			logger.Errorf("error reading request: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(NETWORK_ERROR, req.RequestURI)
			return
		}

		// Execute the template if exists
		if len(req.URL.Query()) > 0 {
			// Apply template with query parameters if defined
			tmpl, err := template.New("request_template").Parse(string(jRequestRaw))
			if err != nil {
				logger.Errorf("error un-templating request: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
				core.PushHttpRouterExchange(UNSERIALIZATION_ERROR, req.RequestURI)
				return
			}

			// Get only one value for each parameter
			var parametersSet map[string]string = make(map[string]string)
			for k, v := range req.URL.Query() {
				parametersSet[k] = v[0]
			}

			// Apply the template
			var tmplRes strings.Builder
			if err := tmpl.Execute(&tmplRes, parametersSet); err != nil {
				logger.Errorf("error un-templating request: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
				core.PushHttpRouterExchange(UNSERIALIZATION_ERROR, req.RequestURI)
				return
			}

			jRequest = []byte(tmplRes.String())

		} else {
			jRequest = jRequestRaw
		}

		var request router.RoutableRadiusRequest
		if err = json.Unmarshal(jRequest, &request); err != nil {
			logger.Errorf("error unmarshalling request: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(UNSERIALIZATION_ERROR, req.RequestURI)
			return
		}

		// Fill the timeout
		if err = request.ParseTimeout(); err != nil {
			logger.Errorf("error parsing Timeoutspec: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(SERIALIZATION_ERROR, req.RequestURI)
			return
		}

		// Generate the Radius Answer, passing it to the router
		answer, err := radiusRouter.RouteRadiusRequest(request.Packet, request.Destination, request.PerRequestTimeout, request.Tries, request.ServerTries, request.Secret)
		if err != nil {
			logger.Errorf("error handling request: %s", err)
			w.WriteHeader(http.StatusGatewayTimeout)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(HANDLER_FUNCTION_ERROR, req.RequestURI)
			return
		}
		jAnswer, err := json.Marshal(answer)
		if err != nil {
			logger.Errorf("error marshaling response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			core.PushHttpRouterExchange(SERIALIZATION_ERROR, req.RequestURI)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jAnswer)
		core.PushHttpRouterExchange(SUCCESS, req.RequestURI)
	}
}
