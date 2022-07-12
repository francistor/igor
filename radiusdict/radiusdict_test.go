package radiusdict

import (
	"os"
	"testing"
)

func TestRadiusDict(t *testing.T) {

	// Read the full Radius Dictionary
	jsonDict, _ := os.ReadFile("/home/francisco/igor/resources/radiusDictionary.json")
	radiusDict := NewDictionaryFromJSON(jsonDict)

	// Basic type
	avp := radiusDict.AVPByCode[AVPCode{0, 1}]
	if avp.Name != "User-Name" {
		t.Errorf("Code {0, 1} Name was not User-Name")
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
	if avp.Tagged == true {
		t.Errorf("Code {0, 1} codes was tagged")
	}
	if avp.Encrypted == true {
		t.Errorf("Code {0, 1} codes was encrypted")
	}

	// Encrypted type
	avp = radiusDict.AVPByCode[AVPCode{0, 2}]
	if avp.Name != "User-Password" {
		t.Errorf("Code {0, 2} Name was not User-Password")
	}
	if avp.Encrypted != true {
		t.Errorf("Code {0, 2} Name was not Encrypted")
	}

	// Enum values
	avp = radiusDict.AVPByName["Service-Type"]
	if avp.Name != "Service-Type" {
		t.Errorf("Service-Type Name was not Service-Type")
	}
	if avp.RadiusType != Integer {
		t.Errorf("Service-Type Type was not type Integer")
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

	// VendorId
	avp = radiusDict.AVPByCode[AVPCode{9, 1}]
	if avp.Name != "Cisco-AVPair" {
		t.Errorf("Code {9, 1} Name was not Cisco-AVPair but %s", avp.Name)
	}

	// Vendor Specific
	avp, err := radiusDict.GetFromName("Igor-ClientId")
	if err != nil {
		t.Errorf("Igor-ClientId not found")
	} else {
		if avp.Code != 1 {
			t.Errorf("Igor-Client id code is not 1")
		}
		if avp.VendorId != 90001 {
			t.Errorf("Igor-Client has not vendorId code 90001")
		}
	}

	avp, err = radiusDict.GetFromCode(AVPCode{90001, 4})
	if err != nil {
		t.Errorf("Igor code 4 not found")
	} else {
		if avp.Name != "Igor-TaggedId" {
			t.Errorf("Igor code 4 is not Igor-TaggedId but %s", avp.Name)
		}
		if avp.Tagged != true {
			t.Error("Igor code 4 is not Tagged", avp.Name)
		}
	}
}

func TestUnknownRadiusAVP(t *testing.T) {
	// Read the full Radius Dictionary
	jsonDict, _ := os.ReadFile("/home/francisco/igor/resources/radiusDictionary.json")
	radiusDict := NewDictionaryFromJSON(jsonDict)

	avp, err := radiusDict.GetFromName("Igor-Nothing")
	if err == nil {
		t.Errorf("Igor-Nothing was found")
	}
	if avp.Name != "UNKNOWN" {
		t.Errorf("Igor-Nothing name is not UNKNOWN")
	}
}
