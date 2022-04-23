package diamcodec

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

// Initializer of the test suite.
func TestMain(m *testing.M) {

	// Initialize logging
	config.SetupLogger()

	// Initialize the Config Object as done in main.go
	bootstrapFile := "resources/searchRules.json"
	instanceName := "unitTestInstance"
	config.Config.Init(bootstrapFile, instanceName)

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
	rebuiltAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != fmt.Sprintf("%x", password) {
		t.Errorf("Octets AVP not properly encoded after unmarshalling. Got %s", rebuiltAVP.GetString())
	}
	if !reflect.DeepEqual(rebuiltAVP.GetOctets(), []byte(password)) {
		t.Errorf("Octets AVP not properly encoded after unmarshalling. Got %v instead of %v", rebuiltAVP.GetOctets(), []byte(password))
	}
}

func TestUTF8StringAVP(t *testing.T) {

	var theString = "%Hola España. 'Quiero €"

	// Create avp
	avp, err := NewAVP("User-Name", theString)
	if err != nil {
		t.Errorf("error creating UTFString AVP %v", err)
		return
	}
	if avp.GetString() != theString {
		t.Errorf("UTF8String AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != theString {
		t.Errorf("UTF8String AVP not properly encoded after unmarshalling. Got %s", rebuiltAVP.GetString())
	}
}

func TestInt32AVP(t *testing.T) {

	var theInt int32 = -65535*16384 - 1000 // 2^31 - 1000

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myInteger32", theInt)
	if err != nil {
		t.Errorf("error creating Int32 AVP %v", err)
		return
	}
	if avp.GetInt() != int64(theInt) {
		t.Errorf("Int32 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != fmt.Sprint(theInt) {
		t.Errorf("Integer32 AVP not properly encoded after unmarshalling (string value). Got %s", rebuiltAVP.GetString())
	}
	if rebuiltAVP.GetInt() != int64(theInt) {
		t.Errorf("Integer32 AVP not properly encoded after unmarshalling (long value). Got %d", rebuiltAVP.GetInt())
	}
	if rebuiltAVP.GetInt() >= 0 {
		t.Errorf("Integer32 should be negative. Got %d", rebuiltAVP.GetInt())
	}
}

func TestInt64AVP(t *testing.T) {

	var theInt int64 = -65535*65535*65534*16384 - 999 // - 2 ^ 62 - 999
	// Create avp
	avp, err := NewAVP("franciscocardosogil-myInteger64", theInt)
	if err != nil {
		t.Errorf("error creating Int64 AVP %v", err)
		return
	}
	if avp.GetInt() != int64(theInt) {
		t.Errorf("Int64 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != fmt.Sprint(theInt) {
		t.Errorf("Integer64 AVP not properly encoded after unmarshalling (string value). Got %s", rebuiltAVP.GetString())
	}
	if rebuiltAVP.GetInt() != int64(theInt) {
		t.Errorf("Integer64 AVP not properly encoded after unmarshalling (long value). Got %d", rebuiltAVP.GetInt())
	}
	if rebuiltAVP.GetInt() >= 0 {

		t.Errorf("Integer64 should be negative. Got %d", rebuiltAVP.GetInt())
	}
}

func TestUnsignedInt32AVP(t *testing.T) {

	var theInt uint32 = 65535 * 40001

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myUnsigned32", int64(theInt))
	if err != nil {
		t.Errorf("error creating UInt32 AVP %v", err)
		return
	}
	if avp.GetInt() != int64(theInt) {
		t.Errorf("UInt32 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != fmt.Sprint(theInt) {
		t.Errorf("UnsignedInteger32 AVP not properly encoded after unmarshalling (string value). Got %s", rebuiltAVP.GetString())
	}
	if rebuiltAVP.GetInt() != int64(theInt) {
		t.Errorf("UnsignedInteger32 AVP not properly encoded after unmarshalling (long value). Got %d", rebuiltAVP.GetInt())
	}
	if rebuiltAVP.GetInt() < 0 {
		t.Errorf("Unsigned Integer32 should be positive. Got %d", rebuiltAVP.GetInt())
	}
}

func TestUnsignedInt64AVP(t *testing.T) {

	// Due to a limitaton of the implementation, it is inernally stored as a signed int64
	var theInt int64 = 65535 * 65535 * 65535 * 16001

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myUnsigned64", theInt)
	if err != nil {
		t.Errorf("error creating UInt64 AVP %v", err)
		return
	}
	if avp.GetInt() != int64(theInt) {
		t.Errorf("Unsigned Int64 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	rebuiltAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if rebuiltAVP.GetString() != fmt.Sprint(theInt) {
		t.Errorf("Unsigned Integer64 AVP not properly encoded after unmarshalling (string value). Got %s", rebuiltAVP.GetString())
	}
	if rebuiltAVP.GetInt() != int64(theInt) {
		t.Errorf("Unsigned Integer64 AVP not properly encoded after unmarshalling (long value). Got %d", rebuiltAVP.GetInt())
	}
	if rebuiltAVP.GetInt() < 0 {
		t.Errorf("Unsigned Integer64 should be positive. Got %d", rebuiltAVP.GetInt())
	}
}

func TestFloat32AVP(t *testing.T) {

	var theFloat float32 = 6.03e23

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myFloat32", theFloat)
	if err != nil {
		t.Errorf("error creating Float32 AVP %v", err)
		return
	}
	if avp.GetFloat() != float64(theFloat) {
		t.Errorf("Float32 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != fmt.Sprintf("%f", theFloat) {
		t.Errorf("Float32 AVP not properly encoded after unmarshalling (string value). Got %s", recoveredAVP.GetString())
	}
	if recoveredAVP.GetFloat() != float64(theFloat) {
		t.Errorf("Float32 AVP not properly encoded after unmarshalling (long value). Got %f", recoveredAVP.GetFloat())
	}
}

func TestFloat64AVP(t *testing.T) {

	var theFloat float64 = 6.03e23

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myFloat64", float64(theFloat))
	if err != nil {
		t.Errorf("error creating Float64 AVP %v", err)
		return
	}
	if avp.GetFloat() != float64(theFloat) {
		t.Errorf("Float64 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != fmt.Sprintf("%f", theFloat) {
		t.Errorf("Float64 AVP not properly encoded after unmarshalling (string value). Got %s", recoveredAVP.GetString())
	}
	if recoveredAVP.GetFloat() != float64(theFloat) {
		t.Errorf("Float64 AVP not properly encoded after unmarshalling (long value). Got %f", recoveredAVP.GetFloat())
	}
}

func TestAddressAVP(t *testing.T) {

	var ipv4Address = "1.2.3.4"
	var ipv6Address = "bebe:cafe::0"

	// Using strings as values

	// IPv4
	// Create avp
	avp, err := NewAVP("franciscocardosogil-myAddress", ipv4Address)
	if err != nil {
		t.Errorf("error creating IPv4 Address AVP: %v", err)
		return
	}
	if avp.GetString() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP not properly encoded after unmarshalling (string value). Got %s %s", recoveredAVP.GetString(), net.ParseIP(ipv4Address).String())
	}

	// IPv6
	// Create avp
	avp, err = NewAVP("franciscocardosogil-myAddress", ipv6Address)
	if err != nil {
		t.Errorf("error creating IPv6 Address AVP: %v", err)
	}
	if avp.GetString() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ = avp.MarshalBinary()
	recoveredAVP, _, _ = DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP not properly encoded after unmarshalling (string value). Got %s %s", recoveredAVP.GetString(), net.ParseIP(ipv6Address).String())
	}

	// Using IP addresses as value
	avp, _ = NewAVP("franciscocardosogil-myAddress", net.ParseIP(ipv4Address))
	if avp.GetString() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value (created as ipaddr) %s %s", avp.GetString(), net.ParseIP(ipv4Address).String())
	}

	avp, _ = NewAVP("franciscocardosogil-myAddress", net.ParseIP(ipv6Address))
	if avp.GetString() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value (created as ipaddr) %s %s", avp.GetString(), net.ParseIP(ipv6Address).String())
	}
}

func TestIPv4Address(t *testing.T) {

	var ipv4Address = "1.2.3.4"

	// Create avp from string
	avp, err := NewAVP("franciscocardosogil-myIPv4Address", ipv4Address)
	if err != nil {
		t.Errorf("error creating IPv4 Address AVP %v", err)
		return
	}
	if avp.GetString() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP not properly encoded after unmarshalling (string value). Got %s", recoveredAVP.GetString())
	}

	// Create avp from address
	avp, _ = NewAVP("franciscocardosogil-myIPv4Address", net.ParseIP(ipv4Address))
	if avp.GetIPAddress().String() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value (created as ipaddr) %s", avp.GetIPAddress())
	}
}

func TestIPv6Address(t *testing.T) {
	var ipv6Address = "bebe:cafe::0"

	// Create avp from string
	avp, err := NewAVP("franciscocardosogil-myIPv6Address", ipv6Address)
	if err != nil {
		t.Errorf("error creating IPv6 Address AVP %v", err)
		return
	}
	if avp.GetString() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP not properly encoded after unmarshalling (string value). Got %s", recoveredAVP.GetString())
	}

	// Create avp from IP address
	avp, _ = NewAVP("franciscocardosogil-myIPv6Address", net.ParseIP(ipv6Address))
	if avp.GetString() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value (created as ipaddr) %s", avp.GetString())
	}
}

