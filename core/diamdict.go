package core

/*
Helpers for reading and using the Diameter dictionary
*/

import (
	"encoding/json"
	"fmt"
)

type DiameterAVPType int

// One for each Diamter AVP Type
const (
	DiameterTypeNone         = 0
	DiameterTypeOctetString  = 1
	DiameterTypeInteger32    = 2
	DiameterTypeInteger64    = 3
	DiameterTypeUnsigned32   = 4
	DiameterTypeUnsigned64   = 5
	DiameterTypeFloat32      = 6
	DiameterTypeFloat64      = 7
	DiameterTypeGrouped      = 8
	DiameterTypeAddress      = 9
	DiameterTypeTime         = 10
	DiameterTypeUTF8String   = 11
	DiameterTypeDiamIdent    = 12
	DiameterTypeDiameterURI  = 13
	DiameterTypeEnumerated   = 14
	DiameterTypeIPFilterRule = 15

	// Radius types for Diameter ecnapsulation
	DiameterTypeIPv4Address = 1001
	DiameterTypeIPv6Address = 1002
	DiameterTypeIPv6Prefix  = 1003
)

var UnknownDiameterDictItem = DiameterAVPDictItem{
	Name: "UNKNOWN",
}

// VendorId and code of AVP in a single attribute
type DiameterAVPCode struct {
	VendorId uint32
	Code     uint32
}

// Attributes of a Grouped AVP
type GroupedProperties struct {
	Mandatory bool
	MinOccurs int
	MaxOccurs int
}

// Diameter Dictionary element
type DiameterAVPDictItem struct {
	// 3 bytes required according to RFC 6733
	VendorId uint32
	// 3 bytes required according to RFC 6733
	Code uint32
	Name string
	// One of the constants above
	DiameterType DiameterAVPType
	// Map of Names to codes
	EnumNames map[string]int
	// Map of codes (ints) to Names
	EnumCodes map[int]string
	// non nil only in grouped type. Key is the internal attribute name
	Group map[string]GroupedProperties
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

	CommandByName map[string]*DiameterCommand

	CommandByCode map[uint32]*DiameterCommand
}

// Represents the full Diameter Dictionary
type DiameterDict struct {
	// Map of vendor id to vendor name
	VendorById map[uint32]string

	// Map of vendor name to vendor id
	VendorByName map[string]uint32

	// Map of avp code to name. Name is <vendorName>-<attributeName>
	AVPByCode map[DiameterAVPCode]*DiameterAVPDictItem

	// Map of avp name to code
	AVPByName map[string]*DiameterAVPDictItem

	// Map of app names
	AppByName map[string]*DiameterApplication

	// Map of app codes
	AppByCode map[uint32]*DiameterApplication
}

// Returns an empty dictionary item if the code is not found
// The user may decide to go on with an UNKNOWN dictionary item when the error is returned
func (dd *DiameterDict) GetAVPFromCode(code DiameterAVPCode) (*DiameterAVPDictItem, error) {
	if di, found := dd.AVPByCode[code]; !found {
		return &UnknownDiameterDictItem, fmt.Errorf("%v not found in dictionary", code)
	} else {
		return di, nil
	}
}

// Returns an empty dictionary item if the code is not found
// The user may decide to go on with an UNKNOWN dictionary item when the error is returned
func (dd *DiameterDict) GetAVPFromName(name string) (*DiameterAVPDictItem, error) {
	if di, found := dd.AVPByName[name]; !found {
		return &UnknownDiameterDictItem, fmt.Errorf("%s not found in dictionary", name)
	} else {
		return di, nil
	}
}

// Returns a DiameterAppplication given the appid and command code
func (dd *DiameterDict) GetApplication(appId uint32) (*DiameterApplication, error) {
	if app, ok := dd.AppByCode[appId]; !ok {
		return nil, fmt.Errorf("appId %d not found", appId)
	} else {
		return app, nil
	}
}

// Returns a DiameterCommand given the appid and command code
func (dd *DiameterDict) GetCommand(appId uint32, commandCode uint32) (*DiameterCommand, error) {
	if app, found := dd.AppByCode[appId]; found {
		if command, ok := app.CommandByCode[commandCode]; ok {
			return command, nil
		} else {
			return nil, fmt.Errorf("appId %d and command %d not found", appId, commandCode)
		}
	} else {
		return nil, fmt.Errorf("appId %d not found", appId)
	}
}

