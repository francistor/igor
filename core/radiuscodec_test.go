package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Initialization

var authenticator = [16]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0F}
var secret = "mysecret"

func TestAVPNotFound(t *testing.T) {
	var _, err = NewRadiusAVP("Unknown AVP", []byte("hello, world!"))
	if err == nil {
		t.Errorf("Unknown AVP was created")
	}
}

func TestPasswordAVP(t *testing.T) {

	var password = "'my-password! and a very long one indeed %&$"

	// Create avp
	avp, err := NewRadiusAVP("User-Password", password)
	if err != nil {
		t.Fatalf("error creating AVP: %v", err)
		return
	}
	if avp.GetString() != password {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
	rebuiltPassword := rebuiltAVP.GetString()
	if err != nil {
		t.Fatalf(err.Error())
	} else if rebuiltPassword != password {
		t.Errorf("password does not match. Got %s", rebuiltPassword)
	}
}

// Salted, Encrypted and WithLen
func TestTunnelPasswordAVP(t *testing.T) {

	var password = "'my-password! and a very long one indeed %&$"

	// Create avp
	avp, err := NewRadiusAVP("Tunnel-Password", password+":1")
	if err != nil {
		t.Fatalf("error creating AVP: %v", err)
		return
	}
	if avp.GetString() != password {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
	rebuiltPassword := rebuiltAVP.GetString()
	if err != nil {
		t.Fatalf(err.Error())
	} else if rebuiltPassword != password {
		t.Errorf("password does not match. Got %s", rebuiltPassword)
	}
}

func TestStringAVP(t *testing.T) {

	var theValue = "this-is the string!"

	// Create avp
	avp, err := NewRadiusAVP("User-Name", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
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
	avp, err := NewRadiusAVP("Igor-StringAttribute", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
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
	avp, err := NewRadiusAVP("Igor-IntegerAttribute", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
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

func TestVendorTaggedAVP(t *testing.T) {

	var theValue = "myString"

	// Create avp
	avp, err := NewRadiusAVP("Igor-TaggedStringAttribute", theValue+":1")
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
		return
	}
	if avp.GetString() != theValue {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
	if rebuiltAVP.GetString() != theValue {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetIPAddress())
	}
}

func TestVendorIPv6AddressAVP(t *testing.T) {

	var theValue = "bebe:cafe::0"

	// Create avp
	avp, err := NewRadiusAVP("Igor-IPv6AddressAttribute", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
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
	avp, err := NewRadiusAVP("Framed-IPv6-Prefix", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
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

	var theValue = "2020-09-06T21:08:09 UTC"
	var timeValue, err = time.Parse(TimeFormatString, theValue)

	// Create avp
	avp, err := NewRadiusAVP("Igor-TimeAttribute", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
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
	avp, err := NewRadiusAVP("Framed-Interface-Id", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
		return
	}

	if avp.GetString() != fmt.Sprintf("%x", theValue) {
		t.Errorf("value does not match")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
	if rebuiltAVP.GetString() != fmt.Sprintf("%x", theValue) {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetString())
	}
	if rebuiltAVP.GetOctets()[2] != 0x03 {
		t.Errorf("value does not match after unmarshalling. Got <%v>", avp.GetOctets()[2])
	}
}

func TestVendorInteger64AVP(t *testing.T) {

	var theValue = -900000000000000000

	// Create avp
	avp, err := NewRadiusAVP("Igor-Integer64Attribute", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
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
	avp, err := NewRadiusAVP("Igor-TaggedStringAttribute", theValue+":1")
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
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
	if rebuiltAVP.GetTaggedString() != theValue+":1" {
		t.Errorf("value does not match after unmarshalling. Got <%v>", rebuiltAVP.GetString())
	}
	if rebuiltAVP.GetTag() != 1 {
		t.Errorf("tag does not match. Got <%v>", rebuiltAVP.GetTag())
	}
}

func TestSaltedOctetsAVP(t *testing.T) {

	theValue := []byte("this is a salted attribute! and a very long one indeed!")

	// Create 0
	avp, err := NewRadiusAVP("Igor-SaltedOctetsAttribute", theValue)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
		return
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)
	// The retrieved value will be padded with zeroes
	var padLen int
	if len(theValue)%16 == 0 {
		padLen = 0
	} else {
		padLen = 16 - len(theValue)%16
	}
	paddedValue := append(theValue, make([]byte, padLen)...)
	if !reflect.DeepEqual(rebuiltAVP.GetOctets(), paddedValue) {
		t.Errorf("value does not match after unmarshalling. Got %v", rebuiltAVP.GetOctets())
	}
}

func TestSaltedIntegerAVP(t *testing.T) {

	var value = 1

	// Create 0
	avp, err := NewRadiusAVP("Unisphere-LI-Action", value)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
		return
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)

	if rebuiltAVP.GetInt() != int64(value) {
		t.Errorf("error recovering value from salted integer. Got %d instead of %v", rebuiltAVP.GetInt(), value)
		return
	}
}

func TestSaltedAddressAVP(t *testing.T) {

	var value = "1.1.1.1"

	// Create
	avp, err := NewRadiusAVP("Unisphere-Med-Ip-Address", value)
	if err != nil {
		t.Fatalf("error creating avp: %v", err)
		return
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.ToBytes(authenticator, secret)
	rebuiltAVP, _, _ := RadiusAVPFromBytes(binaryAVP, authenticator, secret)

	if rebuiltAVP.GetString() != value {
		t.Errorf("error recovering value from salted address. Got %s instead of %s", rebuiltAVP.GetString(), value)
		return
	}
}

func TestEncryptFunction(t *testing.T) {
	authenticator := BuildRandomAuthenticator()
	password := "__! $? this is the - ñ long password  '            7887"

	cipherText := encrypt1([]byte(password), authenticator, "mysecret", nil)

	clearText := decrypt1(cipherText, authenticator, "mysecret", nil)
	if string(bytes.Trim(clearText, "\x00")) != password {
		t.Errorf("cleartext does not match the original one")
	}
}

// ///////////////////////////////////////////////////////////////////////////////////
func TestAccessRequest(t *testing.T) {

	theUserName := "MyUserName"
	thePassword := "pwd"
	theTunnelPassword := "the password for the tunnel:1"

	request := NewRadiusRequest(ACCESS_REQUEST)
	request.Add("User-Name", theUserName)
	request.Add("User-Password", thePassword)
	request.Add("Tunnel-Password", theTunnelPassword)

	// Serialize
	packetBytes, err := request.ToBytes(secret, 0)
	if err != nil {
		t.Fatalf("could not serialize packet: %s", err)
	}

	// Unserialize
	recoveredPacket, err := NewRadiusPacketFromBytes(packetBytes, secret, Zero_authenticator)
	if err != nil {
		t.Fatalf("could not unserialize packet: %s", err)
	}

	if userName := recoveredPacket.GetStringAVP("User-Name"); userName != theUserName {
		t.Errorf("attribute does not match <%s>", userName)
	}

	if password := recoveredPacket.GetStringAVP("User-Password"); password != thePassword {
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

func TestAccountingRequest(t *testing.T) {

	theClass := "MyClass"

	request := NewRadiusRequest(ACCOUNTING_REQUEST)
	request.Add("Class", theClass)

	// Serialize
	packetBytes, err := request.ToBytes(secret, 0)
	if err != nil {
		t.Fatalf("could not serialize packet: %s", err)
	}

	// Unserialize
	recoveredPacket, err := NewRadiusPacketFromBytes(packetBytes, secret, Zero_authenticator)
	if err != nil {
		t.Fatalf("could not unserialize packet: %s", err)
	}

	if class := recoveredPacket.GetStringAVP("Class"); class != theClass {
		t.Errorf("attribute does not match <%s>", class)
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

func TestJSONAndCopyPacket(t *testing.T) {

	jsonPacket := `{
				"Code": 1,
				"AVPs":[
					{"Igor-OctetsAttribute": "0102030405060708090a0b"},
					{"Igor-StringAttribute": "stringvalue"},
					{"Igor-IntegerAttribute": "Zero"},
					{"Igor-IntegerAttribute": "1"},
					{"Igor-IntegerAttribute": 1},
					{"Igor-AddressAttribute": "127.0.0.1"},
					{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
					{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
					{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
					{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
					{"Igor-Integer64Attribute": 999999999999},
					{"Igor-TaggedStringAttribute": "myString:1"},
					{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
					{"User-Name":"MyUserName"},
					{"Tunnel-Password":"secretpassword:2"}
				]
			}`

	// Read JSON to Radius Packet
	rp := RadiusPacket{}
	if err := json.Unmarshal([]byte(jsonPacket), &rp); err != nil {
		t.Fatalf("unmarshal error for radius packet: %s", err)
	}

	// Check attributes
	taggedString := rp.GetTaggedStringAVP("Igor-TaggedStringAttribute")
	if taggedString != "myString:1" {
		t.Fatalf("bad tagged stringattribute %s", taggedString)
	}
	timeAttribute := rp.GetDateAVP("Igor-TimeAttribute")
	if timeAttribute.Hour() != 3 {
		t.Fatalf("bad time attribute %v", timeAttribute)
	}

	// Write RadiusPacket message as JSON
	jsonPacketNew, _ := json.Marshal(&rp)
	if !strings.Contains(string(jsonPacketNew), "1966-11-26T03:34:08 UTC") || !strings.Contains(string(jsonPacketNew), "Zero") {
		t.Fatalf("marshalled json does not contain the expected attributes: %s", string(jsonPacketNew))
	}

	// Copy with positive filter
	positivePacket := rp.Copy([]string{"User-Name", "Igor-SaltedOctetsAttribute"}, nil)
	if positivePacket.GetStringAVP("Igor-OctetsAttribute") != "" {
		t.Fatalf("unexpected attribute after positive filtering")
	}
	if positivePacket.GetStringAVP("Igor-SaltedOctetsAttribute") != "1122aabbccdd" {
		t.Fatalf("missing attribute after positive filtering")
	}

	// Copy with negative filter
	negativePacket := rp.Copy(nil, []string{"Igor-StringAttribute"})
	if negativePacket.GetStringAVP("Igor-StringAttribute") != "" {
		t.Fatalf("unexpected attribute after negative filtering")
	}
	if negativePacket.GetStringAVP("Igor-SaltedOctetsAttribute") != "1122aabbccdd" {
		t.Fatalf("missing attribute after positive filtering")
	}
}

func TestLongAttribute(t *testing.T) {
	longOctets := "0000000000000000100000000000000020000000000000003000000000000000400000000000000050000000000000006000000000000000700000000000000080000000000000009000000000000000a000000000000000b000000000000000c000000000000000d000000000000000e000000000000000f000000000000000"
	longOctets = longOctets + longOctets + "aa"

	packet := NewRadiusRequest(ACCESS_REQUEST).Add("Igor-LongMessage", longOctets)
	packet.Add("User-Name", "theUserName")

	// Serialize
	packetBytes, err := packet.ToBytes(secret, 0)
	if err != nil {
		t.Fatalf("could not serialize packet: %s", err)
	}

	// Unserialize
	recoveredPacket, err := NewRadiusPacketFromBytes(packetBytes, secret, Zero_authenticator)
	if err != nil {
		t.Fatalf("could not unserialize packet: %s", err)
	}
	if recoveredPacket.GetStringAVP("Igor-LongMessage") != longOctets {
		t.Fatalf("bad recovered LongMessage %s", recoveredPacket.GetStringAVP("Igor-LongMessage"))
	}
}

func TestCiscoAVPair(t *testing.T) {
	packet := NewRadiusRequest(ACCESS_REQUEST).
		Add("Cisco-AVPair", "subscriber:sa=internet(shape-rate=1000)").
		Add("Cisco-AVPair", "ip:qos-policy-in=add-class(sub,(class-default),police(512,96,512,192,transmit,transmit,drop))")

	if packet.GetCiscoAVPair("subscriber:sa") != "internet(shape-rate=1000)" {
		t.Fatalf("bad Cisco AVPair <%s>", packet.GetCiscoAVPair("subscriber:sa"))
	}
}

func TestAddIfNotPresent(t *testing.T) {
	jsonPacket := `{
		"Code": 1,
		"AVPs":[
			{"Igor-OctetsAttribute": "0102030405060708090a0b"},
			{"Igor-StringAttribute": "stringvalue"},
			{"Igor-IntegerAttribute": "Zero"},
			{"Igor-IntegerAttribute": "1"},
			{"Igor-IntegerAttribute": 1},
			{"Igor-AddressAttribute": "127.0.0.1"},
			{"Igor-TimeAttribute": "1966-11-26T03:34:08 UTC"},
			{"Igor-IPv6AddressAttribute": "bebe:cafe::0"},
			{"Igor-IPv6PrefixAttribute": "bebe:cafe:cccc::0/64"},
			{"Igor-InterfaceIdAttribute": "00aabbccddeeff11"},
			{"Igor-Integer64Attribute": 999999999999},
			{"Igor-TaggedStringAttribute": "myString:1"},
			{"Igor-SaltedOctetsAttribute": "1122aabbccdd"},
			{"User-Name":"MyUserName"},
			{"Tunnel-Password":"secretpassword:2"}
		]
	}`

	// Read JSON to Radius Packet
	rp := RadiusPacket{}
	if err := json.Unmarshal([]byte(jsonPacket), &rp); err != nil {
		t.Fatalf("unmarshal error for radius packet: %s", err)
	}

	// AddIfNotPresent
	avp1, _ := NewRadiusAVP("Class", "the-class")
	avp2, _ := NewRadiusAVP("Igor-StringAttribute", "tried-to-change")
	avps := []RadiusAVP{*avp1, *avp2}
	rp.AddIfNotPresentAVPs(avps)
	if rp.GetStringAVP("Class") != "the-class" {
		t.Errorf("added class name not found")
	}
	if len(rp.GetAllAVP("Igor-StringAttribute")) != 1 {
		t.Errorf("number of Igor-StringAttribute should be 1")
	}
	if rp.GetStringAVP("Igor-StringAttribute") != "stringvalue" {
		t.Errorf("added already present attribute")
	}
}
