package handlerfunctions

import (
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/radiuscodec"
	"regexp"
	"strings"
)

/////////////////////////////////////////////////////////////////////////////
// User File read helpers
/////////////////////////////////////////////////////////////////////////////

// Represents an entry in a UserFile
type RadiusUserFileEntry struct {
	Key                      string
	CheckItems               map[string]string
	ReplyItems               []radiuscodec.RadiusAVP
	NonOverridableReplyItems []radiuscodec.RadiusAVP
	OOBReplyItems            []radiuscodec.RadiusAVP
}

type RadiusUserFile map[string]RadiusUserFileEntry

// Parses a Radius Userfile
// Entries are of the form
// key:
//
//	checkItems: {attr: value, attr:value}
//	replyItems: [<AVP>],
//	nonOverridableReplyItems: [<AVP>] -- typically for Cisco-AVPair
//	oobReplyItems: [<AVP>]			   -- Service definition queries from BNG
func NewRadiusUserFile(configObjectName string, ci *config.PolicyConfigurationManager) (RadiusUserFile, error) {
	// If we pass nil as last parameter, use the default
	var myCi *config.PolicyConfigurationManager
	if ci == nil {
		myCi = config.GetPolicyConfig()
	} else {
		myCi = ci
	}

	jBytes, err := myCi.CM.GetConfigObjectAsText(configObjectName, false)
	if err != nil {
		return RadiusUserFile{}, err
	}

	ruf := RadiusUserFile{}
	err = json.Unmarshal(jBytes, &ruf)
	fmt.Println(ruf)
	return ruf, err
}

/////////////////////////////////////////////////////////////////////////////
// Radius Packet Checks. Used in order to filter packets, depending on the
// content of the AVPs
/////////////////////////////////////////////////////////////////////////////

type RadiusPacketCheck struct {
	Operation string // "and", "or" or ""
	Branch    []RadiusPacketCheck
	Leaf      []string // Three valued or two valued
}

// The contents of all the checks defined in the corresponding configuration object
type RadiusPacketChecks map[string]RadiusPacketCheck

// Check whether the radius packet is conformant to the RadiusAVPCheck specification
func (cs RadiusPacketChecks) CheckPacket(key string, packet *radiuscodec.RadiusPacket) bool {
	if check, ok := cs[key]; !ok {
		return false
	} else {
		return check.CheckPacket(packet)
	}
}

// Check whether the radius packet is conformant to the RadiusAVPCheck specification
func (c *RadiusPacketCheck) CheckPacket(packet *radiuscodec.RadiusPacket) bool {
	if len(c.Leaf) > 0 {
		// Check is a Leaf
		attributeName := c.Leaf[0]
		attributeValue := packet.GetStringAVP(attributeName)
		condition := c.Leaf[1]
		switch condition {
		case "equals":
			return attributeValue == c.Leaf[2]
		case "present":
			return attributeValue != ""
		case "contains":
			return strings.Contains(attributeValue, c.Leaf[2])
		case "matches":
			if isMatch, err := regexp.MatchString(c.Leaf[2], attributeValue); err != nil {
				return false
			} else {
				return isMatch
			}
		default:
			panic("unknown check condition: " + condition)
		}
	}

	// Check recursively
	switch c.Operation {
	case "and":
		for _, check := range c.Branch {
			if !check.CheckPacket(packet) {
				return false
			}
		}
		return true
	case "or":
		for _, check := range c.Branch {
			if check.CheckPacket(packet) {
				return true
			}
		}
		return false
	default:
		panic("bad operation: " + c.Operation)
	}
}

// Loads the Radius Checks object
// A Check object entry has the form
// "condition":[
//
//	<check objects>
//
// ]
// or
// ["attribute", "check-type", "value to check (optional)"]
//
// that is, it can be a two or three element array, or an object containing a condition and nested check objects
func NewRadiusChecks(configObjectName string, ci *config.PolicyConfigurationManager) (RadiusPacketChecks, error) {

	checks := make(RadiusPacketChecks)

	// If we pass nil as last parameter, use the default
	var myCi *config.PolicyConfigurationManager
	if ci == nil {
		myCi = config.GetPolicyConfig()
	} else {
		myCi = ci
	}

	// Read the configuration object
	c, err := myCi.CM.GetConfigObjectAsJson(configObjectName, false)
	if err != nil {
		return checks, err
	}

	// Parse recursively
	entries, ok := c.(map[string]interface{})
	if !ok {
		return checks, err
	}
	for key, entry := range entries {
		if parsedEntry, err := parseRadiusCheck(entry); err != nil {
			return checks, err
		} else {
			checks[key] = parsedEntry
		}

	}

	return checks, nil
}

