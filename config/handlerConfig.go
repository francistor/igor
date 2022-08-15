package config

import (
	"encoding/json"
	"fmt"
)

// Manages the configuration items for the http handlers
type HandlerConfigurationManager struct {
	cm                   ConfigurationManager
	currentHandlerConfig HandlerConfig
}

// Slice of configuration managers
// Except during testing, there will be only one instance, which will be retrieved by GetConfig(). A
// specific instance is retrieved with GetConfigInstance()
var handlerConfigs []*HandlerConfigurationManager = make([]*HandlerConfigurationManager, 0)

// Adds a Handler configuration object with the specified name
func InitHandlerConfigInstance(bootstrapFile string, instanceName string, isDefault bool) *HandlerConfigurationManager {

	// Check not already instantiated
	for i := range handlerConfigs {
		if handlerConfigs[i].cm.instanceName == instanceName {
			panic(instanceName + " already initalized")
		}
	}

	// Better to create asap
	handlerConfig := HandlerConfigurationManager{cm: NewConfigurationManager(bootstrapFile, instanceName)}
	handlerConfigs = append(handlerConfigs, &handlerConfig)

	// Initialize logger and dictionary, if default
	if isDefault {
		initLogger(&handlerConfig.cm)
		initDictionaries(&handlerConfig.cm)
	}

	// Load handler configuraton
	handlerConfig.UpdateHandlerConfig()

	return &handlerConfig
}

// Retrieves a specific configuration instance
func GetHandlerConfigInstance(instanceName string) *HandlerConfigurationManager {

	for i := range handlerConfigs {
		if handlerConfigs[i].cm.instanceName == instanceName {
			return handlerConfigs[i]
		}
	}

	panic("configuraton instance <" + instanceName + "> not configured")
}

// Retrieves the default configuration instance
func GetHandlerConfig() *HandlerConfigurationManager {
	return handlerConfigs[0]
}

///////////////////////////////////////////////////////////////////////////////

// Holds a http handler configuration
type HandlerConfig struct {
	// The IP address in which the http server will listen
	BindAddress string
	// The port number in which the http server will listen
	BindPort int
	// The IP address of the Radius&Diameter router to which the requests from the handler must be sent
	RouterAddress string
	// The TCP port of the Radius&Diameter router to which the requests from the handler must be sent
	RouterPort int
}

// Retrieves the handler configuration, forcing a refresh
func (c *HandlerConfigurationManager) getHandlerConfig() (HandlerConfig, error) {
	hc := HandlerConfig{}
	h, err := c.cm.GetConfigObject("handler.json", true)
	if err != nil {
		return hc, err
	}
	err = json.Unmarshal(h.RawBytes, &hc)
	if err != nil {
		return hc, err
	}
	return hc, nil
}

// Updates the global variable with the http handler configuration
func (c *HandlerConfigurationManager) UpdateHandlerConfig() error {
	hc, error := c.getHandlerConfig()
	if error != nil {
		return fmt.Errorf("could not retrieve the Handler configuration: %w", error)
	}
	c.currentHandlerConfig = hc
	return nil
}

// Retrieves the current http handler configuration
func (c *HandlerConfigurationManager) HandlerConf() HandlerConfig {
	return c.currentHandlerConfig
}
