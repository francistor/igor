package core

import (
	"testing"
)

func TestFreeradiusParser(t *testing.T) {

	ci := GetPolicyConfig()

	var jDict jRadiusDict
	err := ParseFreeradiusDictionary(&ci.CM, "dictionary", &jDict)
	if err != nil {
		t.Fatal(err)
	}

	dict := newRadiusDictionaryFromJDict(&jDict)

	/*
		for _, avp := range jDict.Avps {
			for _, attr := range avp.Attributes {
				fmt.Println(avp.VendorId, attr.Name)
			}
		}
	*/

	myAVP, ok := dict.AVPByName["Igor-IntegerAttribute"]
	if !ok {
		t.Fatal("Attribute not found")
	}
	if myAVP.Code != 3 {
		t.Fatal("Igor-IntegerAttribute has not code 3")
	}
	if myAVP.EnumValues["One"] != 1 {
		t.Fatal("Igor-IntegerAttribute has no item 'One'")
	}

}
