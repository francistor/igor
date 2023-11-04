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
	"text/template"
	"time"

	"github.com/francistor/igor/cloud"
	"github.com/francistor/igor/resources"
	_ "github.com/go-sql-driver/mysql"
)

// The resources folder is stored in this simulated filesystem
// Any file can be retreived as a resource located in resource://<whatever>

const (
	HTTP_TIMEOUT_SECONDS = 5
)

// Holds a SearchRule, which specifies where to look for a configuration object
type SearchRule struct {
	// Regex for the name of the object. If matching, we'll try to locate
	// it prepending the Origin property to compose the URL (file or http)
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

// The applicable Search Rules. Holds also the configuration for the configuration database
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
// The hierarchy is
// - BuildJSONConfigObject or GetBytesConfigObject or GetRawBytesConfigObject
// - Call getObject. Implements the logic to try first with instance name
// - Call getResource. Reads from database, http or file
// No cache is implemented. Any call to retreive an object will go to the source
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

	// Global configuration parameters. Used as parameters for the configuration
	// objects, if they retrieved as templates.
	configParams map[string]string

	// Database Handle for access to the configuration database
	dbHandle *sql.DB

	// HttpClient
	httpClient *http.Client
}

// The home location for configuration files not referenced as absolute paths
var igorConfigBase string

// Creates and initializes a ConfigurationManager
// The <params> argument is used as parameter to the objects, treated as templates
func NewConfigurationManager(bootstrapFile string, instanceName string, params map[string]string) ConfigurationManager {

	// To avoid null pointers, create an emtpy map if not passed
	if params == nil {
		params = make(map[string]string)
	}

	// Add relevant environment variables to the params object
	for _, envKV := range os.Environ() {
		if strings.HasPrefix(envKV, "igor_") || strings.HasPrefix(envKV, "IGOR_") {
			envKV = strings.TrimPrefix(envKV, "igor_")
			envKV = strings.TrimPrefix(envKV, "IGOR_")

			kv := strings.Split(envKV, "=")
			params[kv[0]] = kv[1]
		}
	}

	cm := ConfigurationManager{
		instanceName:  instanceName,
		bootstrapFile: bootstrapFile,
		configParams:  params,
		httpClient: &http.Client{
			Timeout: HTTP_TIMEOUT_SECONDS * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore invalid SSL certificates
			},
		},
	}

	// Parse the bootstrap configuration file
	icb, bootstrapResource := cm.decomposeFileLocation(bootstrapFile)
	igorConfigBase = icb
	cm.fillSearchRules(icb + bootstrapResource)

	return cm
}

// Parses the object as a template using the parameters ofthe configuration instance.
func (c *ConfigurationManager) untemplateObject(obj []byte) ([]byte, error) {

	// Parse the template
	tmpl, err := template.New("igor_template").Parse(string(obj))
	if err != nil {
		return nil, err
	}
	// Execute the template
	var tmplRes strings.Builder
	if err := tmpl.Execute(&tmplRes, c.configParams); err != nil {
		return nil, err
	}

	return []byte(tmplRes.String()), nil
}

// Fills the object passed as parameter with the configuration object which is
// interpreted as JSON. The contents of the object are treated as a template with
// parameters, which are replaced by the contents of the map passed at initialization
// of the ConfigurationManager
func (c *ConfigurationManager) BuildJSONConfigObject(objectName string, obj any) error {

	jb, err := c.getObject(objectName)
	if err != nil {
		return err
	}

	parsed, err := c.untemplateObject(jb)
	if err != nil {
		return err
	}

	return json.Unmarshal(parsed, obj)
}

// Fills the object passed as parameter with the configuration object which is
// interpreted as raw text. This version treats the contents as a template to be
// parsed
func (c *ConfigurationManager) GetBytesConfigObject(objectName string) ([]byte, error) {

	cb, err := c.getObject(objectName)
	if err != nil {
		return nil, err
	}

	parsed, err := c.untemplateObject(cb)
	if err != nil {
		return nil, err
	}

	return parsed, nil
}

// This version does not treat the object as a template
func (c *ConfigurationManager) GetRawBytesConfigObject(objectName string) ([]byte, error) {
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
			if len(matches) < 1 {
				panic("regular expression without group. Use () to define your object name")
			}
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
		if objectBytes, err := c.readResource("", origin, false); err == nil {
			return objectBytes, nil
		} else {
			return nil, err
		}
	} else {
		// Other object types may have an instance name.
		// Try first with instance name
		var prefix string
		if origin != "" {
			prefix = origin
		} else {
			prefix = igorConfigBase
		}
		if c.instanceName != "" {
			if objectBytes, err := c.readResource(prefix, c.instanceName+"/"+innerName, false); err == nil {
				return objectBytes, nil
			}
		}

		// Try without instance name
		if objectBytes, err := c.readResource(prefix, innerName, false); err == nil {
			return objectBytes, nil
		} else {
			return nil, err
		}
	}
}

