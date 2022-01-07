package diamcodec

import (
	"fmt"
	"igor/config"
	"os"
	"testing"
)

func TestMain(m *testing.M) {

	// Initialize the Config Object as done in main.go
	boot := "resources/searchRules.json"
	instance := "testInstance"
	config.Config.Init(boot, instance)

	os.Exit(m.Run())
}

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
	newavp, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprintf("%x", password) {
		t.Errorf("Octets AVP not properly encoded after unmarshalling. Got %s", newavp.StringValue)
	}
}

func TestUTF8StringAVP(t *testing.T) {

	var theString = "'this-is_the string!"

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
	newavp, _ := DiameterAVPFromBytes(data)
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
	newavp, _ := DiameterAVPFromBytes(data)
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
	newavp, _ := DiameterAVPFromBytes(data)
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
	newavp, _ := DiameterAVPFromBytes(data)
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
	newavp, _ := DiameterAVPFromBytes(data)
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
	newavp, _ := DiameterAVPFromBytes(data)
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
	newavp, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprint(theFloat) {
		t.Errorf("Float64 AVP not properly encoded after unmarshalling (string value). Got %s", newavp.StringValue)
	}
	if newavp.FloatValue != float64(theFloat) {
		t.Errorf("Float64 AVP not properly encoded after unmarshalling (long value). Got %f", newavp.FloatValue)
	}
}
