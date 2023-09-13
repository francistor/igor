package core

// These global variables have to be initialized using the corresponding function below
var diameterDict *DiameterDict
var radiusDict *RadiusDict

// Loads the Diameter dictionary
func initDiameterDict(cm *ConfigurationManager) {

	// Load dictionaries

	// Diameter
	coreJSON, err := cm.GetBytesConfigObject("diameterDictionary.json")
	if err != nil {
		panic("Could not read diameterDictionary.json")
	}
	diameterDict = NewDiameterDictionaryFromJSON([]byte(coreJSON))
}

// Loads the Radius dictionary
func initRadiusDict(cm *ConfigurationManager) {

	// Radius
	// First try freeradius dictionary
	var jDict jRadiusDict

	if err := ParseFreeradiusDictionary(cm, "dictionary", "", &jDict); err == nil {
		// Try first with radius dictionary
		radiusDict = newRadiusDictionaryFromJDict(&jDict)
		GetLogger().Info("freeradius dictionary found")
	} else {
		GetLogger().Info("freeradius dictionary not found")
		if radiusDictJSON, err := cm.GetBytesConfigObject("radiusDictionary.json"); err == nil {
			// Otherwise, build from native format
			radiusDict = NewRadiusDictionaryFromJSON([]byte(radiusDictJSON))
		} else {
			panic("Could not read radiusDictionary.json or dictionary (freeradius format)")
		}
	}
}

// Used globally to get access to the diameter dictionary
func GetDDict() *DiameterDict {
	if diameterDict == nil {
		panic("uninitialized radius dictionary. Use initDiameterDict first")
	}
	return diameterDict
}

func GetRDict() *RadiusDict {
	if radiusDict == nil {
		panic("uninitialized radius dictionary. Use initRadiusDict first")
	}
	return radiusDict
}
