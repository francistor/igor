package handler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/francistor/igor/config"
	"github.com/francistor/igor/radiuscodec"
)

/////////////////////////////////////////////////////////////////////////////
// Radius Packet Checks. Used in order to filter packets, depending on the
// content of the AVPs
/////////////////////////////////////////////////////////////////////////////

// Specification of the rules to check packets
// Posible conditions are
// * <attribute|"code"> equals <value>
// * <attribute> notpresent
// * <attribute> present
// * <attribute> contains <string>
// * <attribute> matches <regex>
type RadiusPacketCheck struct {
	// "and", "or" (if has branches) or "" (if is a leaf)
	Operation string

	// May contain either a Branch (if composite) or a Leaf (single check)

	// Subsidiary checks
	Branches []RadiusPacketCheck

	// Simple check, that is, without nested checks.
	// May be three valued or two valued
	Leaf []string

	// Compiled regular expression, in case the condition uses that
	regex *regexp.Regexp
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
		var value string
		name := c.Leaf[0]
		if name == "code" {
			value = strconv.Itoa(int(packet.Code))
		} else {
			value = packet.GetStringAVP(name)
		}

		condition := c.Leaf[1]
		switch condition {
		case "equals":
			return value == c.Leaf[2]
		case "present":
			return value != ""
		case "notpresent":
			return value == ""
		case "contains":
			return strings.Contains(value, c.Leaf[2])
		case "matches":
			return c.regex.MatchString(value)
		default:
			panic("unknown check condition: " + condition)
		}
	}

	// Otherwise, it is a set of branches: check recursively
	switch c.Operation {
	case "and":
		for _, check := range c.Branches {
			if !check.CheckPacket(packet) {
				return false
			}
		}
		return true
	case "or":
		for _, check := range c.Branches {
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
func NewRadiusPacketChecks(configObjectName string, ci *config.PolicyConfigurationManager) (RadiusPacketChecks, error) {

	checks := make(RadiusPacketChecks)

	// If we pass nil as last parameter, use the default configuration manager
	var myCi *config.PolicyConfigurationManager
	if ci == nil {
		myCi = config.GetPolicyConfig()
	} else {
		myCi = ci
	}

	// Read the configuration object
	var entries map[string]interface{}
	err := myCi.CM.BuildJSONConfigObject(configObjectName, &entries)
	if err != nil {
		return checks, err
	}

	// Parse recursively
	for key, entry := range entries {
		if parsedEntry, err := parseRadiusPacketCheck(entry); err != nil {
			return checks, err
		} else {
			checks[key] = parsedEntry
		}

	}

	return checks, nil
}

// Parses the, possibly nested, check specification
func parseRadiusPacketCheck(radiusCheck interface{}) (RadiusPacketCheck, error) {

	// May be an object that contains an operation and a Branch, or a Leaf, which is a single condition
	// A Branch is an object with a single property ('and' or 'or') and a value which is an array of
	// nested objects of the same type (Branch or Leaf)

	// Check if this is a Leaf. I cannot cast to []string yet
	if array, ok := radiusCheck.([]interface{}); ok {
		// Conversion to []string
		arrayItems := make([]string, 0)
		for _, arrayItem := range array {
			arrayItems = append(arrayItems, arrayItem.(string))
		}
		// Sanity Check
		if len(arrayItems) < 2 {
			return RadiusPacketCheck{}, fmt.Errorf("bad format specification. Missing at least one item: %v", arrayItems)
		} else {
			if arrayItems[1] != "equals" &&
				arrayItems[1] != "matches" &&
				arrayItems[1] != "contains" &&
				arrayItems[1] != "present" &&
				arrayItems[1] != "notpresent" {
				return RadiusPacketCheck{}, fmt.Errorf("bad format specification. Unknown check type: %v", arrayItems)
			}
			if arrayItems[1] != "present" && arrayItems[1] != "notpresent" && len(arrayItems) < 3 {
				return RadiusPacketCheck{}, fmt.Errorf("bad format specification. Missing at least one item: %v", arrayItems)
			}
		}
		// Compile regex if necessary
		if arrayItems[1] == "matches" {
			return RadiusPacketCheck{Leaf: arrayItems, regex: regexp.MustCompile(arrayItems[2])}, nil
		} else {
			return RadiusPacketCheck{Leaf: arrayItems}, nil
		}
	}

	// Otherwise, this is a Branch
	if object, ok := radiusCheck.(map[string]interface{}); ok {
		// There will be only one operation. The following range will be looped through only once
		for operation := range object {
			if operation != "and" && operation != "or" {
				return RadiusPacketCheck{}, fmt.Errorf("operation was not 'and' or 'or'")
			}

			// Placeholder for the elements of the Branch
			branches := make([]RadiusPacketCheck, 0)

			// The value is an array of nested items
			if leafs, ok := object[operation].([]interface{}); !ok {
				return RadiusPacketCheck{}, fmt.Errorf("branch was not an array")
			} else {
				// Parse all the nested items
				for _, leaf := range leafs {
					if check, err := parseRadiusPacketCheck(leaf); err != nil {
						return RadiusPacketCheck{}, err
					} else {
						branches = append(branches, check)
					}
				}
			}

			return RadiusPacketCheck{
				Operation: operation,
				Branches:  branches,
			}, nil
		}
		return RadiusPacketCheck{}, fmt.Errorf("branch is a map with no items")
	} else {
		// If here, something went wrong
		return RadiusPacketCheck{}, fmt.Errorf("type if not recognized %#v", radiusCheck)
	}
}
