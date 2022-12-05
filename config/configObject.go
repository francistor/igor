package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// If the object implements this interface, this method will be executed after each
// updatet, typically for cooking the derived attributes
type Initializable interface {
	initialize() error
}

// Represents an object that will be populated from the configuration resources
type ConfigObject[T any] struct {
	o          T
	objectName string
}

// Creates an uninitialized configuration object
func NewConfigObject[T any](name string) *ConfigObject[T] {
	var co ConfigObject[T]
	co.objectName = name
	return &co
}

// Reads the configuration from the associated resource and initializes it,
// if an initialize() method is defined
func (co *ConfigObject[T]) Update(cm *ConfigurationManager) error {

	var theObject T
	if err := cm.BuildJSONConfigObject(co.objectName, &theObject); err != nil {
		return err
	} else {
		if initializable, ok := any(theObject).(Initializable); ok {
			if err := initializable.initialize(); err != nil {
				return err
			}
		}
		co.o = theObject
		return nil
	}
}

// Provides access to the configuration object. Returns a copy, so the underlying
// object may be modified safely
func (co *ConfigObject[T]) Get() T {
	return co.o
}

///////////////////////////////////////////////////////////////////////////////

// Represents an object that will be populated from the configuration resources
// Those configuration resources are a template and a set of parameters for that template
// T is the type of the template object, P is the type of the parameter object
type TemplatedConfigObject[T, P any] struct {
	o                    map[string]T
	templateObjectName   string
	parametersObjectName string
}

// Creates an uninitialized templated configuration object
func NewTemplatedConfigObject[T, P any](templateObjectName string, parametersObjectName string) *TemplatedConfigObject[T, P] {
	var tco TemplatedConfigObject[T, P]
	tco.templateObjectName = templateObjectName
	tco.parametersObjectName = parametersObjectName
	return &tco
}

// Reads the configuration from the associated resource and initializes it,
// if an initialize() method is defined
func (tco *TemplatedConfigObject[T, P]) Update(cm *ConfigurationManager) error {

	// Retrieve the template
	tmplBytes, err := cm.GetBytesConfigObject(tco.templateObjectName)
	if err != nil {
		return err
	}
	tmpl, err := template.New("igor_template").Parse(string(tmplBytes))
	if err != nil {
		return err
	}

	// Retrieve the map of template parameter objects
	var parametersSet map[string]P
	err = cm.BuildJSONConfigObject(tco.parametersObjectName, &parametersSet)
	if err != nil {
		return err
	}

	// This object will hold the ouptut temporarily
	tco.o = make(map[string]T)

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
		tco.o[k] = v
	}

	return nil
}

// Provides access to the configuration object.
func (tco *TemplatedConfigObject[T, P]) Get() map[string]T {
	return tco.o
}

// Provides access to the configuration object.
func (tco *TemplatedConfigObject[T, P]) GetKey(key string) (T, error) {
	if co, found := tco.o[key]; !found {
		return co, fmt.Errorf("key %s not found", key)
	} else {
		return co, nil
	}

}
