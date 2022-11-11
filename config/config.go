package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	HTTP_TIMEOUT_SECONDS = 5
)

// Custom error type to signal that the object was not found
type ObjectNotFound struct {
	InnerError error
}

func (e *ObjectNotFound) Error() string {
	return "not found"
}

func (e *ObjectNotFound) UnWrap() error {
	return e.InnerError
}

// General utilities to read configuration files, locally or via http

// Type ConfigObject holds both the raw text and the
// Unmarshalled JSON if applicable
type ConfigObject struct {
	Json     interface{}
	RawBytes []byte
}

// Holds a SearchRule, which specifies where to look for a configuration object
type SearchRule struct {
	// Regex for the name of the object. If matching, we'll try to locate
	// it prepending the Base property to compose the URL (file or http)
	// The regex will contain a matching group that will be the part used to
	// look for the object. For instance, in "Gy/(.*)", the part after "Gy/"
	// will be taken as the resource name to look after when retreiving an object
	// name such as Gy/peers.json
	NameRegex string

	// Compiled form of nameRegex
	Regex *regexp.Regexp

	// Can be a URL or a path
	Base string
}

// The applicable Search Rules
type SearchRules []SearchRule

// Basic objects and methods to manage configuration files without yet
// interpreting them. To be embedded in a handlerConfig or policyConfig object
// Multiple "instances" can coexist in a single executable
type ConfigurationManager struct {

	// Configuration objects are to be searched for in a path that contains
	// the instanceName first and, if not found, in a path without it. This
	// way a general configuration can be overriden
	instanceName string

	// The bootstrap file is the first configuration file read, and it contains
	// the rules for searching other files. It can be a local file or a URL
	bootstrapFile string

	// The contents of the bootstrapFile are parsed here
	searchRules SearchRules

	// Cache of the configuration files already read
	objectCache sync.Map

	// inFlight contains a map of object names to Once objects that will retrieve the object
	// from the remote and store in cache. The purpose of using this is to avoid having
	// multiple requests to read the same object at the same time. The second and subsequent
	// ones will block until the first one finishes
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

// Reads the bootstrap file and fills the search rules for the Configuration Manager.
// To be called upon instantiation of the ConfigurationManager.
// The bootstrap file is not subject to instance searching
func (c *ConfigurationManager) fillSearchRules(bootstrapFile string) {
	// Get the search rules object
	rules, err := c.readResource(bootstrapFile)
	if err != nil {
		panic("could not retrieve the bootstrap file in " + bootstrapFile)
	}

	// Decode Search Rules and add them to the ConfigurationManager object
	err = json.Unmarshal(rules, &c.searchRules)
	if err != nil || len(c.searchRules) == 0 {
		panic("could not decode the Search Rules")
	}

	// Add the compiled regular expression for each rule
	for i, sr := range c.searchRules {
		if c.searchRules[i].Regex, err = regexp.Compile(sr.NameRegex); err != nil {
			panic("could not compile Search Rule Regex: " + sr.NameRegex)
		}
	}
}

// Returns the configuration object as a parsed Json. Returns a copy
func (c *ConfigurationManager) GetConfigObjectAsJson(objectName string, refresh bool) (interface{}, error) {
	if co, err := c.GetConfigObject(objectName, refresh); err == nil {
		return co.Json, nil
	} else {
		return nil, err
	}
}

// Returns the contents of a specific key in the JSON configuration object
func (c *ConfigurationManager) GetConfigObjectKeyAsJson(objectName string, key string, refresh bool) (interface{}, error) {
	if co, err := c.GetConfigObject(objectName, refresh); err == nil {
		switch co.Json.(type) {
		case map[string]interface{}:
			if value, found := co.Json.(map[string]interface{})[key]; found {
				return value, nil
			} else {
				return nil, fmt.Errorf("%s.%s not found", objectName, key)
			}
		default:
			return nil, fmt.Errorf("%s is not a JSON properties object", objectName)
		}

	} else {
		return nil, err
	}
}

// Returns the raw text of the configuration object
func (c *ConfigurationManager) GetConfigObjectAsText(objectName string, refresh bool) ([]byte, error) {
	if co, err := c.GetConfigObject(objectName, refresh); err == nil {
		return co.RawBytes, nil
	} else {
		return nil, err
	}
}

// Retrieves the object form the cache or tries to get it from the remote
// and caches it if not found. If refresh is true, ignores the contents of the
// cache and tries to retrieve a fresh copy
func (c *ConfigurationManager) GetConfigObject(objectName string, refresh bool) (*ConfigObject, error) {
	// Try cache
	if !refresh {
		if obj, found := c.objectCache.Load(objectName); found {
			return obj.(*ConfigObject), nil
		}
	}

	// Not found or forced refresh. Retrieve object from origin.
	// InFlight contains a map of object names to Once objects that will retrieve the object
	// from the remote and store in cache. The first requesting goroutine will push the
	// Once to the map, the others will retrieve the Once already pushed. The executing
	// Once will delete the entry from the Inflight map

	// Only one Once will be stored in the inFlight map. Late request will create a Once
	// that will not be used
	var once sync.Once
	var flightOncePtr, _ = c.inFlight.LoadOrStore(objectName, &once)

	// Once function
	var retriever = func() {
		if obj, err := c.readConfigObject(objectName); err == nil {
			c.objectCache.Store(objectName, &obj)
		} else {
			// Logger may not yet be initialized
			if logger := GetLogger(); logger != nil {
				GetLogger().Errorf("error retrieving %s: %v", objectName, err)
			}
		}
		c.inFlight.Delete(objectName)
	}

	// goroutines will block here until object retreived or failed
	flightOncePtr.(*sync.Once).Do(retriever)

	// Try again
	if obj, found := c.objectCache.Load(objectName); found {
		return obj.(*ConfigObject), nil
	} else {
		return &ConfigObject{}, errors.New("could not get configuration object " + objectName)
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

	for _, rule := range c.searchRules {
		if matches := rule.Regex.FindStringSubmatch(objectName); matches != nil {
			innerName = matches[1]
			base = rule.Base
		}
	}
	if base == "" {
		// Not found
		return configObject, errors.New("object name does not match any rules")
	}

	// Found, base var contains the prefix

	// Try first with instance name
	if c.instanceName != "" {
		if object, err := c.readResource(base + c.instanceName + "/" + innerName); err == nil {
			return newConfigObjectFromBytes(object), nil
		}
	}

	// Try without instance name
	if object, err := c.readResource(base + innerName); err == nil {
		return newConfigObjectFromBytes(object), nil
	} else {
		return configObject, err
	}
}

// Reads the configuration item from the specified location, which may be
// a file or an http url
func (c *ConfigurationManager) readResource(location string) ([]byte, error) {

	if strings.HasPrefix(location, "http") {
		// Read from http

		// Create client with timeout
		httpClient := http.Client{Timeout: HTTP_TIMEOUT_SECONDS * time.Second}

		// Location is a http URL
		resp, err := httpClient.Get(location)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if body, err := io.ReadAll(resp.Body); err != nil {
			return nil, err
		} else {
			return body, nil
		}

	} else {
		// Read from file
		configBase := os.Getenv("IGOR_BASE")
		if configBase == "" {
			panic("environment variable IGOR_BASE undefined")
		}
		if resp, err := os.ReadFile(configBase + location); err != nil {
			return nil, err
		} else {
			return resp, nil
		}
	}
}

// Takes a raw string and turns it into a ConfigObject,
// trying to parse the string as JSON
func newConfigObjectFromBytes(object []byte) ConfigObject {
	configObject := ConfigObject{
		RawBytes: object,
	}
	json.Unmarshal(object, &configObject.Json)

	return configObject
}
