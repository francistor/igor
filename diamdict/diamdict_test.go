package diamdict

import (
	"os"
	"testing"
)

func TestDiamDict(t *testing.T) {

	// Read the full Diameter Dictionary
	jsonDict, _ := os.ReadFile("/home/francisco/igor/resources/diameterDictionary.json")
	diameterDict := NewDictionaryFromJSON(jsonDict)

	// Basic type
	avp := diameterDict.AVPByCode[AVPCode{0, 1}]
	if avp.Name != "User-Name" {
		t.Errorf("Code {0, 1} Name was not User-Name")
	}
	if avp.DiameterType != UTF8String {
		t.Errorf("Code {0, 1} Type was not type UTF8String")
	}
	if avp.VendorId != 0 {
		t.Errorf("Code {0, 1} Vendor was not vendorId 0")
	}
	if avp.EnumValues != nil {
		t.Errorf("Code {0, 1} values was not nil")
	}
	if avp.EnumCodes != nil {
		t.Errorf("Code {0, 1} codes was not nil")
	}
	if avp.Group != nil {
		t.Errorf("Code {0, 1} Group was not nil")
	}

	// Enum values
	avp = diameterDict.AVPByName["Service-Type"]
	if avp.Name != "Service-Type" {
		t.Errorf("Service-Type Name was not Service-Type")
	}
	if avp.DiameterType != Enumerated {
		t.Errorf("Service-Type Type was not type Enumerated")
	}
	if avp.VendorId != 0 {
		t.Errorf("Service-Type Vendor was not 0")
	}
	if avp.EnumValues == nil {
		t.Errorf("Service-Type EnumValues was nil")
	}
	if avp.EnumValues["Callback-Login"] != 3 {
		t.Errorf("Service-Type Callback-Login was not 3")
	}
	if avp.EnumCodes[4] != "Callback-Framed" {
		t.Errorf("Service-Type 4 was not Callback-Framed")
	}
	if avp.Group != nil {
		t.Errorf("Service-Type Group was not nil")
	}

	// VendorId
	avp = diameterDict.AVPByCode[AVPCode{10415, 505}]
	if avp.Name != "3GPP-AF-Charging-Identifier" {
		t.Errorf("Code {10415, 505} Name was not 3GPP-AF-Charging-Identifier but %s", avp.Name)
	}

	// Grouped
	avp = diameterDict.AVPByName["3GPP-Flows"]
	if avp.DiameterType != Grouped {
		t.Errorf("3GPP-Flows is not Type Grouped")
	}
	if avp.Group["3GPP-Media-Component-Number"].MinOccurs != 1 {
		t.Errorf("3GPP-Flows.3GPP-Media-Component-Number has not MinOccurs 1")
	}

	// Applications
	app := diameterDict.AppByCode[1000]
	if app.Name != "TestApplication" {
		t.Errorf("Application code 1000 is not named TestApplication")
	}
	if app.CommandByCode[2000].Request["Session-Id"].Mandatory != true {
		t.Errorf("TestApplication Command 2000 Request Session-Id is not mandatory")
	}
	app = diameterDict.AppByName["Gx"]
	if app.Code != 16777238 {
		t.Errorf("Gx code is not 16777238")
	}
	if app.CommandByName["Credit-Control"].Response["3GPP-Online"].MaxOccurs != 1 {
		t.Errorf("Gx Command Credit-Control Response 3GPP-Online MaxOccurs is not 1")
	}
}

func TestUnknownDiameterAVP(t *testing.T) {
	// Read the full Diameter Dictionary
	jsonDict, _ := os.ReadFile("/home/francisco/igor/resources/diameterDictionary.json")
	diameterDict := NewDictionaryFromJSON(jsonDict)

	avp, err := diameterDict.GetFromName("Igor-Nothing")
	if err == nil {
		t.Errorf("Igor-Nothing was found")
	}
	if avp.Name != "UNKNOWN" {
		t.Errorf("Igor-Nothing name is not UNKNOWN")
	}
}
