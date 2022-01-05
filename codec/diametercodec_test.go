package diamcodec

import (
	"fmt"
	"testing"
)

func TestOctetsAVP(t *testing.T) {

	// Create avp
	avp := new(DiameterAVP).SetName("User-Password").SetBytesValue([]byte("my-password"))
	if avp.StringValue != fmt.Sprintf("%x", "my-password") {
		t.Errorf("Password not properly encoded (before unmarshalling)")
	}

	// Serialize and unserialize
	data, _ := avp.MarshalBinary()
	newavp, _ := DiameterAVPFromBytes(data)
	if newavp.StringValue != fmt.Sprintf("%x", "my-password") {
		t.Errorf("Password not properly encoded (after unmarshalling)")
	}
}
