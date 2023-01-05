package core

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	HTTP_TIMEOUT_SECONDS = 5
)

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
	Origin string
}

// The applicable Search Rules. Hold also the configuration for the configuration database
type SearchRules struct {
	Rules []SearchRule
	Db    struct {
		Url          string
		Driver       string
		MaxOpenConns int
	}
}

// Basic objects and methods to manage configuration files without yet
// interpreting them. To be embedded in a handlerConfig or policyConfig object
// Multiple "instances" can coexist in a single executable (mainly for testing)
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

	// Database Handle for access to the configuration database
	dbHandle *sql.DB
}

// The home location for configuration files not referenced as absolute paths
var IgorConfigBase string

// Creates and initializes a ConfigurationManager
func NewConfigurationManager(bootstrapFile string, instanceName string) ConfigurationManager {
	cm := ConfigurationManager{
		instanceName:  instanceName,
		bootstrapFile: bootstrapFile,
	}

	cm.fillSearchRules(cm.fixBootstrapFileLocation(bootstrapFile, true))

	return cm
}

// Fills the object passed as parameter with the configuration object which is
// interpreted as JSON
func (c *ConfigurationManager) BuildJSONConfigObject(objectName string, obj any) error {

	jb, err := c.getObject(objectName)
	if err != nil {
		return err
	}
	return json.Unmarshal(jb, obj)
}

// Fills the object passed as parameter with the configuration object which is
// interpreted as raw text
func (c *ConfigurationManager) GetBytesConfigObject(objectName string) ([]byte, error) {

	return c.getObject(objectName)
}

// Finds the remote from the SearchRules and reads the object, trying with instance
// name first, and then global
func (c *ConfigurationManager) getObject(objectName string) ([]byte, error) {

	// Iterate through Search Rules
	var origin string
	var innerName string

	for _, rule := range c.searchRules.Rules {
		if matches := rule.Regex.FindStringSubmatch(objectName); matches != nil {
			innerName = matches[1]
			origin = rule.Origin
			break
		}
	}
	if innerName == "" {
		// Not found
		return nil, errors.New("object name does not match any rules")
	}

	// Found, origin var contains the prefix

	if strings.HasPrefix(origin, "database:") {
		// Database object
		if objectBytes, err := c.readResource(origin); err == nil {
			return objectBytes, nil
		} else {
			return nil, err
		}
	} else {
		// File object
		// Try first with instance name
		if c.instanceName != "" {
			if objectBytes, err := c.readResource(origin + c.instanceName + "/" + innerName); err == nil {
				return objectBytes, nil
			}
		}

		// Try without instance name
		if objectBytes, err := c.readResource(origin + innerName); err == nil {
			return objectBytes, nil
		} else {
			return nil, err
		}
	}

}

// Reads the configuration item from the specified location, which may be
// a file or an http(s) url
func (c *ConfigurationManager) readResource(location string) ([]byte, error) {

	if strings.HasPrefix(location, "database") {
		// Format is database:table:keycolumn:paramscolumn
		// The returned object is always a JSON whosw first level are properties, not arrays
		// as per the values of the keycolumn
		items := strings.Split(location, ":")
		tableName := items[1]
		keyColumn := items[2]
		paramsColumn := items[3]

		// This is the object that will be returned
		entries := make(map[string]*json.RawMessage)

		stmt, err := c.dbHandle.Prepare(fmt.Sprintf("select %s, %s from %s", keyColumn, paramsColumn, tableName))
		if err != nil {
			return nil, fmt.Errorf("error reading from database. %s, %w", location, err)
		}
		rows, err := stmt.Query()
		if err != nil {
			return nil, fmt.Errorf("error reading from database. %s, %w", location, err)
		}
		defer rows.Close()

		var k string
		for rows.Next() {
			var v json.RawMessage
			err := rows.Scan(&k, &v)
			if err != nil {
				return nil, fmt.Errorf("error reading from database. %s, %w", location, err)
			}
			entries[k] = &v
		}
		err = rows.Err()
		if err != nil {
			return nil, fmt.Errorf("error reading from database. %s, %w", location, err)
		}

		return json.Marshal(entries)

	} else if strings.HasPrefix(location, "http:") || strings.HasPrefix(location, "https:") {
		// Read from http

		// Create client with timeout
		httpClient := http.Client{
			Timeout: HTTP_TIMEOUT_SECONDS * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
			},
		}

		// Location is a http URL
		resp, err := httpClient.Get(location)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("got status code %d while retrieving %s", resp.StatusCode, location)
		}
		if body, err := io.ReadAll(resp.Body); err != nil {
			return nil, err
		} else {
			return body, nil
		}

	} else {
		// Read from file
		/*
			configBase := os.Getenv("IGOR_BASE")
			if configBase == "" {
				panic("environment variable IGOR_BASE undefined")
			}
		*/
		if resp, err := os.ReadFile(IgorConfigBase + location); err != nil {
			return nil, err
		} else {
			return resp, nil
		}
	}
}

