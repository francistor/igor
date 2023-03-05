package core

import (
	"os"
	"testing"
)

func TestRadiusDict(t *testing.T) {

	// Read the full Radius Dictionary
	jsonDict, _ := os.ReadFile("../resources/radiusDictionary.json")
	radiusDict := NewRadiusDictionaryFromJSON(jsonDict)

	// Basic type
	avp := radiusDict.AVPByCode[RadiusAVPCode{0, 1}]
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
	avp = radiusDict.AVPByCode[RadiusAVPCode{0, 2}]
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
	if avp.RadiusType != RadiusTypeInteger {
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
	avp = radiusDict.AVPByCode[RadiusAVPCode{9, 1}]
	if avp.Name != "Cisco-AVPair" {
		t.Errorf("Code {9, 1} Name was not Cisco-AVPair but %s", avp.Name)
	}

	// Vendor Specific
	avp, err := radiusDict.GetFromName("Igor-OctetsAttribute")
	if err != nil {
		t.Errorf("Igor-OctetsAttribute not found")
	} else {
		if avp.Code != 1 {
			t.Errorf("Igor-OctetsAttribute id code is not 1")
		}
		if avp.VendorId != 30001 {
			t.Errorf("Igor-OctetsAttribute has not vendorId code 90001")
		}
	}

	avp, err = radiusDict.GetFromCode(RadiusAVPCode{30001, 10})
	if err != nil {
		t.Errorf("Igor code 10 not found")
	} else {
		if avp.Name != "Igor-Command" {
			t.Errorf("Igor code 10 is not Igor-Command but %s", avp.Name)
		}
	}
	avp, err = radiusDict.GetFromCode(RadiusAVPCode{30001, 13})
	if err != nil {
		t.Errorf("Igor code 13 not found")
	} else {
		if avp.Name != "Igor-TaggedSaltedOctetsAttribute" {
			t.Errorf("Igor code 13 is not Igor-TaggedSaltedOctetsAttribute but %s", avp.Name)
		}
		if !avp.Salted || !avp.Tagged {
			t.Errorf("Igor code 13 is not Tagged or Salted")
		}
	}
}

func TestUnknownRadiusAVP(t *testing.T) {
	// Read the full Radius Dictionary
	jsonDict, _ := os.ReadFile("../resources/radiusDictionary.json")
	radiusDict := NewRadiusDictionaryFromJSON(jsonDict)

	avp, err := radiusDict.GetFromName("Igor-Nothing")
	if err == nil {
		t.Errorf("Igor-Nothing was found")
	}
	if avp.Name != "UNKNOWN" {
		t.Errorf("Igor-Nothing name is not UNKNOWN")
	}
}
