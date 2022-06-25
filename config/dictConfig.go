package config

import (
	"igor/diamdict"
)

// This global variable has to be initialized using SetupDictionaries
var diameterDict *diamdict.DiameterDict

// Loads the Radius and Diameter dictionaries
func initDictionaries(cm *ConfigurationManager) {

	// Load dictionaries
	diamDictJSON, err := cm.GetConfigObjectAsText("diameterDictionary.json")
	if err != nil {
		panic("Could not read diameterDictionary.json")
	}

	diameterDict = diamdict.NewDictionaryFromJSON([]byte(diamDictJSON))
}

// Used globally to get access to the diameter dictionary
func GetDDict() *diamdict.DiameterDict {
	if diameterDict == nil {
		panic("uninitialized diamter dictionary. Use initDictionaries first")
	}
	return diameterDict
}
