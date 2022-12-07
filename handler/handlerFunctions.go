package handler

import (
	"encoding/json"

	"github.com/francistor/igor/core"
)

// The most basic handler ever. Returns an empty response to the received message
func EmptyDiameterHandler(request *core.DiameterMessage) (*core.DiameterMessage, error) {
	hl := core.NewHandlerLogger()
	l := hl.L

	defer func(l *core.HandlerLogger) {
		l.WriteLog()
	}(hl)

	l.Infof("%s", "Starting EmptyDiameterHandler")
	l.Infof("%s %s", "request", request)

	response := core.NewDiameterAnswer(request)
	response.Add("Result-Code", core.DIAMETER_SUCCESS)

	l.Infof("%s %s", "response", request)

	return response, nil
}

// The most basic handler ever. Returns an empty response to the received message
func EmptyRadiusHandler(request *core.RadiusPacket) (*core.RadiusPacket, error) {
	hl := core.NewHandlerLogger()

	defer func(l *core.HandlerLogger) {
		l.WriteLog()
	}(hl)

	resp := core.NewRadiusResponse(request, true)

	return resp, nil
}

// Used to test all possible attribute types
func TestRadiusAttributesHandler(request *core.RadiusPacket) (*core.RadiusPacket, error) {
	hl := core.NewHandlerLogger()
	l := hl.L

	defer func(l *core.HandlerLogger) {
		l.WriteLog()
	}(hl)

	// Print the password
	pwd := request.GetPasswordStringAVP("User-Password")
	l.Infof("Password: <%s>", pwd)

	// Print all received attributes
	for _, avp := range request.AVPs {
		l.Info(avp.Name, avp.GetTaggedString())
	}

	// Reply with one attribute of each type
	// The Igor-SaltedOctetsAttribute contains the length as the first byte, since
	// in Nokia AAA this VSA is "salted-password" type
	jAVPs := `
				[
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
					{"Igor-TaggedStringAttribute": "mystring:1"},
					{"Igor-Integer64Attribute": 999999999999},
					{"Igor-SaltedOctetsAttribute": "0F313233343536373839616263646566"},
					{"Igor-TaggedSaltedOctetsAttribute": "0F313233343536373839616263646566:1"},
					{"User-Name":"MyUserName"}
				]
				`

	resp := core.NewRadiusResponse(request, true)

	var responseAVPs []core.RadiusAVP
	err := json.Unmarshal([]byte(jAVPs), &responseAVPs)
	if err != nil {
		l.Errorf("%s", err.Error())
	}

	for _, avp := range responseAVPs {
		resp.AddAVP(&avp)
	}

	return resp, nil
}