// Reads the bootstrap file and fills the search rules for the Configuration Manager.
// To be called upon instantiation of the ConfigurationManager.
// The bootstrap file is not subject to instance searching rules: must reside in the specified location without
// appending instance name
func (c *ConfigurationManager) fillSearchRules(bootstrapFile string) {
	var shouldInitDB bool

	// Get the search rules object
	rules, err := c.readResource(bootstrapFile)
	if err != nil {
		panic("could not retrieve the bootstrap file in " + bootstrapFile)
	}

	// Decode Search Rules and add them to the ConfigurationManager object
	err = json.Unmarshal(rules, &c.searchRules)
	if err != nil || len(c.searchRules.Rules) == 0 {
		panic("could not decode the Search Rules or empty file")
	}

	// Add the compiled regular expression for each rule and sanity check for base
	for i, sr := range c.searchRules.Rules {
		if c.searchRules.Rules[i].Regex, err = regexp.Compile(sr.NameRegex); err != nil {
			panic("could not compile Search Rule Regex: " + sr.NameRegex)
		}
		origin := c.searchRules.Rules[i].Origin
		if strings.HasPrefix(origin, "database") {
			shouldInitDB = true
			if len(strings.Split(c.searchRules.Rules[i].Origin, ":")) != 4 {
				panic("bad format for database search rule: " + origin)
			}
		}
	}

	// Create database handle
	if shouldInitDB {
		if c.searchRules.Db.Driver != "" && c.searchRules.Db.Url != "" {
			c.dbHandle, err = sql.Open(c.searchRules.Db.Driver, c.searchRules.Db.Url)
			if err != nil {
				panic("could not create database object " + c.searchRules.Db.Driver)
			}
			c.dbHandle.SetMaxOpenConns(c.searchRules.Db.MaxOpenConns)

			err = c.dbHandle.Ping()
			if err != nil {
				// If the database is not available, die
				panic("could not ping database in " + c.searchRules.Db.Url)
			}
		} else {
			panic("db access parameters not specified in searchrules")
		}
	}
}

// Sets the core.IgorConfigBase variable as the directory where the bootstrap file resides
// and returns the normalized location of that bootstrap file, looking for it in the current
// directory and in the parent directory, which is useful for tests
func (c *ConfigurationManager) fixBootstrapFileLocation(bootstrapFileName string, tryWithParent bool) string {

	// Skip if file is in a http location
	if strings.HasPrefix(bootstrapFileName, "http:") || strings.HasPrefix(bootstrapFileName, "https:") {
		return bootstrapFileName
	}

	// Try first with the specification as it is
	if fileInfo, err := os.Stat(bootstrapFileName); err == nil {
		// File found
		abs, err := filepath.Abs(bootstrapFileName)
		if err != nil {
			panic(err)
		}
		IgorConfigBase = filepath.Dir(abs) + "/"
		return fileInfo.Name()
	}

	if !tryWithParent {
		panic("could not find the bootstrap file in " + bootstrapFileName)
	} else {
		return c.fixBootstrapFileLocation("../"+bootstrapFileName, false)
	}
}
