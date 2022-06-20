package config

import (
	"encoding/json"
	"fmt"
)

type HandlerConfigurationManager struct {
	CM ConfigurationManager

	currentHandlerConfig HandlerConfig
}

// Slice of configuration managers
// Except during testing, there will be only one instance, which will be retrieved by GetConfig()
var handlerConfigs []*HandlerConfigurationManager = make([]*HandlerConfigurationManager, 0)

// Adds a Handler configuration object with the specified name
func InitHandlerConfigInstance(bootstrapFile string, instanceName string, isDefault bool) *HandlerConfigurationManager {

	// Check not already instantiated
	for i := range handlerConfigs {
		if handlerConfigs[i].CM.instanceName == instanceName {
			panic(instanceName + " already initalized")
		}
	}

	// Better to create asap
	handlerConfig := HandlerConfigurationManager{CM: NewConfigurationManager(bootstrapFile, instanceName)}
	handlerConfigs = append(handlerConfigs, &handlerConfig)

	// Rules
	handlerConfig.CM.FillSearchRules(bootstrapFile)

	// Initialize logger and dictionary, if default
	if isDefault {
		initLogger(&handlerConfig.CM)
		initDictionaries(&handlerConfig.CM)
	}

	// Load handler configuraton
	handlerConfig.UpdateHandlerConfig()

	return &handlerConfig
}

// Retrieves a specific configuration instance
func GetHandlerConfigInstance(instanceName string) *HandlerConfigurationManager {

	for i := range handlerConfigs {
		if handlerConfigs[i].CM.instanceName == instanceName {
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

type HandlerConfig struct {
	BindAddress string
	BindPort    int
}

// Retrieves the handler configuration
func (c *HandlerConfigurationManager) getHandlerConfig() (HandlerConfig, error) {
	hc := HandlerConfig{}
	h, err := c.CM.GetConfigObject("handler.json")
	if err != nil {
		return hc, err
	}
	json.Unmarshal([]byte(h.RawText), &hc)
	return hc, nil
}

func (c *HandlerConfigurationManager) UpdateHandlerConfig() error {
	hc, error := c.getHandlerConfig()
	if error != nil {
		return fmt.Errorf("could not retrieve the Handler configuration: %w", error)
	}
	c.currentHandlerConfig = hc
	return nil
}

func (c *HandlerConfigurationManager) HandlerConf() HandlerConfig {
	return c.currentHandlerConfig
}
