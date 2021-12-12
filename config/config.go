package config

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
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

// Type for config
type ConfigManager struct {
	ObjectCache sync.Map
}

// The singleton configuration
var Config ConfigManager

// Logging
var sl *zap.SugaredLogger

func init() {
	// Logging
	logger, _ := zap.NewDevelopment()
	sl = logger.Sugar()
	sl.Infof("Logger initialized")

	// Create the cache
	Config.ObjectCache = sync.Map{}

	// Get the search rules
	bootPtr := flag.String("boot", "resources/diameterDictionary.txt", "File or http URL with Configuration Search Rules")
	flag.Parse()
	if bootPtr != nil {
		rules, err := ReadResource(*bootPtr)
		if err != nil {
			panic("Could not retrieve the bootstrap file in " + *bootPtr)
		} else {
			fmt.Println("-------------------")
			fmt.Println(rules)
		}
	}
}

func (c ConfigObject) GetConfigObjectAsJSon(objectName string) (interface{}, error) {
	return nil, nil
}

func (c ConfigObject) GetConfigObjectAsText(objectName string) (string, error) {
	return "", nil
}

// Retrieves the object form the cache or tries to get it from the remote
// and caches it if not found
func (c ConfigObject) GetConfigObject(objectName string) (ConfigObject, error) {
	return ConfigObject{}, nil
}

// Finds the remote from the SearchRules and reads the object
func ReadConfigObject(objectName string) (ConfigObject, error) {

	return ConfigObject{}, nil
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

		// Location is local file
		resp, err := ioutil.ReadFile(location)
		if err != nil {
			sl.Errorw("Error reading resource", "file", location, "error", err)
			return "", err
		}
		return string(resp), err
	}
}
