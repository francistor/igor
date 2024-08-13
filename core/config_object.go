package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// If the object implements this interface, this method will be executed after each
// update, typically for cooking the derived attributes
type Initializable interface {
	initialize() error
}

// Represents an object that will be populated from the configuration resources
type ConfigObject[T any] struct {
	o          *T
	objectName string
}

// Creates an uninitialized configuration object
func NewConfigObject[T any](name string) *ConfigObject[T] {
	var co ConfigObject[T]
	co.objectName = name
	return &co
}

// Reads the configuration from the associated resource and initializes it
// if an initialize() method is defined
func (co *ConfigObject[T]) Update(cm *ConfigurationManager) error {

	var theObject T
	if err := cm.BuildObjectFromJsonConfig(co.objectName, &theObject); err != nil {
		return err
	} else {
		// Passing &theObject so that both pointer and value initializers are executed
		if initializable, ok := any(&theObject).(Initializable); ok {
			if err := initializable.initialize(); err != nil {
				return err
			}
		}
		co.o = &theObject
		return nil
	}
}

// Provides access to the configuration object. Returns a copy, so the underlying
// object may be modified safely
func (co *ConfigObject[T]) Get() T {
	return *co.o
}

///////////////////////////////////////////////////////////////////////////////

// Represents an object that will be populated from two configuration resources.
// Those configuration resources are a template and a set of parameters for that template.
// T is the type of the template object, P is the type of the parameter object
// The parametersObject is a map of strings to P.
// For instance, T may be a RadiusUserFile, and P a struct holding parameters such as speed and timeouts
// The end result will be a map of the keys in the ParametersObject to RadiusUserFile built from
// the specified template and parameters replaced.
type TemplatedMapConfigObject[T, P any] struct {
	o                    *map[string]T
	templateObjectName   string
	parametersObjectName string
}

// Creates an uninitialized templated configuration object
func NewTemplatedMapConfigObject[T, P any](templateObjectName string, parametersObjectName string) *TemplatedMapConfigObject[T, P] {
	var tco TemplatedMapConfigObject[T, P]
	tco.templateObjectName = templateObjectName
	tco.parametersObjectName = parametersObjectName
	return &tco
}

// Reads the configuration from the associated resource and initializes it,
// if an initialize() method is defined
func (tco *TemplatedMapConfigObject[T, P]) Update(cm *ConfigurationManager) error {

	// Retrieve and build the template
	tmplBytes, err := cm.GetRawBytesConfigObject(tco.templateObjectName)
	if err != nil {
		return err
	}
	tmpl, err := template.New("igor_template").Parse(string(tmplBytes))
	if err != nil {
		return err
	}

	// Retrieve the map of template parameter objects
	var parametersSet map[string]P
	err = cm.BuildObjectFromJsonConfig(tco.parametersObjectName, &parametersSet)
	if err != nil {
		return err
	}

	// This object will hold the ouptut temporarily
	var theMap = make(map[string]T)

	// Apply the template to each key of the parameters
	for k, p := range parametersSet {
		var tmplRes strings.Builder
		if err := tmpl.Execute(&tmplRes, p); err != nil {
			return err
		}
		var v T
		if err := json.Unmarshal([]byte(tmplRes.String()), &v); err != nil {
			return err
		}
		theMap[k] = v
	}

	tco.o = &theMap

	return nil
}

// Provides access to the configuration object.
func (tco *TemplatedMapConfigObject[T, P]) Get() map[string]T {
	return *tco.o
}

// Provides access to the specified entry of the configuration object.
func (tco *TemplatedMapConfigObject[T, P]) GetKey(key string) (T, error) {
	var theMap = *tco.o
	if co, found := theMap[key]; !found {
		return co, fmt.Errorf("key %s not found", key)
	} else {
		return co, nil
	}
}