// Returns a Diameter Dictionary object from its serialized representation
func NewDiameterDictionaryFromJSON(data []byte) *DiameterDict {

	// Unmarshall from JSON
	var jDict jDiameterDict
	if err := json.Unmarshal(data, &jDict); err != nil {
		panic("bad diameter dictionary format " + err.Error())
	}

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
	dict.AVPByCode = make(map[DiameterAVPCode]*DiameterAVPDictItem)
	dict.AVPByName = make(map[string]*DiameterAVPDictItem)
	for _, vendorAVPs := range jDict.Avps {
		vendorId := vendorAVPs.VendorId
		vendorName := dict.VendorById[vendorId]

		// Map all atttributtes from this vendor
		for _, attr := range vendorAVPs.Attributes {
			avpDictItem := attr.toAVPDictItem(vendorId, vendorName)
			dict.AVPByCode[DiameterAVPCode{vendorId, attr.Code}] = &avpDictItem
			dict.AVPByName[avpDictItem.Name] = &avpDictItem
		}
	}

	// Build the applications map
	dict.AppByCode = make(map[uint32]*DiameterApplication)
	dict.AppByName = make(map[string]*DiameterApplication)
	for i := range jDict.Applications {
		// Fill the Applications map
		// Do not use the value in the range. Copy the pointer as done here!
		app := &jDict.Applications[i]
		dict.AppByCode[app.Code] = app
		dict.AppByName[app.Name] = app

		// Fill the commands map for the application
		app.CommandByCode = make(map[uint32]*DiameterCommand)
		app.CommandByName = make(map[string]*DiameterCommand)
		for j, command := range app.Commands {
			app.CommandByCode[command.Code] = &app.Commands[j]
			app.CommandByName[command.Name] = &app.Commands[j]
		}
	}

	return &dict
}

/*
The following types are helpers for unserializing the JSON Diameter Dictionary
*/

// To Unmarshall Dictionary from Json
type jDiameterAVP struct {
	Code      uint32
	Name      string
	Type      string
	EnumNames map[string]int
	Group     map[string]GroupedProperties
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

func (javp jDiameterAVP) toAVPDictItem(v uint32, vs string) DiameterAVPDictItem {
	var diameterType DiameterAVPType
	switch javp.Type {
	case "None":
		diameterType = DiameterTypeNone
	case "OctetString":
		diameterType = DiameterTypeOctetString
	case "Integer32":
		diameterType = DiameterTypeInteger32
	case "Integer64":
		diameterType = DiameterTypeInteger64
	case "Unsigned32":
		diameterType = DiameterTypeUnsigned32
	case "Unsigned64":
		diameterType = DiameterTypeUnsigned64
	case "Float32":
		diameterType = DiameterTypeFloat32
	case "Float64":
		diameterType = DiameterTypeFloat64
	case "Grouped":
		diameterType = DiameterTypeGrouped
	case "Address":
		diameterType = DiameterTypeAddress
	case "Time":
		diameterType = DiameterTypeTime
	case "UTF8String":
		diameterType = DiameterTypeUTF8String
	case "DiamIdent":
		diameterType = DiameterTypeDiamIdent
	case "DiameterURI":
		diameterType = DiameterTypeDiameterURI
	case "Enumerated":
		diameterType = DiameterTypeEnumerated
	case "IPFilterRule":
		diameterType = DiameterTypeIPFilterRule

	// Radius types
	case "IPv4Address":
		diameterType = DiameterTypeIPv4Address
	case "IPv6Address":
		diameterType = DiameterTypeIPv6Address
	case "IPv6Prefix":
		diameterType = DiameterTypeIPv6Prefix
	default:
		panic(javp.Type + " is not a valid DiameterType")
	}

	var codes map[int]string
	if javp.EnumNames != nil {
		codes = make(map[int]string)
		for enumName, enumValue := range javp.EnumNames {
			codes[enumValue] = enumName
		}
	}

	var namePrefix string
	if vs != "" {
		namePrefix = vs + "-"
	}

	return DiameterAVPDictItem{
		VendorId:     v,
		Code:         javp.Code,
		Name:         namePrefix + javp.Name,
		DiameterType: diameterType,
		EnumNames:    javp.EnumNames,
		EnumCodes:    codes,
		Group:        javp.Group,
	}
}
