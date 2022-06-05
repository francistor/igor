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
}

// Creates and initializes a ConfigurationManager
func NewConfigurationManager(bootstrapFile string, instanceName string) ConfigurationManager {
	return ConfigurationManager{
		instanceName:  instanceName,
		bootstrapFile: bootstrapFile,
		objectCache:   sync.Map{},
		inFlight:      sync.Map{},
	}
}

func (c *ConfigurationManager) FillSearchRules(bootstrapFile string) {
	// Get the search rules object
	rules, err := c.readResource(bootstrapFile)
	if err != nil {
		panic("Could not retrieve the bootstrap file in " + bootstrapFile)
	}

	// Decode Search Rules
	json.Unmarshal([]byte(rules), &c.sRules)
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
func (c *ConfigurationManager) GetConfigObjectAsJson(objectName string) (interface{}, error) {
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
				GetLogger().Errorf("error retrieving %s: %v", err)
			}
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

	// Try without instance name
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

		resp, err := ioutil.ReadFile(os.Getenv("IGOR_CONFIG_BASE") + location)
		if err != nil {
			return "", err
		}
		return string(resp), nil
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
