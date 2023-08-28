package core

import (
	"time"
)

// Manages the configuration items for the session servers
type RadiusSessionServerConfigurationManager struct {
	CM                        ConfigurationManager
	radiusSessionServerConfig *ConfigObject[RadiusSessionServerConfig]
}

// Slice of configuration managers
// Except during testing, there will be only one instance, which will be retrieved by GetConfig().
// A specific instance is retrieved with GetConfigInstance()
var radiusSessionServerConfigs []*RadiusSessionServerConfigurationManager = make([]*RadiusSessionServerConfigurationManager, 0)

// Adds a Handler configuration object with the specified name
func InitRadiusSessionServerConfigInstance(bootstrapFile string, instanceName string, configParams map[string]string, isDefault bool) *RadiusSessionServerConfigurationManager {

	// Check not already instantiated
	for i := range radiusSessionServerConfigs {
		if radiusSessionServerConfigs[i].CM.instanceName == instanceName {
			panic(instanceName + " already initalized")
		}
	}

	// Better to create asap
	radiusSessionServerConfig := RadiusSessionServerConfigurationManager{
		CM:                        NewConfigurationManager(bootstrapFile, instanceName, configParams),
		radiusSessionServerConfig: NewConfigObject[RadiusSessionServerConfig]("radiusSessionServer.json"),
	}

	radiusSessionServerConfigs = append(radiusSessionServerConfigs, &radiusSessionServerConfig)

	// Initialize logger, dictionary and metrics if default
	if isDefault {
		initLogger(&radiusSessionServerConfig.CM)
		initRadiusDict(&radiusSessionServerConfig.CM)
		initDiameterDict(&radiusSessionServerConfig.CM)
		initInstrumentationServer(&radiusSessionServerConfig.CM)
	}

	// Load handler configuraton
	if err := radiusSessionServerConfig.UpdateRadiusSessionServerConfig(); err != nil {
		panic(err)
	}

	return &radiusSessionServerConfig
}

// Retrieves a specific configuration instance
func GetRadiusSessionServerConfigInstance(instanceName string) *RadiusSessionServerConfigurationManager {

	for i := range radiusSessionServerConfigs {
		if radiusSessionServerConfigs[i].CM.instanceName == instanceName {
			return radiusSessionServerConfigs[i]
		}
	}

	panic("configuraton instance <" + instanceName + "> not configured")
}

// Retrieves the default configuration instance
func GetRadiusSessionServerConfig() *RadiusSessionServerConfigurationManager {
	return radiusSessionServerConfigs[0]
}

///////////////////////////////////////////////////////////////////////////////

type SessionIndexConf struct {
	IndexName string
	IsUnique  bool
}

// Holds a radius session server configuration
type RadiusSessionServerConfig struct {

	// Name of the replication instance, to avoid loops
	Name string

	// The attributes to store in the session
	Attributes []string

	// The names of the attributes with indexes
	IndexConf []SessionIndexConf

	// The names of the attributes that conform the id
	IdAttributes []string

	// Expiration time of started sessions
	ExpirationTimeSeconds int64
	ExpirationTime        time.Duration

	// Expiration time of accepted and stopped sessions
	LimboTimeSeconds int64
	LimboTime        time.Duration

	// For testing only. Default value is 1 second
	PurgeIntervalMillis int64

	// Bind addresses and ports
	RadiusBindAddress string
	RadiusBindPort    int
	HttpBindAddress   string
	HttpBindPort      int

	// ReplicationParams
	ReplicationParams struct {
		OriginPorts []int
		TimeoutSecs int64
		ServerTries int
	}

	// To be cooked
	ReceiveFrom RadiusClients
	SendTo      []RadiusServer
}

// Initializer
func (rssc *RadiusSessionServerConfig) initialize() error {

	if err := rssc.ReceiveFrom.initialize(); err != nil {
		return err
	}

	rssc.ExpirationTime = time.Duration(rssc.ExpirationTimeSeconds) * time.Second
	rssc.LimboTime = time.Duration(rssc.LimboTimeSeconds) * time.Second

	return nil
}

// Updates the global variable with the radius session server handler configuration
func (c *RadiusSessionServerConfigurationManager) UpdateRadiusSessionServerConfig() error {
	return c.radiusSessionServerConfig.Update(&c.CM)
}

// Retrieves the current radius session server handler configuration
func (c *RadiusSessionServerConfigurationManager) RadiusSessionServerConf() RadiusSessionServerConfig {
	return c.radiusSessionServerConfig.Get()
}
