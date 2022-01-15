package diamcodec

import (
	"fmt"
	"igor/config"
	"net"
	"os"
	"reflect"
	"testing"
	"time"
)

// Initializer of the test suite.
func TestMain(m *testing.M) {

	// Initialize the Config Object as done in main.go
	bootstrapFile := "resources/searchRules.json"
	instanceName := "testInstance"
	config.Config.Init(bootstrapFile, instanceName)

	// Execute the tests and exit
	os.Exit(m.Run())
}

// One test for each AVP type

func TestAVPNotFound(t *testing.T) {
	var _, err = DiameterOctetsAVP("Unknown AVP", []byte("hello, world!"))
	if err == nil {
		t.Errorf("Unknown AVP was created")
	}
}

func TestOctetsAVP(t *testing.T) {

	var password = "'my-password!"

	// Create avp
	avp, err := DiameterOctetsAVP("User-Password", []byte(password))
	if err != nil {
		t.Errorf("error creating Octets AVP %v", err)
	}
	if avp.StringValue != fmt.Sprintf("%x", password) {
		t.Errorf("Octets AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprintf("%x", password) {
		t.Errorf("Octets AVP not properly encoded after unmarshalling. Got %s", newavp.StringValue)
	}
}

func TestUTF8StringAVP(t *testing.T) {

	var theString = "%Hola España. 'Quiero €"

	// Create avp
	avp, err := DiameterStringAVP("User-Name", theString)
	if err != nil {
		t.Errorf("error creating UTFString AVP %v", err)
	}
	if avp.StringValue != theString {
		t.Errorf("UTF8String AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != theString {
		t.Errorf("UTF8String AVP not properly encoded after unmarshalling. Got %s", newavp.StringValue)
	}
}

func TestInt32AVP(t *testing.T) {

	var theInt int32 = -65535*16384 - 1000 // 2^31 - 1000

	// Create avp
	avp, err := DiameterLongAVP("francisco.cardosogil@gmail.com-myInteger32", int64(theInt))
	if err != nil {
		t.Errorf("error creating Int32 AVP %v", err)
	}
	if avp.LongValue != int64(theInt) {
		t.Errorf("Int32 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprint(theInt) {
		t.Errorf("Integer32 AVP not properly encoded after unmarshalling (string value). Got %s", newavp.StringValue)
	}
	if newavp.LongValue != int64(theInt) {
		t.Errorf("Integer32 AVP not properly encoded after unmarshalling (long value). Got %d", newavp.LongValue)
	}
	if newavp.LongValue >= 0 {
		t.Errorf("Integer32 should be negative. Got %d", newavp.LongValue)
	}

	// Create from string
	otheravp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myInteger32", fmt.Sprint(theInt))
	if err != nil {
		t.Errorf("error creating Int32 AVP %v", err)
	}
	if otheravp.Code != newavp.Code {
		t.Errorf("Integer32 Codes not matching")
	}
	if otheravp.LongValue != newavp.LongValue {
		t.Errorf("Integer32 Values not mathing %d %d", otheravp.LongValue, newavp.LongValue)
	}
}

func TestInt64AVP(t *testing.T) {

	var theInt int64 = -65535*65535*65534*16384 - 999 // - 2 ^ 62 - 999
	// Create avp
	avp, err := DiameterLongAVP("francisco.cardosogil@gmail.com-myInteger64", theInt)
	if err != nil {
		t.Errorf("error creating Int64 AVP %v", err)
	}
	if avp.LongValue != int64(theInt) {
		t.Errorf("Int64 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprint(theInt) {
		t.Errorf("Integer64 AVP not properly encoded after unmarshalling (string value). Got %s", newavp.StringValue)
	}
	if newavp.LongValue != int64(theInt) {
		t.Errorf("Integer64 AVP not properly encoded after unmarshalling (long value). Got %d", newavp.LongValue)
	}
	if newavp.LongValue >= 0 {
		t.Errorf("Integer64 should be negative. Got %d", newavp.LongValue)
	}
}

func TestUnsignedInt32AVP(t *testing.T) {

	var theInt uint32 = 65535 * 40001

	// Create avp
	avp, err := DiameterLongAVP("francisco.cardosogil@gmail.com-myUnsigned32", int64(theInt))
	if err != nil {
		t.Errorf("error creating UInt32 AVP %v", err)
	}
	if avp.LongValue != int64(theInt) {
		t.Errorf("UInt32 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprint(theInt) {
		t.Errorf("UnsignedInteger32 AVP not properly encoded after unmarshalling (string value). Got %s", newavp.StringValue)
	}
	if newavp.LongValue != int64(theInt) {
		t.Errorf("UnsignedInteger32 AVP not properly encoded after unmarshalling (long value). Got %d", newavp.LongValue)
	}
	if newavp.LongValue < 0 {
		t.Errorf("Unsigned Integer32 should be positive. Got %d", newavp.LongValue)
	}
}

func TestUnsignedInt64AVP(t *testing.T) {

	// Due to a limitaton of the implementation, it is inernally stored as a signed int64
	var theInt int64 = 65535 * 65535 * 65535 * 16001

	// Create avp
	avp, err := DiameterLongAVP("francisco.cardosogil@gmail.com-myUnsigned64", theInt)
	if err != nil {
		t.Errorf("error creating UInt64 AVP %v", err)
	}
	if avp.LongValue != int64(theInt) {
		t.Errorf("Unsigned Int64 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprint(theInt) {
		t.Errorf("Unsigned Integer64 AVP not properly encoded after unmarshalling (string value). Got %s", newavp.StringValue)
	}
	if newavp.LongValue != int64(theInt) {
		t.Errorf("Unsigned Integer64 AVP not properly encoded after unmarshalling (long value). Got %d", newavp.LongValue)
	}
	if newavp.LongValue < 0 {
		t.Errorf("Unsigned Integer64 should be positive. Got %d", newavp.LongValue)
	}
}

func TestFloat32AVP(t *testing.T) {

	var theFloat float32 = 6.03e23

	// Create avp
	avp, err := DiameterFloatAVP("francisco.cardosogil@gmail.com-myFloat32", float64(theFloat))
	if err != nil {
		t.Errorf("error creating Float32 AVP %v", err)
	}
	if avp.FloatValue != float64(theFloat) {
		t.Errorf("Float32 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprint(theFloat) {
		t.Errorf("Float32 AVP not properly encoded after unmarshalling (string value). Got %s", newavp.StringValue)
	}
	if newavp.FloatValue != float64(theFloat) {
		t.Errorf("Float32 AVP not properly encoded after unmarshalling (long value). Got %f", newavp.FloatValue)
	}
}

func TestFloat64AVP(t *testing.T) {

	var theFloat float64 = 6.03e23

	// Create avp
	avp, err := DiameterFloatAVP("francisco.cardosogil@gmail.com-myFloat64", float64(theFloat))
	if err != nil {
		t.Errorf("error creating Float64 AVP %v", err)
	}
	if avp.FloatValue != float64(theFloat) {
		t.Errorf("Float64 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprint(theFloat) {
		t.Errorf("Float64 AVP not properly encoded after unmarshalling (string value). Got %s", newavp.StringValue)
	}
	if newavp.FloatValue != float64(theFloat) {
		t.Errorf("Float64 AVP not properly encoded after unmarshalling (long value). Got %f", newavp.FloatValue)
	}
}

func TestAddressAVP(t *testing.T) {

	var ipv4Address = "1.2.3.4"
	var ipv6Address = "bebe:cafe::0"

	// Using strings as values

	// IPv4
	// Create avp
	avp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myAddress", ipv4Address)
	if err != nil {
		t.Errorf("error creating IPv4 Address AVP %v", err)
	}
	if avp.IPAddressValue.String() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.IPAddressValue.String() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP not properly encoded after unmarshalling (string value). Got %s %s", newavp.IPAddressValue.String(), net.ParseIP(ipv4Address).String())
	}

	// IPv6
	// Create avp
	avp, err = DiameterStringAVP("francisco.cardosogil@gmail.com-myAddress", ipv6Address)
	if err != nil {
		t.Errorf("error creating IPv6 Address AVP %v", err)
	}
	if avp.IPAddressValue.String() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ = avp.MarshalBinary()
	newavp, _, _ = DiameterAVPFromBytes(data)
	if newavp.IPAddressValue.String() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP not properly encoded after unmarshalling (string value). Got %s %s", newavp.IPAddressValue.String(), net.ParseIP(ipv6Address).String())
	}

	// Using IP addresses as value
	avp, _ = DiameterIPAddressAVP("francisco.cardosogil@gmail.com-myAddress", net.ParseIP(ipv4Address))
	if avp.IPAddressValue.String() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value (created as ipaddr) %s %s", avp.IPAddressValue.String(), net.ParseIP(ipv4Address).String())
	}

	avp, _ = DiameterIPAddressAVP("francisco.cardosogil@gmail.com-myAddress", net.ParseIP(ipv6Address))
	if avp.IPAddressValue.String() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value (created as ipaddr) %s %s", avp.IPAddressValue.String(), net.ParseIP(ipv6Address).String())
	}
}

func TestIPv4Address(t *testing.T) {

	var ipv4Address = "1.2.3.4"

	// Create avp
	avp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myIPv4Address", ipv4Address)
	if err != nil {
		t.Errorf("error creating IPv4 Address AVP %v", err)
	}
	if avp.IPAddressValue.String() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.IPAddressValue.String() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP not properly encoded after unmarshalling (string value). Got %s %s", newavp.IPAddressValue.String(), net.ParseIP(ipv4Address).String())
	}

	avp, _ = DiameterIPAddressAVP("francisco.cardosogil@gmail.com-myIPv4Address", net.ParseIP(ipv4Address))
	if avp.IPAddressValue.String() != net.ParseIP(ipv4Address).String() {
		t.Errorf("IPv4 AVP does not match value (created as ipaddr) %s %s", avp.IPAddressValue.String(), net.ParseIP(ipv4Address).String())
	}
}

func TestIPv6Address(t *testing.T) {
	var ipv6Address = "bebe:cafe::0"

	// Create avp
	avp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myIPv6Address", ipv6Address)
	if err != nil {
		t.Errorf("error creating IPv6 Address AVP %v", err)
	}
	if avp.IPAddressValue.String() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.IPAddressValue.String() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP not properly encoded after unmarshalling (string value). Got %s %s", newavp.IPAddressValue.String(), net.ParseIP(ipv6Address).String())
	}

	avp, _ = DiameterIPAddressAVP("francisco.cardosogil@gmail.com-myIPv6Address", net.ParseIP(ipv6Address))
	if avp.IPAddressValue.String() != net.ParseIP(ipv6Address).String() {
		t.Errorf("IPv6 AVP does not match value (created as ipaddr) %s %s", avp.IPAddressValue.String(), net.ParseIP(ipv6Address).String())
	}
}

func TestTimeAVP(t *testing.T) {
	var theTime, _ = time.Parse("02/01/2006 15:04:05 UTC", "26/11/1966 03:21:54 UTC")
	var theStringTime = "1966-11-26T03:21:54"

	avp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myTime", theStringTime)
	if err != nil {
		t.Errorf("error creating Time Address AVP %v", err)
	}
	if avp.DateValue != theTime {
		t.Errorf("Time AVP does not match value")
	}
}

func TestDiamIdentAVP(t *testing.T) {

	var theString = "domain.name"

	// Create avp
	avp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myDiameterIdentity", theString)
	if err != nil {
		t.Errorf("error creating Diameter Identity AVP %v", err)
	}
	if avp.StringValue != theString {
		t.Errorf("Diamident AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != theString {
		t.Errorf("Diameter Identity AVP not properly encoded after unmarshalling. Got %s", newavp.StringValue)
	}
}

func TestDiamURIAVP(t *testing.T) {

	var theString = "domain.name"

	// Create avp
	avp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myDiameterURI", theString)
	if err != nil {
		t.Errorf("error creating Diameter URI AVP %v", err)
	}
	if avp.StringValue != theString {
		t.Errorf("Diamident AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != theString {
		t.Errorf("Diameter URI AVP not properly encoded after unmarshalling. Got %s", newavp.StringValue)
	}
}

func TestIPFilterRuleIAVP(t *testing.T) {

	var theString = "deny 1.2.3.4"

	// Create avp
	avp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myIPFilterRule", theString)
	if err != nil {
		t.Errorf("error creating IP Filter Rule AVP %v", err)
	}
	if avp.StringValue != theString {
		t.Errorf("IP Filter Rule AVP does not match value")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != theString {
		t.Errorf("IP Filter Rule AVP not properly encoded after unmarshalling. Got %s", newavp.StringValue)
	}
}

func TestEnumeratedAVP(t *testing.T) {

	var theString = "zero"
	var theNumber int64 = 0

	avp, err := DiameterStringAVP("francisco.cardosogil@gmail.com-myEnumerated", "zero")
	if err != nil {
		t.Errorf("error creating IP Filter Rule AVP %v", err)
	}
	if avp.StringValue != theString {
		t.Errorf("Enumerated AVP does not match string value")
	}
	if avp.LongValue != theNumber {
		t.Errorf("Enumerated AVP does not match number value")
	}

	avp, err = DiameterLongAVP("francisco.cardosogil@gmail.com-myEnumerated", theNumber)
	if err != nil {
		t.Errorf("error creating IP Filter Rule AVP %v", err)
	}
	if avp.StringValue != theString {
		t.Errorf("Enumerated AVP does not match string value")
	}
	if avp.LongValue != theNumber {
		t.Errorf("Enumerated AVP does not match number value")
	}
}

func TestSerializationError(t *testing.T) {

	avp, _ := DiameterStringAVP("francisco.cardosogil@gmail.com-myOctetString", "blah blah blah")

	theBytes, _ := avp.MarshalBinary()
	fmt.Println(cap(theBytes), len(theBytes))

	var theBytesUnknown []byte
	fmt.Println(cap(theBytesUnknown))
	theBytesUnknown = append(theBytesUnknown, theBytes...)
	fmt.Println(cap(theBytesUnknown))
	// Change the vendorId
	copy(theBytesUnknown[8:12], []byte{11, 12, 13, 14})

	newavp, _, _ := DiameterAVPFromBytes(theBytesUnknown)
	if newavp.VendorId != 11*256*256*256+12*256*256+13*256+14 {
		t.Errorf("Unknown vendor Id was not unmarshalled")
	}

	otherBytes, err := newavp.MarshalBinary()
	if err != nil {
		t.Errorf("Error serializing unknown avp %s", err)
	}
	if !reflect.DeepEqual([]byte{11, 12, 13, 14}, otherBytes[8:12]) {
		t.Error("Error serializing unknown avp. Vendor Id does not match", err)
	}

	// Force unmarshalling error
	copy(theBytesUnknown[5:8], []byte{100, 100, 100})
	_, _, e := DiameterAVPFromBytes(theBytesUnknown)
	if e == nil {
		t.Error("Bad bytes should have reported error")
	}
}
