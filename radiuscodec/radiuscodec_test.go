package radiuscodec

import (
	"fmt"
	"igor/config"
	"os"
	"reflect"
	"testing"
)

// Initialization
var bootstrapFile = "resources/searchRules.json"
var instanceName = "testClient"

// Initializer of the test suite.
func TestMain(m *testing.M) {
	config.InitPolicyConfigInstance(bootstrapFile, instanceName, true)

	// Execute the tests and exit
	os.Exit(m.Run())
}

func TestAVPNotFound(t *testing.T) {
	var _, err = NewAVP("Unknown AVP", []byte("hello, world!"))
	if err == nil {
		t.Errorf("Unknown AVP was created")
	}
}

func TestOctetsAVP(t *testing.T) {

	var password = "'my-password!"

	// Create avp
	avp, err := NewAVP("User-Password", []byte(password))
	if err != nil {
		t.Errorf("error creating Octets AVP: %v", err)
		return
	}
	if avp.GetString() != fmt.Sprintf("%x", password) {
		t.Errorf("Octets AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != fmt.Sprintf("%x", password) {
		t.Errorf("Octets AVP not properly encoded after unmarshalling. Got %s", rebuiltAVP.GetString())
	}
	if !reflect.DeepEqual(rebuiltAVP.GetOctets(), []byte(password)) {
		t.Errorf("Octets AVP not properly encoded after unmarshalling. Got %v instead of %v", rebuiltAVP.GetOctets(), []byte(password))
	}
}

func TestStringAVP(t *testing.T) {

	var theValue = "this-is the string!"

	// Create avp
	avp, err := NewAVP("User-Name", theValue)
	if err != nil {
		t.Errorf("error string String AVP: %v", err)
		return
	}
	if avp.GetString() != theValue {
		t.Errorf("String AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != theValue {
		t.Errorf("String AVP not properly encoded after unmarshalling. Got %s", rebuiltAVP.GetString())
	}
}

func TestVendorStringAVP(t *testing.T) {

	var theValue = "this is the string!"

	// Create avp
	avp, err := NewAVP("Igor-StringAttribute", theValue)
	if err != nil {
		t.Errorf("error vendor specific string AVP: %v", err)
		return
	}
	if avp.GetString() != theValue {
		t.Errorf("String vendor specific AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != theValue {
		t.Errorf("Vendor specific string AVP not properly encoded after unmarshalling. Got <%s>", rebuiltAVP.GetString())
	}
}

func TestVendorIntegerAVP(t *testing.T) {

	var theValue = 2

	// Create avp
	avp, err := NewAVP("Igor-IntegerAttribute", theValue)
	if err != nil {
		t.Errorf("error vendor specific integer AVP: %v", err)
		return
	}
	if int(avp.GetInt()) != theValue {
		t.Errorf("Integer vendor specific AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if int(rebuiltAVP.GetInt()) != theValue {
		t.Errorf("Vendor specific integer AVP not properly encoded after unmarshalling. Got <%d>", rebuiltAVP.GetInt())
	}
	if rebuiltAVP.GetString() != "Two" {
		t.Errorf("Vendor specific integer AVP not properly encoded after unmarshalling. Got <%s>", rebuiltAVP.GetString())
	}
}

func TestTaggedAVP(t *testing.T) {

	theValue := "this is a tagged attribute!"

	// Create 0
	avp, err := NewAVP("Igor-TaggedStringAttribute", theValue+":1")
	if err != nil {
		t.Errorf("error creating Tagged AVP: %v", err)
		return
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, err := RadiusAVPFromBytes(binaryAVP)
	if err != nil {
		t.Errorf("Tagged AVP not properly encoded after unmarshalling. Got %s", err.Error())
	}
	if rebuiltAVP.GetString() != theValue {
		t.Errorf("Tagged AVP not properly encoded after unmarshalling. Got %s", rebuiltAVP.GetString())
	}
}
