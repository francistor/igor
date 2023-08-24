package core

import (
	"encoding/json"
	"fmt"
)

type RadiusAVPType int

const (
	RadiusTypeNone        = 0
	RadiusTypeString      = 1
	RadiusTypeOctets      = 2
	RadiusTypeAddress     = 3
	RadiusTypeInteger     = 4
	RadiusTypeTime        = 5
	RadiusTypeIPv6Address = 6
	RadiusTypeIPv6Prefix  = 7
	RadiusTypeInterfaceId = 8
	RadiusTypeInteger64   = 9
)

var UnknownRadiusDictItem = RadiusAVPDictItem{
	Name: "UNKNOWN",
}

// VendorId and code of AVP in a single attribute
type RadiusAVPCode struct {
	VendorId uint32
	Code     byte
}

// Diameter Dictionary elements
type RadiusAVPDictItem struct {
	VendorId   uint32
	Code       byte
	Name       string
	RadiusType RadiusAVPType  // One of the constants above
	EnumValues map[string]int // non nil only in enum type
	EnumCodes  map[int]string // non  nil only in enum type
	Encrypted  bool
	Tagged     bool
	Salted     bool
	WithLen    bool
	Concat     bool
}

// Represents the full Radius Dictionary
type RadiusDict struct {
	// Map of vendor id to vendor name
	VendorById map[uint32]string

	// Map of vendor name to vendor id
	VendorByName map[string]uint32

	// Map of avp code to name. Name is <vendorName>-<attributeName>
	AVPByCode map[RadiusAVPCode]*RadiusAVPDictItem

	// Map of avp name to code
	AVPByName map[string]*RadiusAVPDictItem
}

// Returns an empty dictionary item if the code is not found
// The user may decide to go on with an UNKNOWN dictionary item when the error is returned
func (rd *RadiusDict) GetFromCode(code RadiusAVPCode) (*RadiusAVPDictItem, error) {
	if di, found := rd.AVPByCode[code]; !found {
		return &UnknownRadiusDictItem, fmt.Errorf("%v not found in dictionary", code)
	} else {
		return di, nil
	}
}

// Returns an empty dictionary item if the code is not found
// The user may decide to go on with an UNKNOWN dictionary item when the error is returned
func (rd *RadiusDict) GetFromName(name string) (*RadiusAVPDictItem, error) {
	if di, found := rd.AVPByName[name]; !found {
		return &UnknownRadiusDictItem, fmt.Errorf("%v not found in dictionary", name)
	} else {
		return di, nil
	}
}

func newRadiusDictionaryFromJDict(jDict *jRadiusDict) *RadiusDict {
	// Build the dictionary
	var dict RadiusDict

	// Build the vendor maps
	dict.VendorById = make(map[uint32]string)
	dict.VendorByName = make(map[string]uint32)
	for _, v := range jDict.Vendors {
		dict.VendorById[v.VendorId] = v.VendorName
		dict.VendorByName[v.VendorName] = v.VendorId
	}

	// Build the AVP maps
	dict.AVPByCode = make(map[RadiusAVPCode]*RadiusAVPDictItem)
	dict.AVPByName = make(map[string]*RadiusAVPDictItem)
	for _, vendorAVPs := range jDict.Avps {
		vendorId := vendorAVPs.VendorId
		vendorName := dict.VendorById[vendorId]

		// Map all atttributtes from this vendor
		for _, attr := range vendorAVPs.Attributes {
			avpDictItem := attr.toAVPDictItem(vendorId, vendorName)
			dict.AVPByCode[RadiusAVPCode{vendorId, attr.Code}] = &avpDictItem
			dict.AVPByName[avpDictItem.Name] = &avpDictItem
		}
	}

	return &dict
}

// Returns a Diameter Dictionary object from its serialized representation
func NewRadiusDictionaryFromJSON(data []byte) *RadiusDict {

	// Unmarshall from JSON
	var jDict jRadiusDict
	if err := json.Unmarshal(data, &jDict); err != nil {
		panic("bad radius dictionary format " + err.Error())
	}

	return newRadiusDictionaryFromJDict(&jDict)
}

/*
The following types are helpers for unserializing the JSON Radius Dictionary
*/

// To Unmarshall Dictionary from Json
type jRadiusAVP struct {
	Code       byte
	Name       string
	Type       string
	EnumValues map[string]int
	Encrypted  bool
	Tagged     bool
	Salted     bool
	WithLen    bool
	Concat     bool
}

type jRadiusVendorAVPs struct {
	VendorId   uint32
	Attributes []jRadiusAVP
}

type jVendor struct {
	VendorId   uint32
	VendorName string
}

type jRadiusDict struct {
	Version int
	Vendors []jVendor
	Avps    []jRadiusVendorAVPs
}

// Builds a cooked AVPDictItem from the raw Json representation
func (javp jRadiusAVP) toAVPDictItem(v uint32, vs string) RadiusAVPDictItem {

	// Sanity check
	var radiusType RadiusAVPType
	switch javp.Type {
	case "None":
		radiusType = RadiusTypeNone
	case "String":
		radiusType = RadiusTypeString
	case "Octets":
		radiusType = RadiusTypeOctets
	case "Address":
		radiusType = RadiusTypeAddress
	case "Integer":
		radiusType = RadiusTypeInteger
	case "Time":
		radiusType = RadiusTypeTime
	case "IPv6Address":
		radiusType = RadiusTypeIPv6Address
	case "IPv6Prefix":
		radiusType = RadiusTypeIPv6Prefix
	case "InterfaceId":
		radiusType = RadiusTypeInterfaceId
	case "Integer64":
		radiusType = RadiusTypeInteger64

	default:
		panic(javp.Type + " is not a valid RadiusType")
	}

	// Sanity check
	if javp.Concat && radiusType != RadiusTypeOctets {
		panic(javp.Name + " is concat but not of type Octets")
	}

	// Build the map for enum values
	var codes map[int]string
	if javp.EnumValues != nil {
		codes = make(map[int]string)
		for enumName, enumValue := range javp.EnumValues {
			codes[enumValue] = enumName
		}
	}

	var namePrefix string
	if vs != "" {
		namePrefix = vs + "-"
	}

	return RadiusAVPDictItem{
		VendorId:   v,
		Code:       javp.Code,
		Name:       namePrefix + javp.Name,
		RadiusType: radiusType,
		EnumValues: javp.EnumValues,
		EnumCodes:  codes,
		Encrypted:  javp.Encrypted,
		Tagged:     javp.Tagged,
		Salted:     javp.Salted,
		WithLen:    javp.WithLen,
		Concat:     javp.Concat,
	}
}
