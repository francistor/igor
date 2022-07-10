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
