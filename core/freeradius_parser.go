package core

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
)

// Reads a radius dictionary in free radius format and generates a jRadiusDict object, used
// to generate the final radius dictionary to be used in the application
func ParseFreeradiusDictionary(c *ConfigurationManager, configObj string, parentConfigObj string, dict *jRadiusDict) error {

	// Sanity check
	if dict == nil {
		panic("the pointer to the jRadius dictionary was null")
	}

	// If the name of the object is embedded in an $INCLUDE directive, interpret the path as relative
	// to the location of the parent object
	if pos := strings.LastIndex(parentConfigObj, "/"); pos != -1 {
		configObj = parentConfigObj[0:pos] + "/" + configObj
	}

	// Retrieve the config object
	dictBytes, err := c.GetBytesConfigObject(configObj)
	if err != nil {
		return err
	}

	// Initially point to standard attributes
	var currentVendorAVPsIndex int
	if len(dict.Avps) == 0 {
		dict.Avps = append(dict.Avps, jRadiusVendorAVPs{VendorId: 0, Attributes: make([]jRadiusAVP, 0)})
	}

	// Iterate through the dictionary lines
	var scanner = bufio.NewScanner(bytes.NewReader(dictBytes))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}
		cpos := strings.IndexByte(line, '#')
		if cpos >= 0 {
			line = line[:cpos]
		}

		// Parse the line
		words := strings.Fields(line)
		if len(words) < 1 {
			continue
		}

		switch words[0] {
		case "$INCLUDE":
			err := ParseFreeradiusDictionary(c, words[1], configObj, dict)
			if err != nil {
				return errors.New("dictionary " + words[1] + " with error " + err.Error())
			}

		case "VENDOR":
			vendorId, err := strconv.Atoi(words[2])
			if err != nil {
				return errors.New("invalid VENDOR " + line)
			}

			// Insert into vendors field
			dict.Vendors = append(dict.Vendors, jVendor{
				VendorId:   uint32(vendorId),
				VendorName: words[1],
			})

			// Initialize avps slice item for vendor
			dict.Avps = append(dict.Avps,
				jRadiusVendorAVPs{
					VendorId:   uint32(vendorId),
					Attributes: make([]jRadiusAVP, 0),
				})

		case "BEGIN-VENDOR":
			// The vendor must have been defined previously

			// Look for vendor id
			var vendorId uint32 = 0
			for i := range dict.Vendors {
				if dict.Vendors[i].VendorName == words[1] {
					vendorId = dict.Vendors[i].VendorId
					break
				}
			}

			if vendorId == 0 {
				return errors.New("vendor " + words[1] + " not found")
			} else {
				// Get the index for that vendorId
				for i := range dict.Avps {
					if dict.Avps[i].VendorId == vendorId {
						currentVendorAVPsIndex = i
						break
					}
				}
			}

		case "END-VENDOR":
			// Reset to default attributes
			currentVendorAVPsIndex = 0

		case "ATTRIBUTE":
			if len(words) < 4 {
				return errors.New("invalid ATTRIBUTE " + line)
			}
			// TODO: Ignoring codes starting with a dot. They are "tlv" type
			code, err := strconv.Atoi(words[2])
			if err != nil {
				break
			}

			// Options: comma separated value
			// We only support the has_tag and encrypt attributes
			// <type>,has_tag,encrypt=[1,2,3]
			radiusType := parseRadiusType(words[3])
			tagged := false
			encrypted := false
			salted := false
			withLen := false
			concat := false
			if len(words) > 4 {
				options := strings.Split(words[4], ",")
				for _, option := range options {
					if option == "has_tag" {
						tagged = true
					} else if option == "encrypt=1" {
						encrypted = true
					} else if option == "encrypt=2" {
						salted = true
						withLen = true
					} else if option == "encrypt=3" {
						// Ascend propietary. Ignore
						radiusType = "Octets"
					} else if option == "encrypt=8" {
						// This one does not exist in freeradius
						tagged = true
						salted = true
					} else if option == "encrypt=9" {
						// This one does not exist in freeradius
						salted = true
					} else if option == "concat" {
						concat = true
					} else if option == "array" {
						radiusType = "Octets"
					} else if option == "abinary" || option == "extended" || option == "long-extended" {
						// Ignore this ones
					} else {
						return errors.New("invalid ATTRIBUTE " + line)
					}
				}
			}

			if radiusType != "VSA" {
				avp := jRadiusAVP{
					Code:      byte(code),
					Name:      words[1],
					Type:      radiusType,
					Tagged:    tagged,
					Encrypted: encrypted,
					Salted:    salted,
					WithLen:   withLen,
					Concat:    concat,
				}
				dict.Avps[currentVendorAVPsIndex].Attributes = append(dict.Avps[currentVendorAVPsIndex].Attributes, avp)
			}

		case "VALUE":
			if len(words) < 4 {
				return errors.New("invalid VALUE " + line)
			}
			val, err := strconv.Atoi(words[3])
			if err != nil {
				// Try in hexa
				if val64, err := strconv.ParseInt(strings.TrimPrefix(words[3], "0x"), 16, 32); err != nil {
					return errors.New("invalid VALUE " + line)
				} else {
					val = int(val64)
				}
			}

			// Look for the attribute name
			for i, attr := range dict.Avps[currentVendorAVPsIndex].Attributes {
				if attr.Name == words[1] {

					// Initialize if necessary
					if attr.EnumValues == nil {
						dict.Avps[currentVendorAVPsIndex].Attributes[i].EnumValues = make(map[string]int)
					}

					// Add item
					dict.Avps[currentVendorAVPsIndex].Attributes[i].EnumValues[words[2]] = val
					break
				}
			}
		}
	}

	return nil
}

// TODO: Parse tlv as proper attribute
func parseRadiusType(t string) string {
	switch t {
	case "integer", "uint32", "byte", "short", "signed", "time_delta":
		return "Integer"
	case "string", "ipv4prefix":
		return "String"
	case "octets", "abinary", "struct", "tlv", "combo-ip", "ether":
		return "Octets"
	case "ipaddr":
		return "Address"
	case "date":
		return "Time"
	case "ipv6addr":
		return "IPv6Address"
	case "ipv6prefix":
		return "IPv6Prefix"
	case "ifid":
		return "InterfaceId"
	case "integer64":
		// Does not exist in freeradius
		return "Integer64"
	case "vsa":
		return "VSA"
	default:
		// Exceptions
		if strings.HasPrefix(t, "octets") {
			// Freeradius uses sometimes octets[size]
			return "Octets"
		}
		if strings.HasPrefix(t, "string") {
			// Freeradius uses sometimes string[size]
			return "String"
		}

		panic("unrecognized attribute type " + t)
	}
}
