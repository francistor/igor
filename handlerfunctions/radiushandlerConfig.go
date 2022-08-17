package handlerfunctions

import (
	"fmt"
	"igor/config"
	"igor/radiuscodec"
)

// Represents an entry in a UserFile
type RadiusUserFileEntry struct {
	Key                      string
	CheckItems               map[string]string
	ReplyItems               []radiuscodec.RadiusAVP
	NonOverridableReplyItems []radiuscodec.RadiusAVP
}

// Loads an entry from an existing configuration object
func NewRadiusUserFileEntry(key string, configObjectName string, ci *config.PolicyConfigurationManager) (RadiusUserFileEntry, error) {

	radiusEntry := RadiusUserFileEntry{
		Key:                      key,
		CheckItems:               make(map[string]string),
		ReplyItems:               make([]radiuscodec.RadiusAVP, 0),
		NonOverridableReplyItems: make([]radiuscodec.RadiusAVP, 0),
	}

	// If we pass nil as last parameter, use the default
	var myCi *config.PolicyConfigurationManager
	if ci == nil {
		myCi = config.GetPolicyConfig()
	} else {
		myCi = ci
	}

	/*
		configObject, err := myCi.CM.GetConfigObjectAsJson(configObjectName, false)
		if err != nil {
			return RadiusUserFileEntry{}, err
		}

		objectMap, ok := configObject.(map[string]interface{})
		if !ok {
			return RadiusUserFileEntry{}, fmt.Errorf("%s bad file format", configObjectName)
		}

		keyEntry, found := objectMap[key]
		if !found {
			return RadiusUserFileEntry{}, fmt.Errorf("%s not found in %s", key, configObjectName)
		}
	*/

	keyEntry, err := myCi.CM.GetConfigObjectKeyAsJson(configObjectName, key, false)
	if err != nil {
		return RadiusUserFileEntry{}, err
	}

	keyEntryMap, ok := keyEntry.(map[string]interface{})
	if !ok {
		return RadiusUserFileEntry{}, fmt.Errorf("%s.%s could not be indexed by key", configObjectName, key)
	}

	// Parse checkItems
	if checkItems, found := keyEntryMap["checkItems"]; found {
		checkItemsMap, ok := checkItems.(map[string]interface{})
		if !ok {
			return RadiusUserFileEntry{}, fmt.Errorf("%s.%s.checkItems could not be indexed by key", configObjectName, key)
		}
		for checkItemName, checkItemValue := range checkItemsMap {
			if checkItemValueString, ok := checkItemValue.(string); !ok {
				return RadiusUserFileEntry{}, fmt.Errorf("%s.%s.checkItems found non string ckeckItem value", configObjectName, key)
			} else {
				radiusEntry.CheckItems[checkItemName] = checkItemValueString
			}
		}
	}

	// Parse replyItems
	if replyItems, found := keyEntryMap["replyItems"]; found {
		replyItemsMap, ok := replyItems.([]interface{})
		if !ok {
			return RadiusUserFileEntry{}, fmt.Errorf("%s.%s.replyItems is not an array", configObjectName, key)
		}
		for _, replyItem := range replyItemsMap {
			if replyItemMap, ok := replyItem.(map[string]interface{}); !ok {
				return RadiusUserFileEntry{}, fmt.Errorf("%s.%s.replyItem bad format %#v", configObjectName, key, replyItemMap)
			} else {
				for replyItemName, replyItemValue := range replyItemMap {
					if avp, err := radiuscodec.NewAVP(replyItemName, replyItemValue); err != nil {
						return RadiusUserFileEntry{}, err
					} else {
						radiusEntry.ReplyItems = append(radiusEntry.ReplyItems, *avp)
					}
				}
			}
		}
	}

	return radiusEntry, nil
}