func TestTimeAVP(t *testing.T) {
	var theTime, _ = time.Parse("02/01/2006 15:04:05 UTC", "26/11/1966 03:21:54 UTC")
	var theStringTime = "1966-11-26T03:21:54 UTC"

	// Create avp from string
	avp, err := NewAVP("franciscocardosogil-myTime", theStringTime)
	if err != nil {
		t.Errorf("error creating Time Address AVP %v", err)
		return
	}
	if avp.GetDate() != theTime {
		t.Errorf("Time AVP does not match value")
	}
}

func TestDiamIdentAVP(t *testing.T) {

	var theString = "domain.name"

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myDiameterIdentity", theString)
	if err != nil {
		t.Errorf("error creating Diameter Identity AVP %v", err)
		return
	}
	if avp.GetString() != theString {
		t.Errorf("Diamident AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != theString {
		t.Errorf("Diameter Identity AVP not properly encoded after unmarshalling. Got %s", recoveredAVP.GetString())
	}
}

func TestDiamURIAVP(t *testing.T) {

	var theString = "domain.name"

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myDiameterURI", theString)
	if err != nil {
		t.Errorf("error creating Diameter URI AVP %v", err)
		return
	}
	if avp.GetString() != theString {
		t.Errorf("Diamident AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != theString {
		t.Errorf("Diameter URI AVP not properly encoded after unmarshalling. Got %s", recoveredAVP.GetString())
	}
}

func TestIPFilterRuleIAVP(t *testing.T) {

	var theString = "deny 1.2.3.4"

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myIPFilterRule", theString)
	if err != nil {
		t.Errorf("error creating IP Filter Rule AVP %v", err)
		return
	}
	if avp.GetString() != theString {
		t.Errorf("IP Filter Rule AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != theString {
		t.Errorf("IP Filter Rule AVP not properly encoded after unmarshalling. Got %s", recoveredAVP.GetString())
	}
}

func TestIPv6PrefixAVP(t *testing.T) {

	var thePrefix = "bebe:cafe::/16"

	// Create avp
	avp, err := NewAVP("franciscocardosogil-myIPv6Prefix", thePrefix)
	if err != nil {
		t.Errorf("error creating IPv6 prefix AVP %v", err)
		return
	}
	if avp.GetString() != thePrefix {
		t.Errorf("IPv6 Prefix AVP does not match value")
	}

	// Serialize and unserialize
	binaryAVP, _ := avp.MarshalBinary()
	recoveredAVP, _, _ := DiameterAVPFromBytes(binaryAVP)
	if recoveredAVP.GetString() != thePrefix {
		t.Errorf("IPv6 Prefix AVP not properly encoded after unmarshalling. Got %s", recoveredAVP.GetString())
	}
}

func TestEnumeratedAVP(t *testing.T) {

	var theString = "zero"
	var theNumber int64 = 0

	avp, err := NewAVP("franciscocardosogil-myEnumerated", "zero")
	if err != nil {
		t.Errorf("error creating Enumerated AVP: %v", err)
		return
	}
	if avp.GetString() != theString {
		t.Errorf("Enumerated AVP does not match string value")
	}
	if avp.GetInt() != theNumber {
		t.Errorf("Enumerated AVP does not match number value")
	}

	avp, err = NewAVP("franciscocardosogil-myEnumerated", theNumber)
	if err != nil {
		t.Errorf("error creating Enumerated AVP: %v", err)
		return
	}
	if avp.GetString() != theString {
		t.Errorf("Enumerated AVP does not match string value")
	}
	if avp.GetInt() != theNumber {
		t.Errorf("Enumerated AVP does not match number value")
	}
}

func TestGroupedAVP(t *testing.T) {

	var theInt int64 = 99
	var theString = "theString"

	// Create grouped AVP
	avpl0, _ := NewAVP("franciscocardosogil-myGroupedInGrouped", nil)
	avpl1, _ := NewAVP("franciscocardosogil-myGrouped", nil)

	avpInt, _ := NewAVP("franciscocardosogil-myInteger32", theInt)
	avpString, _ := NewAVP("franciscocardosogil-myString", theString)

	avpl1.AddAVP(*avpInt).AddAVP(*avpString)
	avpl0.AddAVP(*avpl1)

	// Serialize and unserialize
	binaryAVP, _ := avpl0.MarshalBinary()
	recoveredAVPl0, _, _ := DiameterAVPFromBytes(binaryAVP)

	// Navigate to the values
	recoveredAVPl1 := recoveredAVPl0.GetAllAVP("franciscocardosogil-myGrouped")[0]
	newInt, _ := recoveredAVPl1.GetAVP("franciscocardosogil-myInteger32")
	if newInt.GetInt() != theInt {
		t.Error("Integer value does not match or not found in Group")
	}
	newString, _ := recoveredAVPl1.GetAVP("franciscocardosogil-myString")
	if newString.GetString() != theString {
		t.Error("String value does not match or not found in Group")
	}

	// Non existing AVP
	_, err := recoveredAVPl1.GetAVP("non-existing")
	if err == nil {
		t.Error("No error when trying to find a non existing AVP")
	}

	// Printed avp
	var targetString = "{franciscocardosogil-myGrouped={franciscocardosogil-myInteger32=99,franciscocardosogil-myString=theString}}"
	stringRepresentation := recoveredAVPl0.GetString()
	if stringRepresentation != targetString {
		t.Errorf("Grouped string representation does not match %s", stringRepresentation)
	}
}

func TestSerializationError(t *testing.T) {

	// Generate an AVP
	avp, err := NewAVP("franciscocardosogil-myOctetString", "0A0B0C0c765654")
	theBytes, _ := avp.MarshalBinary()

	if err != nil {
		t.Errorf("error creating octectstring from string: %s", err)
		return
	}

	// Change the vendorId to something not existing in the dict
	var theBytesUnknown []byte
	theBytesUnknown = append(theBytesUnknown, theBytes...)
	copy(theBytesUnknown[8:12], []byte{11, 12, 13, 14})

	// Simulate we read an AVP not in the dictionary
	// It should create an AVP with name UNKNOWN
	newavp, _, _ := DiameterAVPFromBytes(theBytesUnknown)
	if newavp.VendorId != 11*256*256*256+12*256*256+13*256+14 {
		t.Errorf("unknown vendor Id was not unmarshalled")
	}
	if newavp.DictItem.Name != "UNKNOWN" {
		t.Errorf("unknown AVP not named UNKNOWN")
	}

	// We should be able to serialize the unknown AVP
	// The vendorId should be the same
	otherBytes, marshalError := newavp.MarshalBinary()
	if marshalError != nil {
		t.Errorf("error serializing unknown avp: %s", marshalError)
	}
	if !reflect.DeepEqual([]byte{11, 12, 13, 14}, otherBytes[8:12]) {
		t.Errorf("error serializing unknown avp. Vendor Id does not match: %s", marshalError)
	}

	// Force unmarshalling error. Size is some big number

	copy(theBytesUnknown[5:8], []byte{100, 100, 100})
	_, _, e := DiameterAVPFromBytes(theBytesUnknown)
	if e == nil {
		t.Error("bad bytes should have reported error")
	}

}

func TestJSON(t *testing.T) {

	var javp = `{
		"franciscocardosogil-myTestAllGrouped": [
			{"franciscocardosogil-myOctetString": "0102030405060708090a0b"},
			{"franciscocardosogil-myInteger32": -99},
			{"franciscocardosogil-myInteger64": -99},
			{"franciscocardosogil-myUnsigned32": 99},
			{"franciscocardosogil-myUnsigned64": 99},
			{"franciscocardosogil-myFloat32": 99.9},
			{"franciscocardosogil-myFloat64": 99.9},
			{"franciscocardosogil-myAddress": "1.2.3.4"},
			{"franciscocardosogil-myTime": "1966-11-26T03:34:08 UTC"},
			{"franciscocardosogil-myString": "Hello, world!"},
			{"franciscocardosogil-myDiameterIdentity": "Diameter@identity"},
			{"franciscocardosogil-myDiameterURI": "Diameter@URI"},
			{"franciscocardosogil-myIPFilterRule": "allow all"},
			{"franciscocardosogil-myIPv4Address": "4.5.6.7"},
			{"franciscocardosogil-myIPv6Address": "bebe:cafe::0"},
			{"franciscocardosogil-myIPv6Prefix": "bebe:cafe::0/128"},
			{"franciscocardosogil-myEnumerated": "two"}
		]
	}`

	// Read JSON to AVP
	var avp DiameterAVP
	err := json.Unmarshal([]byte(javp), &avp)
	if err != nil {
		t.Errorf("unmarshal error for avp: %s", err)
	}
	// Check the contents of the unmarshalled avp
	if avp.Name != "franciscocardosogil-myTestAllGrouped" {
		t.Errorf("unmarshalled avp has the wrong name: %s", avp.Name)
	}
	if v, _ := avp.GetAVP("franciscocardosogil-myEnumerated"); v.GetInt() != 2 {
		t.Errorf("unmarshalled avp has the wrong name: %s", avp.Name)
	}
	v, _ := avp.GetAVP("franciscocardosogil-myTime")
	vv, _ := time.Parse(timeFormatString, "1966-11-26T03:34:08 UTC")
	if v.GetDate() != vv {
		t.Errorf("unmarshalled avp has the wrong date value: %s", v.String())
	}

	// Marshal again
	jNewAVP, _ := json.Marshal(&avp)
	if !strings.Contains(string(jNewAVP), "bebe:cafe::0/128") {
		t.Errorf("part of the expected JSON content was not found")
	}

	/*
		var jBytes bytes.Buffer
		if err := json.Indent(&jBytes, []byte(jRecovered), "", "    "); err != nil {
			t.Errorf("prettyprint error %s", err)
		}

		fmt.Println(jBytes.String())
		fmt.Println(avp.String())
	*/
}

func TestDiameterMessage(t *testing.T) {

	diameterMessage, err := NewDiameterRequest("TestApplication", "TestRequest")
	if err != nil {
		t.Errorf("could not create diameter request for application TestAppliciaton and command TestRequest")
		return
	}
	sessionIdAVP, _ := NewAVP("Session-Id", "my-session-id")
	originHostAVP, _ := NewAVP("Origin-Host", "server.igor")
	originRealmAVP, _ := NewAVP("Origin-Realm", "igor")
	destinationHostAVP, _ := NewAVP("Destination-Host", "server.igor")
	destinationRealmAVP, _ := NewAVP("Destination-Realm", "igor")
	groupedInGroupedAVP, _ := NewAVP("franciscocardosogil-myGroupedInGrouped", nil)
	groupedAVP, _ := NewAVP("franciscocardosogil-myGrouped", nil)
	intAVP, _ := NewAVP("franciscocardosogil-myInteger32", 1)
	stringAVP, _ := NewAVP("franciscocardosogil-myString", "hello")
	groupedAVP.AddAVP(*intAVP)
	groupedAVP.AddAVP(*stringAVP)
	groupedInGroupedAVP.AddAVP(*groupedAVP)
	groupedInGroupedAVP.AddAVP(*intAVP)
	groupedInGroupedAVP.AddAVP(*stringAVP)

	diameterMessage.AddAVP(sessionIdAVP)
	diameterMessage.AddAVP(originHostAVP)
	diameterMessage.AddAVP(originRealmAVP)
	diameterMessage.AddAVP(destinationHostAVP)
	diameterMessage.AddAVP(destinationRealmAVP)
	diameterMessage.AddAVP(groupedInGroupedAVP)

	diameterMessage.Add("franciscocardosogil-myUnsigned32", 8)
	diameterMessage.Add("franciscocardosogil-myUnsigned32", 9)

	// Serialize
	theBytes, err := diameterMessage.MarshalBinary()
	if err != nil {
		t.Errorf("could not serialize diameter message %s", err)
		return
	}

	// Unserialize
	recoveredMessage, _, err := DiameterMessageFromBytes(theBytes)
	if err != nil {
		t.Errorf("could not unserialize diameter message %s", err)
		return
	}

	// Get and check the values of simple AVP
	unsignedAVPs := recoveredMessage.GetAllAVP("franciscocardosogil-myUnsigned32")
	if len(unsignedAVPs) != 2 {
		t.Errorf("did not get two unsigned32 avps in Diameter message")
	}
	for _, avp := range unsignedAVPs {
		value := avp.GetInt()
		if value != 8 && value != 9 {
			t.Errorf("incorrect value")
		}
	}

	// Delete the avp
	recoveredMessage.DeleteAllAVP("franciscocardosogil-myUnsigned32")
	unsignedAVPs = recoveredMessage.GetAllAVP("franciscocardosogil-myUnsigned32")
	if len(unsignedAVPs) != 0 {
		t.Errorf("avp still there after being deleted")
	}

	// Get and check the value of a grouped AVP
	gig, err := recoveredMessage.GetAVP("franciscocardosogil-myGroupedInGrouped")
	if err != nil {
		t.Errorf("could not retrieve groupedingrouped avp: %s", err)
		return
	}
	g, err := gig.GetAVP("franciscocardosogil-myGrouped")
	if err != nil {
		t.Errorf("could not retrieve grouped avp: %s", err)
		return
	}
	s, err := g.GetAVP("franciscocardosogil-myString")
	if err != nil {
		t.Errorf("could not retrieve string avp: %s", err)
		return
	}
	if s.GetString() != "hello" {
		t.Errorf("got incorrect value for string avp: %s instead of <hello>", err)
	}

	// Generate reply message
	replyMessage := NewDiameterAnswer(&recoveredMessage)
	if replyMessage.IsRequest {
		t.Errorf("reply message is a request")
	}

	// TODO:
	// Cuando se hace return de un item de un slice ¿Es una copia?
	// Cuando se añade un AVP ¿es una copia o se puede modificar el orgiginal?
}

func TestDiameterMessageAllAttributeTypes(t *testing.T) {

	jDiameterMessage := `
	{
		"IsRequest": true,
		"IsProxyable": false,
		"IsError": false,
		"IsRetransmission": false,
		"CommandCode": 2000,
		"ApplicationId": 1000,
		"avps":[
			{
			  "franciscocardosogil-myTestAllGrouped": [
  				{"franciscocardosogil-myOctetString": "0102030405060708090a0b"},
  				{"franciscocardosogil-myInteger32": -99},
  				{"franciscocardosogil-myInteger64": -99},
  				{"franciscocardosogil-myUnsigned32": 99},
  				{"franciscocardosogil-myUnsigned64": 99},
  				{"franciscocardosogil-myFloat32": 99.9},
  				{"franciscocardosogil-myFloat64": 99.9},
  				{"franciscocardosogil-myAddress": "1.2.3.4"},
  				{"franciscocardosogil-myTime": "1966-11-26T03:34:08 UTC"},
  				{"franciscocardosogil-myString": "Hello, world!"},
  				{"franciscocardosogil-myDiameterIdentity": "Diameter@identity"},
  				{"franciscocardosogil-myDiameterURI": "Diameter@URI"},
  				{"franciscocardosogil-myIPFilterRule": "allow all"},
  				{"franciscocardosogil-myIPv4Address": "4.5.6.7"},
  				{"franciscocardosogil-myIPv6Address": "bebe:cafe::0"},
  				{"franciscocardosogil-myIPv6Prefix": "bebe:cafe::0/128"},
  				{"franciscocardosogil-myEnumerated": "two"}
			  ]
			}
		]
	}
	`

	// Read JSON to DiameterMessage
	var diameterMessage DiameterMessage
	err := json.Unmarshal([]byte(jDiameterMessage), &diameterMessage)
	if err != nil {
		t.Errorf("unmarshal error for diameter message: %s", err)
	}
	diameterMessage.Tidy()

	// Write message to buffer
	messageBytes, error := diameterMessage.MarshalBinary()
	if error != nil {
		t.Fatal("Marshal error")
	}

	// Recover message from buffer
	recoveredMessage := DiameterMessage{}
	_, err = recoveredMessage.ReadFrom(bytes.NewReader(messageBytes))
	if err != nil {
		t.Fatalf("Error recovering DiameterMessage from bytes: %s", err)
	}

	if recoveredMessage.GetStringAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myAddress") != "1.2.3.4" {
		t.Errorf("Error recovering IP address. Got <%s> instead of 1.2.3.4", recoveredMessage.GetStringAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myAddress"))
	}
	if recoveredMessage.GetStringAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myEnumerated") != "two" {
		t.Errorf("Error recovering Enumerated. Got <%s> instead of <two>", recoveredMessage.GetStringAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myEnumerated"))
	}
	targetTime, _ := time.Parse("2006-01-02T15:04:05 UTC", "1966-11-26T03:34:08 UTC")
	if recoveredMessage.GetDateAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myTime") != targetTime {
		t.Errorf("Error recovering date. Got <%v> instead of <1966-11-26T03:34:08 UTC>", recoveredMessage.GetDateAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myTime"))
	}
	if recoveredMessage.GetIntAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myInteger32") != -99 {
		t.Errorf("Error recovering int. Got <%d> instead of -99", recoveredMessage.GetIntAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myInteger32"))
	}
	targetIPAddress := net.ParseIP("4.5.6.7")
	if !recoveredMessage.GetIPAddressAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myIPv4Address").Equal(targetIPAddress) {
		t.Errorf("Error recovering IPv4Address. Got <%v> instead of <4.5.6.7>", recoveredMessage.GetIPAddressAVP("franciscocardosogil-myTestAllGrouped.franciscocardosogil-myIPv4Address"))
	}
}

func TestDiameterMessageJSON(t *testing.T) {
	jDiameterMessage := `
	{
		"IsRequest": true,
		"IsProxyable": false,
		"IsError": false,
		"IsRetransmission": false,
		"CommandCode": 2000,
		"ApplicationId": 1000,
		"avps":[
			{
			  "franciscocardosogil-myTestAllGrouped": [
  				{"franciscocardosogil-myOctetString": "0102030405060708090a0b"},
  				{"franciscocardosogil-myInteger32": -99},
  				{"franciscocardosogil-myInteger64": -99},
  				{"franciscocardosogil-myUnsigned32": 99},
  				{"franciscocardosogil-myUnsigned64": 99},
  				{"franciscocardosogil-myFloat32": 99.9},
  				{"franciscocardosogil-myFloat64": 99.9},
  				{"franciscocardosogil-myAddress": "1.2.3.4"},
  				{"franciscocardosogil-myTime": "1966-11-26T03:34:08 UTC"},
  				{"franciscocardosogil-myString": "Hello, world!"},
  				{"franciscocardosogil-myDiameterIdentity": "Diameter@identity"},
  				{"franciscocardosogil-myDiameterURI": "Diameter@URI"},
  				{"franciscocardosogil-myIPFilterRule": "allow all"},
  				{"franciscocardosogil-myIPv4Address": "4.5.6.7"},
  				{"franciscocardosogil-myIPv6Address": "bebe:cafe::0"},
  				{"franciscocardosogil-myIPv6Prefix": "bebe:cafe::0/128"},
  				{"franciscocardosogil-myEnumerated": "two"}
			  ]
			}
		]
	}
	`

	// Read JSON to DiameterMessage
	var diameterMessage DiameterMessage
	err := json.Unmarshal([]byte(jDiameterMessage), &diameterMessage)
	if err != nil {
		t.Errorf("unmarshal error for diameter message: %s", err)
	}
	diameterMessage.Tidy()

	// Write Diameter message as JSON
	jNewDiameterMessage, _ := json.Marshal(&diameterMessage)
	if !strings.Contains(string(jNewDiameterMessage), "TestApplication") || !strings.Contains(string(jNewDiameterMessage), "TestRequest") {
		t.Errorf("marshalled json does not contain the tidied attributes")
	}

	var jBytes bytes.Buffer
	if err := json.Indent(&jBytes, []byte(jNewDiameterMessage), "", "    "); err != nil {
		t.Errorf("prettyprint error %s", err)
	}

	// Uncoment this to see the result
	// fmt.Println(jBytes.String())
}
