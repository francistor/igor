package core

import (
	"testing"
)

func TestFreeradiusParser(t *testing.T) {

	ci := GetPolicyConfig()

	var jDict jRadiusDict
	err := ParseFreeradiusDictionary(&ci.CM, "dictionary", "", &jDict)
	if err != nil {
		t.Fatal(err)
	}

	dict := newRadiusDictionaryFromJDict(&jDict)

	myAVP, ok := dict.AVPByName["Igor-IntegerAttribute"]
	if !ok {
		t.Fatal("Attribute Igor-IntegerAttribute not found")
	}
	if myAVP.Code != 3 {
		t.Fatal("Igor-IntegerAttribute has not code 3")
	}
	if myAVP.EnumNames["One"] != 1 {
		t.Fatal("Igor-IntegerAttribute has no item 'One'")
	}

	otherAVP, ok := dict.AVPByName["SessionStore-Id"]
	if !ok {
		t.Fatal("Attribute SessionStore-Id not found")
	}
	if otherAVP.Code != 3 {
		t.Fatal("SessionStore-Id has not code 3")
	}
}
