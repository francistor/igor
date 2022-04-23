package config2

import (
	"encoding/json"
	"errors"
	"igor/diamdict"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// Type ConfigObject holds both the raw text and the
// Unmarshalled JSON if applicable
type ConfigObject struct {
	Json    interface{}
	RawText string
}

// Types for Search Rules
type searchRule struct {
	NameRegex string
	Base      string
	Regex     *regexp.Regexp
}

type searchRules []searchRule

// Holds a configuration instance
// To be retrieved
type ConfigurationManager struct {
	instanceName  string
	bootstrapFile string

	objectCache sync.Map
	sRules      searchRules
	inFlight    sync.Map

	IgorLogger   *zap.SugaredLogger
	DiameterDict diamdict.DiameterDict

	currentDiameterServerConfig DiameterServerConfig
	currentRoutingRules         DiameterRoutingRules
	currentDiameterPeers        DiameterPeers
}

// Slice of configuration managers
// Except during testing, there will be only one instance, which will be retrieved by GetConfig()
var configs []ConfigurationManager

// Called by go once
func init() {
	configs = make([]ConfigurationManager, 0)
}

// Intializes the config object
// To be called only once, from main function
// The instanceName must be unique
func InitConfigurationInstance(bootstrapFile string, instanceName string) {

	// Check not already instantiated
	for i, _ := range configs {
		if configs[i].instanceName == instanceName {
			panic(instanceName + " already initalized")
		}
	}

	// Better to create asap
	config := ConfigurationManager{}
	configs = append(configs, config)

	config.bootstrapFile = bootstrapFile
	config.instanceName = instanceName

	// Intialize logger
	config.IgorLogger = SetupLogger()
	config.IgorLogger.Debugw("Init with instace name", "instance", instanceName)

	// Get the search rules object
	rules, err := config.readResource(bootstrapFile)
	if err != nil {
		panic("Could not retrieve the bootstrap file in " + bootstrapFile)
	}

	config.IgorLogger.Debugw("Read bootstrap file", "contents", rules)

	// Decode Search Rules
	json.Unmarshal([]byte(rules), &config.sRules)
	if len(config.sRules) == 0 {
		panic("Could not decode the Search Rules")
	}
	for i, sr := range config.sRules {
		// Add the compiled regular expression for each rule
		config.sRules[i].Regex, err = regexp.Compile(sr.NameRegex)
		if err != nil {
			panic("Could not compile Search Rule Regex " + sr.NameRegex)
		}
	}

	// Load dictionaries
	diamDictJSON, err := config.GetConfigObjectAsText("diameterDictionary.json")
	if err != nil {
		panic("Could not read diameterDictionary.json")
	}

	config.DiameterDict = diamdict.NewDictionaryFromJSON([]byte(diamDictJSON))

	// Load diameter configuraton
	config.UpdateDiameterServerConfig()
	config.UpdateDiameterPeers()
	config.UpdateDiameterRoutingRules()
}

// Retrieves a specific configuration instance
// Mainly used for testing
func GetConfigurationInstance(instanceName string) ConfigurationManager {

	for i, _ := range configs {
		if configs[i].instanceName == instanceName {
			return configs[i]
		}
	}

	panic("configuraton instance " + instanceName + "not configured")
}

// Retrieves the default configuration instance
func GetConfig() ConfigurationManager {
	return configs[0]
}

// Returns the configuration object as a parsed Json
func (c *ConfigurationManager) GetConfigObjectAsJSon(objectName string) (interface{}, error) {
	co, err := c.GetConfigObject(objectName)
	if err == nil {
		return co.Json, nil
	} else {
		return nil, err
	}
}

// Returns the raw text of the configuration object
func (c *ConfigurationManager) GetConfigObjectAsText(objectName string) (string, error) {
	co, err := c.GetConfigObject(objectName)
	if err == nil {
		return co.RawText, nil
	} else {
		return "", err
	}
}

// Retrieves the object form the cache or tries to get it from the remote
// and caches it if not found
func (c *ConfigurationManager) GetConfigObject(objectName string) (ConfigObject, error) {
	// Try cache
	obj, found := c.objectCache.Load(objectName)
	if found {
		return obj.(ConfigObject), nil
	}

	// Not found. Retrieve
	// InFlight contains a map of object names to Once objects that will retrieve the object
	// from the remote and store in cache. The first requesting goroutine will push the
	// Once to the map, the others will retrieve the once already pushed. The executing
	// Once will delete the entry from the Inflight map
	var once sync.Once
	var flightOncePtr, _ = c.inFlight.LoadOrStore(objectName, &once)

	// Once function
	var retriever = func() {
		obj, err := c.readConfigObject(objectName)
		if err != nil {
			c.IgorLogger.Errorw("could not read config object", "name", objectName, "error", err)
		} else {
			c.objectCache.Store(objectName, obj)
		}
		c.inFlight.Delete(objectName)
	}

	// goroutines will block here until object retreived or failed
	flightOncePtr.(*sync.Once).Do(retriever)

	// Try again
	obj, found = c.objectCache.Load(objectName)
	if found {
		return obj.(ConfigObject), nil
	} else {
		return ConfigObject{}, errors.New("could not get configuration object " + objectName)
	}
}

// Removes a ConfigObject from the cache
func (c *ConfigurationManager) InvalidateConfigObject(objectName string) {
	c.objectCache.Delete(objectName)
}

/////////////////////////////////////////////////////////////////
// Helper functions
/////////////////////////////////////////////////////////////////

// Finds the remote from the SearchRules and reads the object
func (c *ConfigurationManager) readConfigObject(objectName string) (ConfigObject, error) {

	configObject := ConfigObject{}

	// Iterate through Search Rules
	var base string
	var innerName string
	for _, rule := range c.sRules {
		matches := rule.Regex.FindStringSubmatch(objectName)
		if matches != nil {
			innerName = matches[1]
			base = rule.Base
		}
	}
	if base == "" {
		// Not found
		return configObject, errors.New("object name does not match any rules")
	}

	// Found, base var contains the prefix
	var objectLocation string

	// Try first with instance name
	if c.instanceName != "" {
		objectLocation = base + c.instanceName + "/" + innerName
		object, err := c.readResource(objectLocation)
		if err == nil {
			return newConfigObjectFromString(object), nil
		}
	}

	// Try without instance name.
	objectLocation = base + innerName
	object, err := c.readResource(objectLocation)
	if err == nil {
		configObject = newConfigObjectFromString(object)
	}

	return configObject, err
}

// Reads the configuration item from the specified location, which may be
// a file or an http url
func (c *ConfigurationManager) readResource(location string) (string, error) {

	if strings.HasPrefix(location, "http") {

		// Location is a http URL
		resp, err := http.Get(location)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil

	} else {

		c.IgorLogger.Debugw("Reading Configuration file", "fileName", os.Getenv("IGOR_CONFIG_BASE")+location)
		resp, err := ioutil.ReadFile(os.Getenv("IGOR_CONFIG_BASE") + location)
		if err != nil {
			c.IgorLogger.Debugw("resource not found", "file", location, "error", err)
			return "", err
		}
		c.IgorLogger.Debugw("resource found", "file", location)
		return string(resp), err
	}
}

// Takes a raw string and turns it into a ConfigObject, which is
// trying to parse the string as Json and returing both the
// original string and the JSON in a composite ConfigObject
func newConfigObjectFromString(object string) ConfigObject {
	configObject := ConfigObject{
		RawText: object,
	}
	json.Unmarshal([]byte(object), &configObject.Json)

	return configObject
}
