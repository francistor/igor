package config

import (
	"fmt"
)

// Manages the configuration items for the http handlers
type HttpHandlerConfigurationManager struct {
	CM                       ConfigurationManager
	currentHttpHandlerConfig HttpHandlerConfig
}

// Slice of configuration managers
// Except during testing, there will be only one instance, which will be retrieved by GetConfig(). A
// specific instance is retrieved with GetConfigInstance()
var httpHandlerConfigs []*HttpHandlerConfigurationManager = make([]*HttpHandlerConfigurationManager, 0)

// Adds a Handler configuration object with the specified name
func InitHttpHandlerConfigInstance(bootstrapFile string, instanceName string, isDefault bool) *HttpHandlerConfigurationManager {

	// Check not already instantiated
	for i := range httpHandlerConfigs {
		if httpHandlerConfigs[i].CM.instanceName == instanceName {
			panic(instanceName + " already initalized")
		}
	}

	// Better to create asap
	httpHandlerConfig := HttpHandlerConfigurationManager{CM: NewConfigurationManager(bootstrapFile, instanceName)}
	httpHandlerConfigs = append(httpHandlerConfigs, &httpHandlerConfig)

	// Initialize logger and dictionary, if default
	if isDefault {
		initLogger(&httpHandlerConfig.CM)
		initDictionaries(&httpHandlerConfig.CM)
	}

	// Load handler configuraton
	httpHandlerConfig.UpdateHttpHandlerConfig()

	return &httpHandlerConfig
}

// Retrieves a specific configuration instance
func GetHttpHandlerConfigInstance(instanceName string) *HttpHandlerConfigurationManager {

	for i := range httpHandlerConfigs {
		if httpHandlerConfigs[i].CM.instanceName == instanceName {
			return httpHandlerConfigs[i]
		}
	}

	panic("configuraton instance <" + instanceName + "> not configured")
}

// Retrieves the default configuration instance
func GetHttpHandlerConfig() *HttpHandlerConfigurationManager {
	return httpHandlerConfigs[0]
}

///////////////////////////////////////////////////////////////////////////////

// Holds a http handler configuration
type HttpHandlerConfig struct {
	// The IP address in which the http server will listen
	BindAddress string
	// The port number in which the http server will listen
	BindPort int
	// The IP address of the Radius&Diameter router to which the requests from the handler must be sent
	RouterAddress string
	// The TCP port of the Radius&Diameter router to which the requests from the handler must be sent
	RouterPort int
}

// Updates the global variable with the http handler configuration
func (c *HttpHandlerConfigurationManager) UpdateHttpHandlerConfig() error {
	hc := HttpHandlerConfig{}
	err := c.CM.BuildJSONConfigObject("httpHandler.json", &hc)
	if err != nil {
		return fmt.Errorf("could not retrieve the Handler configuration: %w", err)
	}
	c.currentHttpHandlerConfig = hc
	return nil
}

// Retrieves the current http handler configuration
func (c *HttpHandlerConfigurationManager) HttpHandlerConf() HttpHandlerConfig {
	return c.currentHttpHandlerConfig
}
