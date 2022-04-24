package diamdict

/*
Package diamdict impements helpers for reading and using the Diameter dictionary

*/

import (
	"encoding/json"
	"fmt"
)

// One for each Diamter AVP Type
const (
	None         = 0
	OctetString  = 1
	Integer32    = 2
	Integer64    = 3
	Unsigned32   = 4
	Unsigned64   = 5
	Float32      = 6
	Float64      = 7
	Grouped      = 8
	Address      = 9
	Time         = 10
	UTF8String   = 11
	DiamIdent    = 12
	DiameterURI  = 13
	Enumerated   = 14
	IPFilterRule = 15

	// Radius types
	IPv4Address = 1001
	IPv6Address = 1002
	IPv6Prefix  = 1003
)

// VendorId and code of AVP in a single attribute
type AVPCode struct {
	VendorId uint32
	Code     uint32
}

// Attributes of a Grouped AVP
type GroupedProperties struct {
	Mandatory bool
	MinOccurs int
	MaxOccurs int
}

// Diameter Dictionary elements
type AVPDictItem struct {
	VendorId     uint32 // 3 bytes required according to RFC 6733
	Code         uint32 // 3 bytes required according to RFC 6733
	Name         string
	DiameterType int                          // One of the constants above
	EnumValues   map[string]int               // non nil only in enum type
	EnumCodes    map[int]string               // non  nil only in enum type
	Group        map[string]GroupedProperties // non nil only in grouped type
}

// Represents a Diameter Command
type DiameterCommand struct {
	Name     string
	Code     uint32
	Request  map[string]GroupedProperties
	Response map[string]GroupedProperties
}

// Represents a Diameter Application
type DiameterApplication struct {
	Name     string
	Code     uint32
	AppType  string
	Commands []DiameterCommand

	CommandByName map[string]DiameterCommand

	CommandByCode map[uint32]DiameterCommand
}

// Represents the full Diameter Dictionary
type DiameterDict struct {
	// Map of vendor id to vendor name
	VendorById map[uint32]string

	// Map of vendor name to vendor id
	VendorByName map[string]uint32

	// Map of avp code to name. Name is <vendorName>-<attributeName>
	AVPByCode map[AVPCode]AVPDictItem

	// Map of avp name to code
	AVPByName map[string]AVPDictItem

	// Map of app names
	AppByName map[string]DiameterApplication

	// Map of app codes
	AppByCode map[uint32]DiameterApplication
}

// Returns an empty dictionary item if the code is not found
// The user may decide to go on with an UNKNOWN dictionary item when the error is returned
func (dd *DiameterDict) GetFromCode(code AVPCode) (AVPDictItem, error) {
	di := dd.AVPByCode[code]
	if di.Name == "" {
		di.Name = "UNKNOWN"
		return di, fmt.Errorf("%v not found in dictionary", code)
	}
	return di, nil
}

// Returns an empty dictionary item if the code is not found
// The user may decide to go on with an UNKNOWN dictionary item when the error is returned
func (dd *DiameterDict) GetFromName(name string) (AVPDictItem, error) {
	di, ok := dd.AVPByName[name]
	if !ok {
		di.Name = "UNKNOWN"
		return di, fmt.Errorf("%s not found in dictionary", name)
	}
	return di, nil
}

// Returns a Diameter Dictionary object from its serialized representation
func NewDictionaryFromJSON(data []byte) *DiameterDict {

	// Unmarshall from JSON
	var jDict jDiameterDict
	json.Unmarshal(data, &jDict)

	// Build the dictionary
	var dict DiameterDict

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

	// Build the applications map
	dict.AppByCode = make(map[uint32]DiameterApplication)
	dict.AppByName = make(map[string]DiameterApplication)
	for _, app := range jDict.Applications {
		app.CommandByName = make(map[string]DiameterCommand)
		app.CommandByCode = make(map[uint32]DiameterCommand)
		for _, command := range app.Commands {
			// Fill the commands map for the application
			app.CommandByCode[command.Code] = command
			app.CommandByName[command.Name] = command
		}

		// Fill the Applications map
		dict.AppByCode[app.Code] = app
		dict.AppByName[app.Name] = app
	}

	return &dict
}

/*
The following types are helpers for unserializing the JSON Diameter Dictionary
*/

// To Unmarshall Dictionary from Json
type jDiameterAVP struct {
	Code       uint32
	Name       string
	Type       string
	EnumValues map[string]int
	Group      map[string]GroupedProperties
}

func (javp jDiameterAVP) toAVPDictItem(v uint32, vs string) AVPDictItem {
	var diameterType int
	switch javp.Type {
	case "None":
		diameterType = None
	case "OctetString":
		diameterType = OctetString
	case "Integer32":
		diameterType = Integer32
	case "Integer64":
		diameterType = Integer64
	case "Unsigned32":
		diameterType = Unsigned32
	case "Unsigned64":
		diameterType = Unsigned64
	case "Float32":
		diameterType = Float32
	case "Float64":
		diameterType = Float64
	case "Grouped":
		diameterType = Grouped
	case "Address":
		diameterType = Address
	case "Time":
		diameterType = Time
	case "UTF8String":
		diameterType = UTF8String
	case "DiamIdent":
		diameterType = DiamIdent
	case "DiameterURI":
		diameterType = DiameterURI
	case "Enumerated":
		diameterType = Enumerated
	case "IPFilterRule":
		diameterType = IPFilterRule

	// Radius types
	case "IPv4Address":
		diameterType = IPv4Address
	case "IPv6Address":
		diameterType = IPv6Address
	case "IPv6Prefix":
		diameterType = IPv6Prefix
	default:
		panic(javp.Type + " is not a valid DiameterType")
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
		VendorId:     v,
		Code:         javp.Code,
		Name:         namePrefix + javp.Name,
		DiameterType: diameterType,
		EnumValues:   javp.EnumValues,
		EnumCodes:    codes,
		Group:        javp.Group,
	}
}

type jDiameterVendorAVPs struct {
	VendorId   uint32
	Attributes []jDiameterAVP
}

type jDiameterDict struct {
	Version int
	Vendors []struct {
		VendorId   uint32
		VendorName string
	}
	Avps         []jDiameterVendorAVPs
	Applications []DiameterApplication
}
