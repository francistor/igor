package core

import (
	"crypto/tls"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
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
	// compose the file or http url name to retrieve. For instance, in "Gy/(.*)",
	// the part after "Gy/" will be taken as the last part of the resource name
	// retrieve, e.g. Gy/peers.json --> http://<host:port>/base/peers.json

	NameRegex string

	// Compiled form of nameRegex
	Regex *regexp.Regexp

	// Can be a URL or a path. URLs may include http://, file:// but also
	// local://, and resource://. db: is also treated specially
	Origin string
}

// The applicable Search Rules. Holds also the settings for the configuration database
type SearchRules struct {
	Rules []SearchRule
	Db    struct {
		Url          string
		Driver       string
		MaxOpenConns int
	}
}

// Basic objects and methods to manage configuration object names without yet
// interpreting them. To be embedded in a handlerConfig or policyConfig object
// Multiple "instances" can coexist in a single executable (mainly for testing)
// The call stack is as follows:
// - BuildJSONConfigObject or GetBytesConfigObject or GetRawBytesConfigObject
// - Call GetObject. Implements the logic to try first with instance name
// - Call readResource. Reads from database, http or file
// No cache is implemented. Any call to retreive an object will go to the source
type ConfigurationManager struct {

	// Configuration objects are to be searched for in a path that contains
	// the instanceName first and, if not found, in a path without it. This
	// way a general and shared configuration can be overriden
	instanceName string

	// The bootstrap file is the first resource read, and it contains
	// the rules for searching other resources. It can be a local file or a URL
	bootstrapFile string

	// The contents of the bootstrapFile are parsed and stored here
	searchRules SearchRules

	// Global configuration parameters. Used as parameters for the configuration
	// objects, when they are as templates.
	configParams map[string]string

	// Database Handle for access to the configuration database
	dbHandle *sql.DB

	// HttpClient for retrieving http resources
	httpClient *http.Client

	// Filesystem with embedded resources. Additional to resources.Fs, which will
	// hold the igor library resources, this one is used to store resources
	// created in the user applications, when origin is set to local://
	localFS embed.FS
}

// The home location for configuration files not referenced as absolute paths
var igorConfigBase string