// Parses the, possibly nested, check specification
func parseRadiusCheck(radiusCheck interface{}) (RadiusPacketCheck, error) {

	// May be an object that contains an operation and a Branch, or a Leaf, which is a single condition
	// A Branch is an object with a single property ('and' or 'or') and a value which is an array of
	// nested objects of the same type (Branch or Leaf)

	// Check if this is a Leaf. I cannot cast to []string yet
	if array, ok := radiusCheck.([]interface{}); ok {
		// Converion to []string
		arrayItems := make([]string, 0)
		for _, arrayItem := range array {
			arrayItems = append(arrayItems, arrayItem.(string))
		}
		// Sanity Check
		if len(arrayItems) < 2 {
			return RadiusPacketCheck{}, fmt.Errorf("bad format specification %v", arrayItems)
		} else {
			if arrayItems[1] != "equals" &&
				arrayItems[1] != "matches" &&
				arrayItems[1] != "contains" &&
				arrayItems[1] != "present" {
				return RadiusPacketCheck{}, fmt.Errorf("bad format specification %v", arrayItems)
			}
			if arrayItems[1] != "present" && len(arrayItems) < 3 {
				return RadiusPacketCheck{}, fmt.Errorf("bad format specification %v", arrayItems)
			}
		}
		return RadiusPacketCheck{Leaf: arrayItems}, nil
	}

	// Otherwise, this is a Branch
	if object, ok := radiusCheck.(map[string]interface{}); ok {
		// There will be only one operation. The following range will be looped through only once
		for operation := range object {
			if operation != "and" && operation != "or" {
				return RadiusPacketCheck{}, fmt.Errorf("operation was not 'and' or 'or'")
			}

			// Placeholder for the elements of the Branch
			branch := make([]RadiusPacketCheck, 0)

			// The value is an array of nested items
			if leafs, ok := object[operation].([]interface{}); !ok {
				return RadiusPacketCheck{}, fmt.Errorf("branch was not an array")
			} else {
				// Parse all the nested items
				for _, leaf := range leafs {
					if check, err := parseRadiusCheck(leaf); err != nil {
						return RadiusPacketCheck{}, err
					} else {
						branch = append(branch, check)
					}
				}
			}

			return RadiusPacketCheck{
				Operation: operation,
				Branch:    branch,
			}, nil
		}
		return RadiusPacketCheck{}, fmt.Errorf("branch is a map with no items")
	} else {
		// If here, something went wrong
		return RadiusPacketCheck{}, fmt.Errorf("type if not recognized %#v", radiusCheck)
	}
}

/////////////////////////////////////////////////////////////////////////////
// Radius Packet Attribute Filter
/////////////////////////////////////////////////////////////////////////////

// Entry in AVPFilter file
type AVPFilter struct {
	Allow  []string
	Remove []string
	Force  [][]string
}

// Contents of the AVPFilters file. Set of AVPFilters by key
type AVPFilters map[string]AVPFilter

// Creates a copy of the radius packet with the attributes filtered as specified in the filter for the passed key
func (fs AVPFilters) FilterPacket(key string, packet *radiuscodec.RadiusPacket) (*radiuscodec.RadiusPacket, error) {
	if filter, ok := fs[key]; !ok {
		return &radiuscodec.RadiusPacket{}, fmt.Errorf("%s filter not found", key)
	} else {
		return filter.FilterPacket(packet), nil
	}
}

// Copy the radius packet with the attributes modified as defined in the specified filter
func (f *AVPFilter) FilterPacket(packet *radiuscodec.RadiusPacket) *radiuscodec.RadiusPacket {
	fmt.Printf("%#v\n", f)
	var rp *radiuscodec.RadiusPacket
	if len(f.Allow) > 0 {
		rp = packet.Copy(f.Allow, nil)
	} else if len(f.Remove) > 0 {
		rp = packet.Copy(f.Allow, nil)
	} else {
		rp = packet.Copy(nil, nil)
	}

	for _, forceSpec := range f.Force {
		if len(forceSpec) == 2 {
			rp.DeleteAllAVP(forceSpec[0])
			rp.Add(forceSpec[0], forceSpec[1])
		}
	}

	return rp
}

// Returns an object representing the configured AVPFilters
func NewAVPFilters(configObjectName string, ci *config.PolicyConfigurationManager) (AVPFilters, error) {

	filters := make(AVPFilters)

	// If we pass nil as last parameter, use the default
	var myCi *config.PolicyConfigurationManager
	if ci == nil {
		myCi = config.GetPolicyConfig()
	} else {
		myCi = ci
	}

	// Read the configuration object
	fs, err := myCi.CM.GetConfigObjectAsText(configObjectName, false)
	if err != nil {
		return filters, err
	}

	if err = json.Unmarshal(fs, &filters); err != nil {
		return filters, fmt.Errorf(err.Error())
	}

	return filters, nil
}