// Reads the configuration item from the specified location.
func (c *ConfigurationManager) readResource(prefix string, name string, tryParentForTesting bool) ([]byte, error) {

	// Used except for relative file schema
	location := prefix + name

	if strings.HasPrefix(location, "database") {
		// Format is database:table:keycolumn:paramscolumn
		// The returned object is always a JSON whose first level are properties, not arrays
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
		defer stmt.Close()
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
		resp, err := http.Get(location)
		if err != nil {
			return nil, fmt.Errorf("request for http resource %v got error: %v", location, err)
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

	} else if strings.HasPrefix(location, "gs://") {
		if resp, err := cloud.GetGoogleStorageObject(location); err != nil {
			return nil, err
		} else {
			return resp, nil
		}
	} else if strings.HasPrefix(location, "resource://") {
		if resp, err := resources.Fs.ReadFile(location[11:]); err != nil {
			return nil, err
		} else {
			return resp, nil
		}
	} else {
		// Read from file
		if strings.HasPrefix(location, "/") {
			// Treat as absolute
			if resp, err := os.ReadFile(location); err != nil {
				return nil, err
			} else {
				return resp, nil
			}
		} else {
			// Treat as relative to the bootstrap file
			if resp, err := os.ReadFile(igorConfigBase + name); err != nil {
				if tryParentForTesting {
					if resp, err := os.ReadFile("../" + igorConfigBase + name); err != nil {
						return nil, err
					} else {
						igorConfigBase = "../" + igorConfigBase
						return resp, nil
					}
				}
				return nil, err
			} else {
				return resp, nil
			}
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
	p, n := c.decomposeFileLocation(bootstrapFile)
	rules, err := c.readResource(p, n, true)
	if err != nil {
		panic("could not retrieve the bootstrap file in " + bootstrapFile + " due to: " + err.Error())
	}

	// Parse template
	rules, err = c.untemplateObject(rules)
	if err != nil {
		panic("could not parse the bootstrap file in " + bootstrapFile + " due to: " + err.Error())
	}

	// Decode Search Rules and add them to the ConfigurationManager object
	err = json.Unmarshal(rules, &c.searchRules)
	if err != nil || len(c.searchRules.Rules) == 0 {
		panic("could not decode the Search Rules or the file was empty")
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

			// If IGOR_ABORT_IF_DB_ERROR is defined, panic on database error
			if os.Getenv("IGOR_ABORT_IF_DB_ERROR") != "" {
				err = c.dbHandle.Ping()
				if err != nil {
					// If the database is not available, die
					panic("could not ping database in " + c.searchRules.Db.Url)
				}
			}
		} else {
			panic("db access parameters not specified in searchrules")
		}
	}
}

// Sets the core.igorConfigBase variable as the directory where the bootstrap file resides
// and returns the base location of the configuration, to be used when there is no origin, and
// the name of the bootrstrap file resource
// TODO: Replace by path
func (c *ConfigurationManager) decomposeFileLocation(bootstrapFileName string) (string, string) {

	lastSlash := strings.LastIndex(bootstrapFileName, "/")
	if lastSlash == -1 {
		// Assumed to be the current directory
		abs, err := filepath.Abs(bootstrapFileName)
		if err != nil {
			panic(err)
		}
		return filepath.Dir(abs) + "/", bootstrapFileName
	} else {
		return bootstrapFileName[0 : lastSlash+1], bootstrapFileName[lastSlash+1:]
	}

	/*

		// Skip if file is in a http or gs location
		if strings.HasPrefix(bootstrapFileName, "http:") || strings.HasPrefix(bootstrapFileName, "https:") || strings.HasPrefix(bootstrapFileName, "gs:") {
			return bootstrapFileName
		}

		// Try first with the specification as it is
		if fileInfo, err := os.Stat(bootstrapFileName); err == nil {
			// File found
			abs, err := filepath.Abs(bootstrapFileName)
			if err != nil {
				panic(err)
			}
			igorConfigBase = filepath.Dir(abs) + "/"
			return fileInfo.Name()
		}

		if !tryWithParent {
			panic("could not find the bootstrap file in " + bootstrapFileName)
		} else {
			return c.fixBootstrapFileLocation("../"+bootstrapFileName, false)
		}
	*/
}
