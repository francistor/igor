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
	instanceName := "testInstance"
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myInteger32", theInt)
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myInteger64", theInt)
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myUnsigned32", int64(theInt))
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myUnsigned64", theInt)
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myFloat32", theFloat)
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myFloat64", float64(theFloat))
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myAddress", ipv4Address)
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
	avp, err = NewAVP("francisco.cardosogil@gmail.com-myAddress", ipv6Address)
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
	avp, _ = NewAVP("francisco.cardosogil@gmail.com-myAddress", net.ParseIP(ipv4Address))
	if avp.GetString() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value (created as ipaddr) %s %s", avp.GetString(), net.ParseIP(ipv4Address).String())
	}

	avp, _ = NewAVP("francisco.cardosogil@gmail.com-myAddress", net.ParseIP(ipv6Address))
	if avp.GetString() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value (created as ipaddr) %s %s", avp.GetString(), net.ParseIP(ipv6Address).String())
	}
}

func TestIPv4Address(t *testing.T) {

	var ipv4Address = "1.2.3.4"

	// Create avp from string
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myIPv4Address", ipv4Address)
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
	avp, _ = NewAVP("francisco.cardosogil@gmail.com-myIPv4Address", net.ParseIP(ipv4Address))
	if avp.GetIPAddress().String() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value (created as ipaddr) %s", avp.GetIPAddress())
	}
}

func TestIPv6Address(t *testing.T) {
	var ipv6Address = "bebe:cafe::0"

	// Create avp from string
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myIPv6Address", ipv6Address)
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
	avp, _ = NewAVP("francisco.cardosogil@gmail.com-myIPv6Address", net.ParseIP(ipv6Address))
	if avp.GetString() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value (created as ipaddr) %s", avp.GetString())
	}
}

func TestTimeAVP(t *testing.T) {
	var theTime, _ = time.Parse("02/01/2006 15:04:05 UTC", "26/11/1966 03:21:54 UTC")
	var theStringTime = "1966-11-26T03:21:54 UTC"

	// Create avp from string
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myTime", theStringTime)
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myDiameterIdentity", theString)
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myDiameterURI", theString)
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myIPFilterRule", theString)
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
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myIPv6Prefix", thePrefix)
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

	avp, err := NewAVP("francisco.cardosogil@gmail.com-myEnumerated", "zero")
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

	avp, err = NewAVP("francisco.cardosogil@gmail.com-myEnumerated", theNumber)
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
	avpl0, _ := NewAVP("francisco.cardosogil@gmail.com-myGroupedInGrouped", nil)
	avpl1, _ := NewAVP("francisco.cardosogil@gmail.com-myGrouped", nil)

	avpInt, _ := NewAVP("francisco.cardosogil@gmail.com-myInteger32", theInt)
	avpString, _ := NewAVP("francisco.cardosogil@gmail.com-myString", theString)

	avpl1.AddAVP(*avpInt).AddAVP(*avpString)
	avpl0.AddAVP(*avpl1)

	// Serialize and unserialize
	binaryAVP, _ := avpl0.MarshalBinary()
	recoveredAVPl0, _, _ := DiameterAVPFromBytes(binaryAVP)

	// Navigate to the values
	recoveredAVPl1 := recoveredAVPl0.GetAllAVP("francisco.cardosogil@gmail.com-myGrouped")[0]
	newInt, _ := recoveredAVPl1.GetAVP("francisco.cardosogil@gmail.com-myInteger32")
	if newInt.GetInt() != theInt {
		t.Error("Integer value does not match or not found in Group")
	}
	newString, _ := recoveredAVPl1.GetAVP("francisco.cardosogil@gmail.com-myString")
	if newString.GetString() != theString {
		t.Error("String value does not match or not found in Group")
	}

	// Non existing AVP
	_, err := recoveredAVPl1.GetAVP("non-existing")
	if err == nil {
		t.Error("No error when trying to find a non existing AVP")
	}

	// Printed avp
	var targetString = "{francisco.cardosogil@gmail.com-myGrouped={francisco.cardosogil@gmail.com-myInteger32=99,francisco.cardosogil@gmail.com-myString=theString}}"
	stringRepresentation := recoveredAVPl0.GetString()
	if stringRepresentation != targetString {
		t.Errorf("Grouped string representation does not match %s", stringRepresentation)
	}
}

