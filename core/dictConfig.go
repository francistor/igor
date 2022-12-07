package core

// These global variables have to be initialized using initDictionaries
var diameterDict *DiameterDict
var radiusDict *RadiusDict

// Loads the Radius and Diameter dictionaries
func initDictionaries(cm *ConfigurationManager) {

	// Load dictionaries

	// Diameter
	coreJSON, err := cm.GetBytesConfigObject("diameterDictionary.json")
	if err != nil {
		panic("Could not read diameterDictionary.json")
	}
	diameterDict = NewDiameterDictionaryFromJSON([]byte(coreJSON))

	// Radius
	radiusDictJSON, err := cm.GetBytesConfigObject("radiusDictionary.json")
	if err != nil {
		panic("Could not read radiusDictionary.json")
	}

	radiusDict = NewRadiusDictionaryFromJSON([]byte(radiusDictJSON))
}

// Used globally to get access to the diameter dictionary
func GetDDict() *DiameterDict {
	if diameterDict == nil {
		panic("uninitialized radius dictionary. Use initDictionaries first")
	}
	return diameterDict
}

func GetRDict() *RadiusDict {
	if diameterDict == nil {
		panic("uninitialized radius dictionary. Use initDictionaries first")
	}
	return radiusDict
}
