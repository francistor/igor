package config

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
)

// General utilities to read configuration files, locally or via http

// Type ConfigObject holds both the raw text and the
// Unmarshalled JSON if applicable
type ConfigObject struct {
	Json     interface{}
	RawBytes []byte
}

// Types for Search Rules
type searchRule struct {
	// Regex for the name of the object. If matching, we'll try to locate
	// it prepending the Base property to compose the URL (file or http)
	NameRegex string

	// Compiled form of NameRegex
	Regex *regexp.Regexp

	// Can be a URL or a path
	Base string
}

// Tye applicable Search Rules
type searchRules []searchRule

// Holds a configuration instance
// To be embedded in a handlerConfig or policyConfig object
// Includes the basic methods to manage configuration files
// without interpreting them.
type ConfigurationManager struct {

	// Configuration objects are to be searched for in a path that contains
	// the instanceName first and, if not found, in a path without it. This
	// way a general configuration can be overriden
	instanceName string

	// The bootstrap file is the first configuration file read, and it contains
	// the rules for searching other files
	bootstrapFile string

	// The contents of the bootstrapFile are parsed here
	sRules searchRules

	// Cache of the configuration files already red
	objectCache sync.Map

	// inFlight contains a map of object names to Once objects that will retrieve the object
	// from the remote and store in cache.
	inFlight sync.Map
}

// Creates and initializes a ConfigurationManager
func NewConfigurationManager(bootstrapFile string, instanceName string) ConfigurationManager {
	cm := ConfigurationManager{
		instanceName:  instanceName,
		bootstrapFile: bootstrapFile,
		objectCache:   sync.Map{},
		inFlight:      sync.Map{},
	}

	cm.fillSearchRules(bootstrapFile)

	return cm
}

// Reads the bootstrap file and fills the search rules for the Configuration Manager
// To be called upon instantiation
func (c *ConfigurationManager) fillSearchRules(bootstrapFile string) {
	// Get the search rules object
	rules, err := c.readResource(bootstrapFile)
	if err != nil {
		panic("Could not retrieve the bootstrap file in " + bootstrapFile)
	}

	// Decode Search Rules
	json.Unmarshal(rules, &c.sRules)
	if len(c.sRules) == 0 {
		panic("Could not decode the Search Rules")
	}
	for i, sr := range c.sRules {
		// Add the compiled regular expression for each rule
		c.sRules[i].Regex, err = regexp.Compile(sr.NameRegex)
		if err != nil {
			panic("Could not compile Search Rule Regex " + sr.NameRegex)
		}
	}
}

// Returns the configuration object as a parsed Json
func (c *ConfigurationManager) GetConfigObjectAsJson(objectName string, refresh bool) (interface{}, error) {
	co, err := c.GetConfigObject(objectName, refresh)
	if err == nil {
		return co.Json, nil
	} else {
		return nil, err
	}
}

// Returns the raw text of the configuration object
func (c *ConfigurationManager) GetConfigObjectAsText(objectName string, refresh bool) ([]byte, error) {
	co, err := c.GetConfigObject(objectName, refresh)
	if err == nil {
		return co.RawBytes, nil
	} else {
		return nil, err
	}
}

// Retrieves the object form the cache or tries to get it from the remote
// and caches it if not found
func (c *ConfigurationManager) GetConfigObject(objectName string, refresh bool) (ConfigObject, error) {
	// Try cache
	if !refresh {
		obj, found := c.objectCache.Load(objectName)
		if found {
			return obj.(ConfigObject), nil
		}
	}

	// Not found. Retrieve
	// InFlight contains a map of object names to Once objects that will retrieve the object
	// from the remote and store in cache. The first requesting goroutine will push the
	// Once to the map, the others will retrieve the once already pushed. The executing
	// Once will delete the entry from the Inflight map

	// Only one Once will be stored in the inFlight map. Late request will create a Once
	// that will not be used
	var once sync.Once
	var flightOncePtr, _ = c.inFlight.LoadOrStore(objectName, &once)

	// Once function
	var retriever = func() {
		obj, err := c.readConfigObject(objectName)
		if err == nil {
			c.objectCache.Store(objectName, obj)
		} else {
			if logger := GetLogger(); logger != nil {
				GetLogger().Errorf("error retrieving %s: %v", objectName, err)
			}
		}
		c.inFlight.Delete(objectName)
	}

	// goroutines will block here until object retreived or failed
	flightOncePtr.(*sync.Once).Do(retriever)

	// Try again
	obj, found := c.objectCache.Load(objectName)
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
			return newConfigObjectFromBytes(object), nil
		}
	}

	// Try without instance name
	objectLocation = base + innerName
	object, err := c.readResource(objectLocation)
	if err == nil {
		configObject = newConfigObjectFromBytes(object)
	}

	return configObject, err
}

// Reads the configuration item from the specified location, which may be
// a file or an http url
func (c *ConfigurationManager) readResource(location string) ([]byte, error) {

	if strings.HasPrefix(location, "http") {

		// Location is a http URL
		resp, err := http.Get(location)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return body, nil

	} else {

		resp, err := ioutil.ReadFile(os.Getenv("IGOR_CONFIG_BASE") + location)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

// Takes a raw string and turns it into a ConfigObject, which is
// trying to parse the string as Json and returing both the
// original string and the JSON in a composite ConfigObject
func newConfigObjectFromBytes(object []byte) ConfigObject {
	configObject := ConfigObject{
		RawBytes: object,
	}
	json.Unmarshal(object, &configObject.Json)

	return configObject
}
