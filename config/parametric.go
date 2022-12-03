package config

import (
	"encoding/json"
	"strings"
	"text/template"
)

// Builds a JSON object which has as properties the keys of the parameters map, and as
// values the result of applying the object to the template passed as parameter
// T is the type of the object passed as parameter to the templates
func GetBytesTemplatedConfigObject[T any](templateObjectName string, parametersObjectName string, ci *PolicyConfigurationManager) ([]byte, error) {

	// If we pass nil as last parameter, use the default
	var myCi *PolicyConfigurationManager
	if ci == nil {
		myCi = GetPolicyConfig()
	} else {
		myCi = ci
	}

	// Retrieve the template
	tmplBytes, err := myCi.CM.GetBytesConfigObject(templateObjectName)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("igor_template").Parse(string(tmplBytes))
	if err != nil {
		return nil, err
	}

	// Retrieve the map of template parameter objects
	var parametersSet map[string]T
	err = myCi.CM.BuildJSONConfigObject(parametersObjectName, &parametersSet)
	if err != nil {
		return nil, err
	}

	// This object will hold the ouptut
	out := make(map[string]*json.RawMessage)

	// Apply the template to each key of the parameters
	for k, p := range parametersSet {
		var tmplRes strings.Builder
		if err := tmpl.Execute(&tmplRes, p); err != nil {
			return nil, err
		}
		var a json.RawMessage = []byte(tmplRes.String())
		out[k] = &a
	}

	return json.Marshal(out)
}
