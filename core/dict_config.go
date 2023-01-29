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
	// First try freeradius dictionary
	var jDict jRadiusDict
	err = ParseFreeradiusDictionary(cm, "dictionary", &jDict)
	if err != nil {
		// If not found, try
		radiusDictJSON, err := cm.GetBytesConfigObject("radiusDictionary.json")
		if err != nil {
			panic("Could not read radiusDictionary.json or dictionary (freeradius format)")
		}

		radiusDict = NewRadiusDictionaryFromJSON([]byte(radiusDictJSON))
	} else {
		radiusDict = newRadiusDictionaryFromJDict(&jDict)
	}
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
