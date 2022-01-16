package config

// Set IGOR_CONFIG_BASE environment variable to the absolute location of the resource directory (finishing in slash)
// otherwise, file resources will be looked up in the executing directory

// The configuration must be inited once at application startup, invoking Config.Init()

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

// Type for config
type ConfigManager struct {
	InstanceName string
	objectCache  sync.Map
	sRules       searchRules
	inFlight     sync.Map
}

// Singletons
var Config ConfigManager
var DDict diamdict.DiameterDict

// Automatically called by go at startup. Makes sure there
// is a "Config" singleton object
func init() {

	Config = ConfigManager{
		objectCache: sync.Map{},
	}
}

// Intializes the config object
// To be called only once, from main function
func (c *ConfigManager) Init(bootstrapFile string, instanceName string) {

	IgorLogger.Debugw("Init with instace name", "instance", instanceName)
	c.InstanceName = instanceName

	// Get the search rules object
	rules, err := ReadResource(bootstrapFile)
	if err != nil {
		panic("Could not retrieve the bootstrap file in " + bootstrapFile)
	}

	IgorLogger.Debugw("Read bootstrap file", "contents", rules)

	// Decode Search Rules
	json.Unmarshal([]byte(rules), &Config.sRules)
	if len(Config.sRules) == 0 {
		panic("Could not decode the Search Rules")
	}
	for i, sr := range Config.sRules {
		// Add the compiled regular expression for each rule
		Config.sRules[i].Regex, err = regexp.Compile(sr.NameRegex)
		if err != nil {
			panic("Could not compile Search Rule Regex " + sr.NameRegex)
		}
	}

	// Load dictionaries
	diamDictJSON, err := Config.GetConfigObjectAsText("diameterDictionary.json")
	if err != nil {
		panic("Could not read diameterDictionary.json")
	}

	DDict = diamdict.NewDictionaryFromJSON([]byte(diamDictJSON))
}

// Returns the configuration object as a parsed Json
func (c *ConfigManager) GetConfigObjectAsJSon(objectName string) (interface{}, error) {
	co, err := c.GetConfigObject(objectName)
	if err == nil {
		return co.Json, nil
	} else {
		return nil, err
	}
}

// Returns the raw text of the configuration object
func (c *ConfigManager) GetConfigObjectAsText(objectName string) (string, error) {
	co, err := c.GetConfigObject(objectName)
	if err == nil {
		return co.RawText, nil
	} else {
		return "", err
	}
}

// Retrieves the object form the cache or tries to get it from the remote
// and caches it if not found
func (c *ConfigManager) GetConfigObject(objectName string) (ConfigObject, error) {
	// Try cache
	obj, found := Config.objectCache.Load(objectName)
	if found {
		return obj.(ConfigObject), nil
	}

	// Not found. Retrieve
	// InFlight contains a map of object names to Once objects that will retrieve the object
	// from the remote and store in cache. The first requesting goroutine will push the
	// once to the map, the others will retrieve the once already pushed. The executing
	// once will delete the entry from the Inflight map
	var once sync.Once
	var flightOncePtr, _ = Config.inFlight.LoadOrStore(objectName, &once)

	// Once function
	var retriever = func() {
		obj, err := ReadConfigObject(objectName)
		if err != nil {
			IgorLogger.Errorw("Could not read config object", "name", objectName, "error", err)
		} else {
			Config.objectCache.Store(objectName, obj)
		}
		Config.inFlight.Delete(objectName)
	}

	// goroutines will block here until object retreived or failed
	flightOncePtr.(*sync.Once).Do(retriever)

	// Try again
	obj, found = Config.objectCache.Load(objectName)
	if found {
		return obj.(ConfigObject), nil
	} else {
		return ConfigObject{}, errors.New("could not get configuration object " + objectName)
	}
}

// Removes a ConfigObject from the cache
func (c *ConfigManager) InvalidateConfigObject(objectName string) {
	c.objectCache.Delete(objectName)
}

// Finds the remote from the SearchRules and reads the object
func ReadConfigObject(objectName string) (ConfigObject, error) {

	configObject := ConfigObject{}

	// Iterate through Search Rules
	var base string
	var innerName string
	for _, rule := range Config.sRules {
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
	if Config.InstanceName != "" {
		objectLocation = base + Config.InstanceName + "/" + innerName
		object, err := ReadResource(objectLocation)
		if err == nil {
			return newConfigObjectFromString(object), nil
		}
	}

	// Try without instance name.
	objectLocation = base + innerName
	object, err := ReadResource(objectLocation)
	if err == nil {
		configObject = newConfigObjectFromString(object)
	}

	return configObject, err
}

// Reads the configuration item from the specified location, which may be
// a file or an http url
func ReadResource(location string) (string, error) {

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

		IgorLogger.Debugw("Reading Configuration file", "fileName", os.Getenv("IGOR_CONFIG_BASE")+location)
		resp, err := ioutil.ReadFile(os.Getenv("IGOR_CONFIG_BASE") + location)
		if err != nil {
			IgorLogger.Debugw("Resource not found", "file", location, "error", err)
			return "", err
		}
		IgorLogger.Debugw("Resource found", "file", location, "error", err)
		return string(resp), err
	}
}

// Takes a raw string and turns it into a ConfigObject, which is
// trying to parse the string as Json and returing both the
// original string and the JSON in a composite Configobject
func newConfigObjectFromString(object string) ConfigObject {
	configObject := ConfigObject{
		RawText: object,
	}
	json.Unmarshal([]byte(object), &configObject.Json)

	return configObject
}
