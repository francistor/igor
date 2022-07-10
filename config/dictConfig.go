package config

import (
	"igor/diamdict"
	"igor/radiusdict"
)

// This global variable has to be initialized using SetupDictionaries
var diameterDict *diamdict.DiameterDict
var radiusDict *radiusdict.RadiusDict

// Loads the Radius and Diameter dictionaries
func initDictionaries(cm *ConfigurationManager) {

	// Load dictionaries

	// Diameter
	diamDictJSON, err := cm.GetConfigObjectAsText("diameterDictionary.json", false)
	if err != nil {
		panic("Could not read diameterDictionary.json")
	}
	diameterDict = diamdict.NewDictionaryFromJSON([]byte(diamDictJSON))

	// Radius
	radiusDictJSON, err := cm.GetConfigObjectAsText("radiusDictionary.json", false)
	if err != nil {
		panic("Could not read radiusDictionary.json")
	}

	radiusDict = radiusdict.NewDictionaryFromJSON([]byte(radiusDictJSON))
}

// Used globally to get access to the diameter dictionary
func GetDDict() *diamdict.DiameterDict {
	if diameterDict == nil {
		panic("uninitialized radius dictionary. Use initDictionaries first")
	}
	return diameterDict
}

func GetRDict() *radiusdict.RadiusDict {
	if diameterDict == nil {
		panic("uninitialized radius dictionary. Use initDictionaries first")
	}
	return radiusDict
}
