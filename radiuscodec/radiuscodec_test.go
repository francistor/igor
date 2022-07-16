package radiuscodec

import (
	"bytes"
	"fmt"
	"igor/config"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
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
		t.Errorf("error creating AVP: %v", err)
		return
	}
	if avp.GetString() != fmt.Sprintf("%x", password) {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != fmt.Sprintf("%x", password) {
		t.Errorf("value does not match after unmarshalling. Got %s", rebuiltAVP.GetString())
	}
	if !reflect.DeepEqual(rebuiltAVP.GetOctets(), []byte(password)) {
		t.Errorf("value does not match after unmarshalling. Got %v", rebuiltAVP.GetOctets())
	}
}

func TestStringAVP(t *testing.T) {

	var theValue = "this-is the string!"

	// Create avp
	avp, err := NewAVP("User-Name", theValue)
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}
	if avp.GetString() != theValue {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != theValue {
		t.Errorf("value does not match after unmarshalling. Got %s", rebuiltAVP.GetString())
	}
}

func TestVendorStringAVP(t *testing.T) {

	var theValue = "this is the string!"

	// Create avp
	avp, err := NewAVP("Igor-StringAttribute", theValue)
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}
	if avp.GetString() != theValue {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != theValue {
		t.Errorf("value does not match after unmarshalling. Got %s", rebuiltAVP.GetString())
	}
}

func TestVendorIntegerAVP(t *testing.T) {

	var theValue = 2

	// Create avp
	avp, err := NewAVP("Igor-IntegerAttribute", theValue)
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}
	if int(avp.GetInt()) != theValue {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if int(rebuiltAVP.GetInt()) != theValue {
		t.Errorf("value does not match after unmarshalling. Got %d", rebuiltAVP.GetInt())
	}
	if rebuiltAVP.GetString() != "Two" {
		t.Errorf("value does not match after unmarshalling. Got <%v>", rebuiltAVP.GetString())
	}
}

func TestVendorAddressTaggedAVP(t *testing.T) {

	var theValue = "1.2.3.4"

	// Create avp
	avp, err := NewAVP("Igor-AddressAttribute", theValue+":1")
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}
	if avp.GetString() != theValue {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if !rebuiltAVP.GetIPAddress().Equal(net.ParseIP(theValue)) {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetIPAddress())
	}
}

func TestVendorIPv6AddressAVP(t *testing.T) {

	var theValue = "bebe:cafe::0"

	// Create avp
	avp, err := NewAVP("Igor-IPv6AddressAttribute", theValue)
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}

	if avp.GetIPAddress().Equal(net.IP(theValue)) {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if !rebuiltAVP.GetIPAddress().Equal(net.ParseIP(theValue)) {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetIPAddress())
	}
}

func TestIPv6PrefixAVP(t *testing.T) {

	var theValue = "bebe:cafe::0/16"

	// Create avp
	avp, err := NewAVP("Framed-IPv6-Prefix", theValue)
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}

	if avp.GetString() != theValue {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if !strings.Contains(rebuiltAVP.GetString(), "bebe:cafe") {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetString())
	}
	if !strings.Contains(rebuiltAVP.GetString(), "/16") {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetString())
	}
}

func TestVendorTimeAVP(t *testing.T) {

	var timeFormatString = "2006-01-02T15:04:05 UTC"
	var theValue = "2020-09-06T21:08:09 UTC"
	var timeValue, err = time.Parse(timeFormatString, theValue)

	// Create avp
	avp, err := NewAVP("Igor-TimeAttribute", theValue)
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}

	if avp.GetString() != theValue {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetDate() != timeValue {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetDate())
	}
}

func TestInterfaceIdAVP(t *testing.T) {

	var theValue = []byte{0x01, 0x02, 0x03, 0x04, 0x01, 0x02, 0x03, 0x04}

	// Create avp
	avp, err := NewAVP("Framed-Interface-Id", theValue)
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}

	if avp.GetString() != fmt.Sprintf("%x", theValue) {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != fmt.Sprintf("%x", theValue) {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetDate())
	}
}

func TestVendorInteger64AVP(t *testing.T) {

	var theValue = -9000

	// Create avp
	avp, err := NewAVP("Igor-Integer64Attribute", theValue)
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}
	if int(avp.GetInt()) != theValue {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP)
	if int(rebuiltAVP.GetInt()) != theValue {
		t.Errorf("value does not match after unmarshalling. Got %d", rebuiltAVP.GetInt())
	}
}

func TestTaggedAVP(t *testing.T) {

	theValue := "this is a tagged attribute!"

	// Create 0
	avp, err := NewAVP("Igor-TaggedStringAttribute", theValue+":1")
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, err := RadiusAVPFromBytes(binaryAVP)
	if err != nil {
		t.Errorf("value does not match after unmarshalling. Got <%v>", err.Error())
	}
	if rebuiltAVP.GetString() != theValue {
		t.Errorf("value does not match after unmarshalling. Got <%v>", rebuiltAVP.GetString())
	}
}

func TestEncrypFunction(t *testing.T) {
	authenticator := GetAuthenticator()
	password := "__! $? this is the - ñ long password  '            7887"

	cipherText := Encrypt1("mysecret", authenticator, []byte(password))

	clearText := Decrypt1("mysecret", authenticator, cipherText)
	if string(bytes.Trim(clearText, "\x00")) != password {
		t.Errorf("cleartext does not match the original one")
	}
}
