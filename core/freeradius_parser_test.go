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

	// fmt.Println(jDict)

	/*
		dict := newRadiusDictionaryFromJDict(&jDict)

		myAVP := dict.AVPByName["Igor-IntegerAttribute"]
		if myAVP.Code != 3 {
			t.Fatal("Igor-IntegerAttribute has not code 3")
		}
		if myAVP.EnumValues["One"] != 1 {
			t.Fatal("Igor-IntegerAttribute has no item 'One'")
		}
	*/
}
