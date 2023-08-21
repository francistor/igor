package core

// Manages the configuration items for the http handlers
type HttpHandlerConfigurationManager struct {
	CM                ConfigurationManager
	httpHandlerConfig *ConfigObject[HttpHandlerConfig]
}

// Slice of configuration managers
// Except during testing, there will be only one instance, which will be retrieved by GetConfig().
// A specific instance is retrieved with GetConfigInstance()
var httpHandlerConfigs []*HttpHandlerConfigurationManager = make([]*HttpHandlerConfigurationManager, 0)

// Adds a Handler configuration object with the specified name
func InitHttpHandlerConfigInstance(bootstrapFile string, instanceName string, configParams map[string]string, isDefault bool) *HttpHandlerConfigurationManager {

	// Check not already instantiated
	for i := range httpHandlerConfigs {
		if httpHandlerConfigs[i].CM.instanceName == instanceName {
			panic(instanceName + " already initalized")
		}
	}

	// Better to create asap
	httpHandlerConfig := HttpHandlerConfigurationManager{
		CM:                NewConfigurationManager(bootstrapFile, instanceName, configParams),
		httpHandlerConfig: NewConfigObject[HttpHandlerConfig]("httpHandler.json"),
	}
	httpHandlerConfigs = append(httpHandlerConfigs, &httpHandlerConfig)

	// Initialize logger, dictionary and metrics if default
	if isDefault {
		initLogger(&httpHandlerConfig.CM)
		initRadiusDict(&httpHandlerConfig.CM)
		initDiameterDict(&httpHandlerConfig.CM)
		initMetricsServer(&httpHandlerConfig.CM)
	}

	// Load handler configuraton
	if err := httpHandlerConfig.UpdateHttpHandlerConfig(); err != nil {
		panic(err)
	}

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
	return c.httpHandlerConfig.Update(&c.CM)
}

// Retrieves the current http handler configuration
func (c *HttpHandlerConfigurationManager) HttpHandlerConf() HttpHandlerConfig {
	return c.httpHandlerConfig.Get()
}
