package radiusdict

import (
	"encoding/json"
	"fmt"
)

const (
	None        = 0
	String      = 1
	Octets      = 2
	Address     = 3
	Integer     = 4
	Time        = 5
	IPv6Address = 6
	IPv6Prefix  = 7
	InterfaceId = 8
	Integer64   = 9
)

// VendorId and code of AVP in a single attribute
type AVPCode struct {
	VendorId uint32
	Code     uint32
}

// Diameter Dictionary elements
type AVPDictItem struct {
	VendorId   uint32
	Code       uint32
	Name       string
	RadiusType int            // One of the constants above
	EnumValues map[string]int // non nil only in enum type
	EnumCodes  map[int]string // non  nil only in enum type
}

// Represents the full Radius Dictionary
type RadiusDict struct {
	// Map of vendor id to vendor name
	VendorById map[uint32]string

	// Map of vendor name to vendor id
	VendorByName map[string]uint32

	// Map of avp code to name. Name is <vendorName>-<attributeName>
	AVPByCode map[AVPCode]AVPDictItem

	// Map of avp name to code
	AVPByName map[string]AVPDictItem
}

// Returns an empty dictionary item if the code is not found
// The user may decide to go on with an UNKNOWN dictionary item when the error is returned
func (rd *RadiusDict) GetFromCode(code AVPCode) (AVPDictItem, error) {
	di := rd.AVPByCode[code]
	if di.Name == "" {
		di.Name = "UNKNOWN"
		return di, fmt.Errorf("%v not found in dictionary", code)
	}
	return di, nil
}

// Returns an empty dictionary item if the code is not found
// The user may decide to go on with an UNKNOWN dictionary item when the error is returned
func (rd *RadiusDict) GetFromName(name string) (AVPDictItem, error) {
	di, ok := rd.AVPByName[name]
	if !ok {
		di.Name = "UNKNOWN"
		return di, fmt.Errorf("%s not found in dictionary", name)
	}
	return di, nil
}

// Returns a Diameter Dictionary object from its serialized representation
func NewDictionaryFromJSON(data []byte) *RadiusDict {

	// Unmarshall from JSON
	var jDict jRadiusDict
	if err := json.Unmarshal(data, &jDict); err != nil {
		panic("bad radius dictionary format " + err.Error())
	}

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
	dict.AVPByCode = make(map[AVPCode]AVPDictItem)
	dict.AVPByName = make(map[string]AVPDictItem)
	for _, vendorAVPs := range jDict.Avps {
		vendorId := vendorAVPs.VendorId
		vendorName := dict.VendorById[vendorId]

		// For a specific vendor
		for _, attr := range vendorAVPs.Attributes {
			avpDictItem := attr.toAVPDictItem(vendorId, vendorName)
			dict.AVPByCode[AVPCode{vendorId, attr.Code}] = avpDictItem
			dict.AVPByName[avpDictItem.Name] = avpDictItem
		}
	}

	return &dict
}

/*
The following types are helpers for unserializing the JSON Radius Dictionary
*/

// To Unmarshall Dictionary from Json
type jRadiusAVP struct {
	Code       uint32
	Name       string
	Type       string
	EnumValues map[string]int
	Encrypt    bool
	Tagged     bool
}

type jRadiusVendorAVPs struct {
	VendorId   uint32
	Attributes []jRadiusAVP
}

type jRadiusDict struct {
	Version int
	Vendors []struct {
		VendorId   uint32
		VendorName string
	}
	Avps []jRadiusVendorAVPs
}

// Builds a cooked AVPDictItem from the raw Json representation
func (javp jRadiusAVP) toAVPDictItem(v uint32, vs string) AVPDictItem {
	var radiusType int
	switch javp.Type {
	case "None":
		radiusType = None
	case "String":
		radiusType = String
	case "Octets":
		radiusType = Octets
	case "Address":
		radiusType = Address
	case "Integer":
		radiusType = Integer
	case "Time":
		radiusType = Time
	case "IPv6Address":
		radiusType = IPv6Address
	case "IPv6Prefix":
		radiusType = IPv6Prefix
	case "InterfaceId":
		radiusType = InterfaceId
	case "Integer64":
		radiusType = Integer64

	default:
		panic(javp.Type + " is not a valid RadiusType")
	}

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

	return AVPDictItem{
		VendorId:   v,
		Code:       javp.Code,
		Name:       namePrefix + javp.Name,
		RadiusType: radiusType,
		EnumValues: javp.EnumValues,
		EnumCodes:  codes,
	}
}
