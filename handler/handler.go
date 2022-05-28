package handler

import (
	"encoding/json"
	"igor/config"
	"igor/diamcodec"
	"io/ioutil"

	"github.com/gin-gonic/gin"
)

type DiameterHandlerFunction func(request diamcodec.DiameterMessage) (diamcodec.DiameterMessage, error)

type DiameterHandler struct {
	instanceName string
}

// Creates a new DiameterHandler object
func NewDiameterHandler(i string) DiameterHandler {
	return DiameterHandler{instanceName: i}
}

// Execute the DiameterHandler. This function blocks. Should be executed
// in a goroutine.
func (dh *DiameterHandler) Run() {

	logger := config.GetConfigInstance(dh.instanceName).IgorLogger

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

	r.RunTLS("localhost:8080", "/home/francisco/cert.pem", "/home/francisco/key.pem")
	// r.Run("localhost:8080")
}
