package diamcodec

import (
	"fmt"
	"igor/config"
	"os"
	"strings"
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
	var avp = DiameterOctetsAVP("Unknown AVP", []byte("hello, world!"))
	_, err := avp.MarshalBinary()
	if err == nil {
		t.Errorf("Unknown AVP was marshalled")
	} else if !strings.Contains(err.Error(), "found in dictionary") {
		t.Errorf("Incorrect error message when marshalling unknown avp")
	}
}

func TestOctetsAVP(t *testing.T) {

	var password = "'my-password!"

	// Create avp
	avp := DiameterOctetsAVP("User-Password", []byte(password))
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
	avp := DiameterStringAVP("User-Name", theString)
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

	var theInt = 65535*3 + 11

	// Create avp
	avp := DiameterLongAVP("francisco.cardosogil@gmail.com-myInteger32", int64(theInt))
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
}
