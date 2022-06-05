package handler

import (
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamcodec"
	"io/ioutil"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	// Holds the configuration instance for this DiameterPeer
	ci *config.HandlerConfigurationManager
}

// Creates a new DiameterHandler object
func NewHandler(instanceName string) Handler {
	h := Handler{ci: config.GetHandlerConfigInstance(instanceName)}

	// TODO: Close gracefully
	go h.Run()
	return h
}

// Execute the DiameterHandler. This function blocks. Should be executed
// in a goroutine.
func (dh *Handler) Run() {

	logger := config.GetLogger()

	// Configure Server
	r := gin.Default()

	r.POST("/diameterRequest", func(c *gin.Context) {

		// Get the Diameter Request
		jRequest, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			logger.Error("error reading request %s", err)
			c.AbortWithError(500, err)
			return
		}
		var request diamcodec.DiameterMessage
		json.Unmarshal(jRequest, &request)

		// Generate the Diameter Answer
		answer, err := EmptyHandler(request)
		if err != nil {
			logger.Error("error handling request %s", err)
			c.AbortWithError(500, err)
			return
		}
		c.JSON(200, answer)
	})

	bindAddrPort := fmt.Sprintf("%s:%d", dh.ci.HandlerConf().BindAddress, dh.ci.HandlerConf().BindPort)

	r.RunTLS(bindAddrPort, "/home/francisco/cert.pem", "/home/francisco/key.pem")
	// r.Run("localhost:8080")
}
