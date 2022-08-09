package radiuscodec

import (
	"bytes"
	"encoding/json"
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
var instanceName = "testServer"

var authenticator = [16]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0F}
var secret = "mysecret"

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

func TestPasswordAVP(t *testing.T) {

	//var password = "'my-password! and a very long one indeed %&$"
	//var password = "1234567890123456"
	var password = "0"

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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
	if !reflect.DeepEqual(bytes.Trim(rebuiltAVP.GetOctets(), "\x00"), []byte(password)) {
		t.Errorf("value does not match after unmarshalling. Got %v", rebuiltAVP.GetOctets())
	}
	rebuiltPassword, err := rebuiltAVP.GetPasswordString()
	if err != nil {
		t.Errorf(err.Error())
	} else if rebuiltPassword != password {
		t.Errorf("password does not match. Got %s", rebuiltPassword)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
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
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, err := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
	if err != nil {
		t.Errorf("value does not match after unmarshalling. Got <%v>", err.Error())
	}
	if rebuiltAVP.GetString() != theValue {
		t.Errorf("value does not match after unmarshalling. Got <%v>", rebuiltAVP.GetString())
	}
}

func TestSaltedAVP(t *testing.T) {

	theValue := "this is a salted attribute! and a very long one indeed!"

	// Create 0
	avp, err := NewAVP("Igor-SaltedOctetsAttribute", []byte(theValue))
	if err != nil {
		t.Errorf("error creating avp: %v", err)
		return
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
	if !reflect.DeepEqual(bytes.Trim(rebuiltAVP.GetOctets(), "\x00"), []byte(theValue)) {
		t.Errorf("value does not match after unmarshalling. Got %v", rebuiltAVP.GetOctets())
	}
	rebuiltValue, err := rebuiltAVP.GetPasswordString()
	if err != nil {
		t.Errorf(err.Error())
	} else if rebuiltValue != theValue {
		t.Errorf("value does not match. Got %s", rebuiltValue)
	}
}

func TestEncryptFunction(t *testing.T) {
	authenticator := GetAuthenticator()
	password := "__! $? this is the - ñ long password  '            7887"

	cipherText := encrypt1([]byte(password), authenticator, "mysecret", nil)

	clearText := decrypt1(cipherText, authenticator, "mysecret", nil)
	if string(bytes.Trim(clearText, "\x00")) != password {
		t.Errorf("cleartext does not match the original one")
	}
}

/////////////////////////////////////////////////////////////////////////////////////
func TestRadiusPacket(t *testing.T) {

	theUserName := "MyUserName"
	thePassword := "pwd"

	request := NewRadiusRequest(ACCESS_REQUEST)
	request.Add("User-Name", theUserName)
	request.Add("User-Password", []byte(thePassword))

	// Serialize
	packetBytes, err := request.ToBytes(secret, 0)
	if err != nil {
		t.Errorf("could not serialize packet: %s", err)
	}

	// Unserialize
	recoveredPacket, err := RadiusPacketFromBytes(packetBytes, secret)
	if err != nil {
		t.Errorf("could not unserialize packet: %s", err)
	}

	if userName := recoveredPacket.GetStringAVP("User-Name"); userName != theUserName {
		t.Errorf("attribute does not match <%s>", userName)
	}

	if password := recoveredPacket.GetPasswordStringAVP("User-Password"); password != thePassword {
		t.Errorf("attribute does not match <%s>", password)
	}

	response := NewRadiusResponse(request, true)
	responseBytes, err := response.ToBytes(secret, 0)
	if err != nil {
		t.Error(err)
	}

	if !ValidateResponseAuthenticator(responseBytes, request.Authenticator, secret) {
		t.Errorf("response has invalid authenticator")
	}
}

func TestJSONAVP(t *testing.T) {

	var javp = `{
		"Igor-TaggedStringAttribute": "TaggedAttribute:1"
	}`

	// Unserialize
	avp := RadiusAVP{}
	if err := json.Unmarshal([]byte(javp), &avp); err != nil {
		t.Fatalf("could not unmarshal avp: %s", err)
	}
	if avp.GetString() != "TaggedAttribute" {
		t.Errorf("attribute does not match expected value. Got <%s>", avp.GetString())
	}
	if avp.Tag != 1 {
		t.Errorf("tag does not match expected value. got %d", avp.Tag)
	}

	// Serialize
	if jsonBytes, err := json.Marshal(&avp); err != nil {
		t.Fatalf("could not marshal avp: %s", err)
	} else {
		if string(jsonBytes) != "{\"Igor-TaggedStringAttribute\":\"TaggedAttribute:1\"}" {
			t.Errorf("serialized avp not as expected. got <%s>", string(jsonBytes))
		}
	}
}

func TestJSONPacket(t *testing.T) {

	jsonPacket := `{
				"Code": 1,
				"AVPs":[
					{"Igor-OctetsAttribute": "0102030405060708090a0b"},
					{"Igor-StringAttribute": "stringvalue"},
					{"Igor-IntegerAttribute": "Zero"},
					{"Igor-IntegerAttribute": "1"},
					{"Igor-IntegerAttribute": 1},
					{"Igor-AddressAttribute": "127.0.0.1:1"},
					{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
					{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
					{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
					{"Igor-InterfaceIdAttribute": "00aabbccdd"},
					{"Igor-Integer64Attribute": 999999999999},
					{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
					{"User-Name":"MyUserName"}
				]
			}`

	// Read JSON to Radius Packet
	radiusPacket := RadiusPacket{}
	if err := json.Unmarshal([]byte(jsonPacket), &radiusPacket); err != nil {
		t.Fatalf("unmarshal error for radius packet: %s", err)
	}

	// Check attributes
	taggedIPAddress := radiusPacket.GetTaggedStringAVP("Igor-AddressAttribute")
	if taggedIPAddress != "127.0.0.1:1" {
		t.Fatalf("bad tagged IPAddress attribute %s", taggedIPAddress)
	}
	timeAttribute := radiusPacket.GetDateAVP("Igor-TimeAttribute")
	if timeAttribute.Hour() != 3 {
		t.Fatalf("bad time attribute %v", timeAttribute)
	}

	// Write RadiusPacket message as JSON
	jsonPacketNew, _ := json.Marshal(&radiusPacket)
	if !strings.Contains(string(jsonPacketNew), "1966-11-26T03:34:08 UTC") || !strings.Contains(string(jsonPacketNew), "Zero") {
		t.Errorf("marshalled json does not contain the expected attributes: %s", string(jsonPacketNew))
	}
}