// Creates and initializes a ConfigurationManager
// The <params> argument is used as parameter to the objects, treated as templates
func NewConfigurationManager(bootstrapFile string, instanceName string, params map[string]string, localFs embed.FS) ConfigurationManager {

	// To avoid null pointers, create an emtpy map if not passed
	if params == nil {
		params = make(map[string]string)
	}

	// Add relevant environment variables to the params object
	var rx *regexp.Regexp = regexp.MustCompile("(?i)IGOR_(.+)=(.+)")
	for _, envKV := range os.Environ() {
		if match := rx.FindStringSubmatch(envKV); match != nil {
			params[match[1]] = match[2]
		}
	}

	cm := ConfigurationManager{
		instanceName:  instanceName,
		bootstrapFile: bootstrapFile,
		configParams:  params,
		localFS:       localFs,
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

	// Hack for testing. The location of the bootstrap file may be in the parent directory of
	// the specified one. If this is the case, patch igorConfigBase
	if _, error := cm.readResource(igorConfigBase, bootstrapResource); error != nil {
		if strings.Contains(bootstrapResource, "://") {
			panic("could  not read bootstrap resource from " + bootstrapFile)
		}
		igorConfigBase = path.Clean("../"+igorConfigBase) + "/" // Clean removes the trailing slash
		// Try again here, just to show clean error
		if _, error := cm.readResource(igorConfigBase, bootstrapResource); error != nil {
			panic("could not read bootstrap file from " + bootstrapFile + " or its parent directory (hack testing only) " + error.Error())
		}
	}
	// End of hack

	cm.fillSearchRules(igorConfigBase, bootstrapResource)

	return cm
}

// Parses the object as a template using the parameters of the configuration instance.
func (c *ConfigurationManager) untemplateObject(obj []byte) ([]byte, error) {

	// Parse the template. The name is irrelevant
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
func (c *ConfigurationManager) BuildObjectFromJsonConfig(objectName string, obj any) error {

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

// Retrieves and untemplates the specified object name
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

	// database objects do not have an instance name
	if strings.HasPrefix(origin, "database:") {
		// Database object
		objectBytes, err := c.readResource("", origin)
		if err != nil {
			return nil, err
		}
		return objectBytes, nil
	}

	// Other object types may have an instance name.

	// Cook origin, explicit or implicit
	var prefix = origin
	if prefix == "" {
		prefix = igorConfigBase
	}

	// Try first with instance name
	if c.instanceName != "" {
		if objectBytes, err := c.readResource(prefix, c.instanceName+"/"+innerName); err == nil {
			return objectBytes, nil
		}
	}

	// Try without instance name
	objectBytes, err := c.readResource(prefix, innerName)
	if err != nil {
		return nil, err
	}
	return objectBytes, nil
}

// Reads the configuration item from the specified location.
func (c *ConfigurationManager) readResource(prefix string, name string) ([]byte, error) {

	// Used except for relative file schema
	location := prefix + name

	if strings.HasPrefix(location, "database") {
		// Format is database:table:keycolumn:paramscolumn
		// The returned object is always a JSON whose first level are properties, not arrays
		// as per the values of the keycolumn
		items := strings.Split(location, ":")
		if len(items) != 4 {
			panic("database origin has format not matching database:table:keycolumn:paramscolumn")
		}
		tableName := items[1]
		keyColumn := items[2]
		paramsColumn := items[3]

		// This is the object that will be returned
		entries := make(map[string]*json.RawMessage)

		stmt, err := c.dbHandle.Prepare(fmt.Sprintf("select %s, %s from %s", keyColumn, paramsColumn, tableName))
		if err != nil {
			return nil, fmt.Errorf("error preparing statement: %s, %w", location, err)
		}
		defer stmt.Close()
		rows, err := stmt.Query()
		if err != nil {
			return nil, fmt.Errorf("error reading from database: %s, %w", location, err)
		}
		defer rows.Close()

		var k string
		for rows.Next() {
			var v json.RawMessage
			err := rows.Scan(&k, &v)
			if err != nil {
				return nil, fmt.Errorf("error reading row from database: %s, %w", location, err)
			}
			entries[k] = &v
		}
		err = rows.Err()
		if err != nil {
			return nil, fmt.Errorf("final error reading from database: %s, %w", location, err)
		}

		return json.Marshal(entries)

	}

	if strings.HasPrefix(location, "http:") || strings.HasPrefix(location, "https:") {

		// Read from http
		resp, err := http.Get(location)
		if err != nil {
			return nil, fmt.Errorf("request for http resource %v got error: %v", location, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("got status code %d while retrieving %s", resp.StatusCode, location)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return body, nil
	}

	if strings.HasPrefix(location, "gs://") {
		resp, err := cloud.GetGoogleStorageObject(location)
		if err != nil {
			return nil, err
		}

		return resp, nil
	}

	if strings.HasPrefix(location, "resource://") {
		resp, err := resources.Fs.ReadFile(location[11:])
		if err != nil {
			return nil, err
		}

		return resp, nil
	}

	if strings.HasPrefix(location, "local://") {
		resp, err := c.localFS.ReadFile(location[8:])
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// Read from file (absolute)
	if strings.HasPrefix(location, "/") {
		// Treat as absolute
		resp, err := os.ReadFile(location)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// Treat as relative to the bootstrap file
	resp, err := os.ReadFile(igorConfigBase + name)
	//resp, err := os.ReadFile(location)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Reads the bootstrap file and fills the search rules for the Configuration Manager.
// To be called upon instantiation of the ConfigurationManager.
// The bootstrap file is not subject to instance searching rules: must reside in the specified location without
// appending instance name
func (c *ConfigurationManager) fillSearchRules(base string, bootstrapFile string) {
	var shouldInitDB bool

	// Get the search rules object
	rules, err := c.readResource(base, bootstrapFile)
	if err != nil {
		panic("could not retrieve the bootstrap file in " + base + "/" + bootstrapFile + " due to: " + err.Error())
	}

	// Parse template
	rules, err = c.untemplateObject(rules)
	if err != nil {
		panic("could not parse the bootstrap file in " + base + "/" + bootstrapFile + " due to: " + err.Error())
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

// Returns the igorBase and the resource name for the bootstrap object, calculated as the
// base path and file name respectively. If bootstrapFileName is a file name without path or
// URL specification, it is asumed to reside in the current directory, which is set as igorBase
func (c *ConfigurationManager) decomposeFileLocation(bootstrapFileName string) (string, string) {

	lastSlash := strings.LastIndex(bootstrapFileName, "/")

	if lastSlash == -1 {
		// Assumed to be the current directory
		if abs, err := filepath.Abs(bootstrapFileName); err != nil {
			panic(err)
		} else {
			return filepath.Dir(abs) + "/", bootstrapFileName
		}
	}

	return bootstrapFileName[0 : lastSlash+1], bootstrapFileName[lastSlash+1:]
}