func TestSerializationError(t *testing.T) {

	// Generate an AVP
	avp, err := NewAVP("francisco.cardosogil@gmail.com-myOctetString", "0A0B0C0c765654")
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
		"francisco.cardosogil@gmail.com-myTestAllGrouped": [
			{"francisco.cardosogil@gmail.com-myOctetString": "0102030405060708090a0b"},
			{"francisco.cardosogil@gmail.com-myInteger32": -99},
			{"francisco.cardosogil@gmail.com-myInteger64": -99},
			{"francisco.cardosogil@gmail.com-myUnsigned32": 99},
			{"francisco.cardosogil@gmail.com-myUnsigned64": 99},
			{"francisco.cardosogil@gmail.com-myFloat32": 99.9},
			{"francisco.cardosogil@gmail.com-myFloat64": 99.9},
			{"francisco.cardosogil@gmail.com-myAddress": "1.2.3.4"},
			{"francisco.cardosogil@gmail.com-myTime": "1966-11-26T03:34:08 UTC"},
			{"francisco.cardosogil@gmail.com-myString": "Hello, world!"},
			{"francisco.cardosogil@gmail.com-myDiameterIdentity": "Diameter@identity"},
			{"francisco.cardosogil@gmail.com-myDiameterURI": "Diameter@URI"},
			{"francisco.cardosogil@gmail.com-myIPFilterRule": "allow all"},
			{"francisco.cardosogil@gmail.com-myIPv4Address": "4.5.6.7"},
			{"francisco.cardosogil@gmail.com-myIPv6Address": "bebe:cafe::0"},
			{"francisco.cardosogil@gmail.com-myIPv6Prefix": "bebe:cafe::0/128"},
			{"francisco.cardosogil@gmail.com-myEnumerated": "two"}
		]
	}`

	// Read JSON to AVP
	var avp DiameterAVP
	err := json.Unmarshal([]byte(javp), &avp)
	if err != nil {
		t.Errorf("unmarshal error for avp: %s", err)
	}
	// Check the contents of the unmarshalled avp
	if avp.Name != "francisco.cardosogil@gmail.com-myTestAllGrouped" {
		t.Errorf("unmarshalled avp has the wrong name: %s", avp.Name)
	}
	if v, _ := avp.GetAVP("francisco.cardosogil@gmail.com-myEnumerated"); v.GetInt() != 2 {
		t.Errorf("unmarshalled avp has the wrong name: %s", avp.Name)
	}
	v, _ := avp.GetAVP("francisco.cardosogil@gmail.com-myTime")
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
	groupedInGroupedAVP, _ := NewAVP("francisco.cardosogil@gmail.com-myGroupedInGrouped", nil)
	groupedAVP, _ := NewAVP("francisco.cardosogil@gmail.com-myGrouped", nil)
	intAVP, _ := NewAVP("francisco.cardosogil@gmail.com-myInteger32", 1)
	stringAVP, _ := NewAVP("francisco.cardosogil@gmail.com-myString", "hello")
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

	diameterMessage.Add("francisco.cardosogil@gmail.com-myUnsigned32", 8)
	diameterMessage.Add("francisco.cardosogil@gmail.com-myUnsigned32", 9)

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
	unsignedAVPs := recoveredMessage.GetAllAVP("francisco.cardosogil@gmail.com-myUnsigned32")
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
	recoveredMessage.DeleteAllAVP("francisco.cardosogil@gmail.com-myUnsigned32")
	unsignedAVPs = recoveredMessage.GetAllAVP("francisco.cardosogil@gmail.com-myUnsigned32")
	if len(unsignedAVPs) != 0 {
		t.Errorf("avp still there after being deleted")
	}

	// Get and check the value of a grouped AVP
	gig, err := recoveredMessage.GetAVP("francisco.cardosogil@gmail.com-myGroupedInGrouped")
	if err != nil {
		t.Errorf("could not retrieve groupedingrouped avp: %s", err)
		return
	}
	g, err := gig.GetAVP("francisco.cardosogil@gmail.com-myGrouped")
	if err != nil {
		t.Errorf("could not retrieve grouped avp: %s", err)
		return
	}
	s, err := g.GetAVP("francisco.cardosogil@gmail.com-myString")
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
			  "francisco.cardosogil@gmail.com-myTestAllGrouped": [
  				{"francisco.cardosogil@gmail.com-myOctetString": "0102030405060708090a0b"},
  				{"francisco.cardosogil@gmail.com-myInteger32": -99},
  				{"francisco.cardosogil@gmail.com-myInteger64": -99},
  				{"francisco.cardosogil@gmail.com-myUnsigned32": 99},
  				{"francisco.cardosogil@gmail.com-myUnsigned64": 99},
  				{"francisco.cardosogil@gmail.com-myFloat32": 99.9},
  				{"francisco.cardosogil@gmail.com-myFloat64": 99.9},
  				{"francisco.cardosogil@gmail.com-myAddress": "1.2.3.4"},
  				{"francisco.cardosogil@gmail.com-myTime": "1966-11-26T03:34:08 UTC"},
  				{"francisco.cardosogil@gmail.com-myString": "Hello, world!"},
  				{"francisco.cardosogil@gmail.com-myDiameterIdentity": "Diameter@identity"},
  				{"francisco.cardosogil@gmail.com-myDiameterURI": "Diameter@URI"},
  				{"francisco.cardosogil@gmail.com-myIPFilterRule": "allow all"},
  				{"francisco.cardosogil@gmail.com-myIPv4Address": "4.5.6.7"},
  				{"francisco.cardosogil@gmail.com-myIPv6Address": "bebe:cafe::0"},
  				{"francisco.cardosogil@gmail.com-myIPv6Prefix": "bebe:cafe::0/128"},
  				{"francisco.cardosogil@gmail.com-myEnumerated": "two"}
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

	// fmt.Println(jBytes.String())
}

func TestMessageSize(t *testing.T) {
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
			  "francisco.cardosogil@gmail.com-myTestAllGrouped": [
				{"User-Name" : "this-is-the-user-name"},
  				{"francisco.cardosogil@gmail.com-myOctetString": "0102030405060708090a0b"},
  				{"francisco.cardosogil@gmail.com-myInteger32": -99},
  				{"francisco.cardosogil@gmail.com-myInteger64": -99},
  				{"francisco.cardosogil@gmail.com-myUnsigned32": 99},
  				{"francisco.cardosogil@gmail.com-myUnsigned64": 99},
  				{"francisco.cardosogil@gmail.com-myFloat32": 99.9},
  				{"francisco.cardosogil@gmail.com-myFloat64": 99.9},
  				{"francisco.cardosogil@gmail.com-myAddress": "1.2.3.4"},
  				{"francisco.cardosogil@gmail.com-myTime": "1966-11-26T03:34:08 UTC"},
  				{"francisco.cardosogil@gmail.com-myString": "Hello, world!"},
  				{"francisco.cardosogil@gmail.com-myDiameterIdentity": "Diameter@identity"},
  				{"francisco.cardosogil@gmail.com-myDiameterURI": "Diameter@URI"},
  				{"francisco.cardosogil@gmail.com-myIPFilterRule": "allow all"},
  				{"francisco.cardosogil@gmail.com-myIPv4Address": "4.5.6.7"},
  				{"francisco.cardosogil@gmail.com-myIPv6Address": "bebe:cafe::0"},
  				{"francisco.cardosogil@gmail.com-myIPv6Prefix": "bebe:cafe::0/128"},
  				{"francisco.cardosogil@gmail.com-myEnumerated": "two"}
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

	// Get size of serialized message
	buffer, _ := diameterMessage.MarshalBinary()
	if len(buffer) != diameterMessage.Len() {
		t.Errorf("error in diameter message len. Actual serialized size %d and reported len is %d", len(buffer), diameterMessage.Len())
	}
}
