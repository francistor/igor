package radiuscodec

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"igor/config"
	"io"
	"net"
	"time"
)

const (
	// Success
	ACCESS_REQUEST = 1
	ACCESS_ACCEPT  = 2
	ACCESS_REJECT  = 3

	ACCOUNTING_REQUEST  = 4
	ACCOUNTING_RESPONSE = 5

	DISCONNECT_REQUEST = 40
	DISCONECT_ACK      = 41
	DISCONNECT_NAK     = 42

	COA_REQUEST = 43
	COA_ACK     = 44
	COA_NAK     = 45
)

var zero_authenticator = [16]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

// code: 1 byte
// identifier: 1 byte
// length: 2: 2 byte
// authenticator: 16 octets
// AVP[]

// Represents a Radius packet
type RadiusPacket struct {

	// Radius code
	Code byte

	// Identifier of the request, if this is a response packet.
	Identifier byte

	// The authenticator is auto-generated in an access request, used for encryption, and calculated
	// as the md5 of the packet in other requests
	// In responses it is calculated as md5 of the response where the authenticator bytes are those
	// of the request
	Authenticator [16]byte

	// The AVPs of the radius packet
	AVPs []RadiusAVP
}

// Reads the RadiusPacket from a Reader interface, such as a network connection
func (rp *RadiusPacket) FromReader(reader io.Reader, secret string) (n int64, err error) {

	var packetLen uint16

	currentIndex := int64(0)

	// Read code
	if err := binary.Read(reader, binary.BigEndian, &rp.Code); err != nil {
		return 0, err
	}
	currentIndex += 1

	// Read identifier
	if err := binary.Read(reader, binary.BigEndian, &rp.Identifier); err != nil {
		return int64(currentIndex), err
	}
	currentIndex += 1

	// Read length
	if err := binary.Read(reader, binary.BigEndian, &packetLen); err != nil {
		return int64(currentIndex), err
	}
	currentIndex += 2

	// Read authenticator
	if err := binary.Read(reader, binary.BigEndian, &rp.Authenticator); err != nil {
		return 0, err
	}
	currentIndex += 16

	rp.AVPs = make([]RadiusAVP, 0)
	for currentIndex < int64(packetLen) {
		nextAVP := RadiusAVP{}
		bytesRead, err := nextAVP.FromReader(reader, rp.Authenticator, secret)
		if err != nil {
			return currentIndex, err
		}
		rp.AVPs = append(rp.AVPs, nextAVP)
		currentIndex += bytesRead
	}

	if int64(packetLen) != currentIndex {
		panic("assert failed. Bad header size in diameter message")
	}

	return int64(packetLen), nil
}

// Builds a Radius Packet from a Byte slice
func RadiusPacketFromBytes(inputBytes []byte, secret string) (*RadiusPacket, error) {
	reader := bytes.NewReader(inputBytes)

	radiusPacket := RadiusPacket{}
	_, err := radiusPacket.FromReader(reader, secret)

	return &radiusPacket, err
}

// code: 1 byte
// identifier: 1 byte
// length: 2: 2 byte
// authenticator: 16 octets
// AVP[]
//
// Writes the radius message to the specified writer
// ACCESS_REQUEST
//   Authenticator is created from scratch
// OTHER REQUEST
//   Authenticator is md5(code+identifier+zeroed_authenticator+request_attributes+secret)
// RESPONSE
//   Authenticator is md5(Code+ID+Length+RequestAuth+Attributes+Secret)
// id is ignored in responses, where the id from the request and stored in the avp will be used
func (rp *RadiusPacket) ToWriter(outWriter io.Writer, secret string, id byte) (int64, error) {

	currentIndex := int64(0)
	var err error

	// If not an ACCESS_REQUEST, the authenticator is calculated based on a hash
	// of the full packet, so it cannot be written beforehand
	// Using this buffer as a temporary scratch pad
	var writer bytes.Buffer

	// Write code
	if err = binary.Write(&writer, binary.BigEndian, rp.Code); err != nil {
		return 0, err
	}
	currentIndex += 1

	// Write identifier
	var identifier byte
	if rp.Code == ACCESS_REQUEST || rp.Code == ACCOUNTING_REQUEST || rp.Code == DISCONNECT_REQUEST || rp.Code == COA_REQUEST {
		identifier = id
	} else {
		// The parameter is ignored. We use the one in the object
		identifier = rp.Identifier
	}
	if err = binary.Write(&writer, binary.BigEndian, identifier); err != nil {
		return 0, err
	}
	currentIndex += 1

	// Write length
	packetLen := rp.Len()
	if err = binary.Write(&writer, binary.BigEndian, packetLen); err != nil {
		return 0, err
	}
	currentIndex += 2

	// Write authenticator
	// If it is a response, authenticator will be set to the request authenticator,
	// Otherwise, set to a new one or to zero
	if rp.Code == ACCESS_REQUEST {
		rp.Authenticator = GetAuthenticator()
	} else if rp.Code == ACCOUNTING_REQUEST || rp.Code == DISCONNECT_REQUEST || rp.Code == COA_REQUEST {
		rp.Authenticator = zero_authenticator
	} else {
		// Do nothing. Authenticator will be set to the one in the request
	}
	if err = binary.Write(&writer, binary.BigEndian, rp.Authenticator); err != nil {
		return 0, err
	}
	currentIndex += 16

	// Write all the AVP
	for i := range rp.AVPs {
		n, err := rp.AVPs[i].ToWriter(&writer, rp.Authenticator, secret)
		if err != nil {
			return 0, err
		}
		currentIndex += int64(n)
	}

	// Saninty check
	if currentIndex != int64(packetLen) {
		panic("assert failed. Bad message size")
	}

	// Calculate final authenticator and write to stream
	var writtenBytes int64
	if rp.Code == ACCESS_REQUEST {
		n, err := writer.WriteTo(outWriter)
		if err != nil {
			return n, err
		}
		writtenBytes = n
	} else {
		// Authenticator is md5(code+identifier+(current authenticator in Authenticator field)+request_attributes+secret)
		//                                      (zero or the authenticator in the request    )
		hasher := md5.New()
		hasher.Write(writer.Bytes())
		hasher.Write([]byte(secret))
		auth := hasher.Sum(nil)

		// Write first bytes in the header
		bytesToWrite := writer.Bytes()
		n1, err := outWriter.Write(bytesToWrite[0:4])
		if err != nil {
			return int64(n1), err
		}
		// Write authenticator
		n2, err := outWriter.Write(auth)
		if err != nil {
			return int64(n1 + n2), err
		}
		// Write AVPs
		n3, err := outWriter.Write(bytesToWrite[20:])
		if err != nil {
			return int64(n1 + n2 + n3), err
		}

		writtenBytes = int64(n1 + n2 + n3)
	}

	// Sanity check
	if uint16(writtenBytes) != packetLen {
		panic(fmt.Sprintf("written %d bytes instead of %d", writtenBytes, packetLen))
	}

	return int64(packetLen), nil
}

// Returns a byte slice with the contents of the AVP
func (rp *RadiusPacket) ToBytes(secret string, id byte) (data []byte, err error) {

	// Will write the output here
	var buffer = new(bytes.Buffer)
	if _, err := rp.ToWriter(buffer, secret, id); err != nil {
		return buffer.Bytes(), err
	}

	return buffer.Bytes(), nil
}

// Returns the size of the Radius packet
func (dm *RadiusPacket) Len() uint16 {
	var avpLen byte = 0
	for i := range dm.AVPs {
		avpLen += dm.AVPs[i].Len()
	}

	return uint16(20 + avpLen)
}

///////////////////////////////////////////////////////////////
// AVP manipulation
///////////////////////////////////////////////////////////////

// Adds a new AVP to the message
func (rp *RadiusPacket) AddAVP(avp *RadiusAVP) *RadiusPacket {
	rp.AVPs = append(rp.AVPs, *avp)
	return rp
}

// Adds a new AVP specified by name to the diameter message
func (rp *RadiusPacket) Add(name string, value interface{}) *RadiusPacket {

	// If avp to add is nil, do nothing
	if value == nil {
		return rp
	}

	avp, error := NewAVP(name, value)

	if error != nil {
		config.GetLogger().Errorf("avp %s could not be added: %v, due to %s", name, value, error)
		return rp
	}

	rp.AVPs = append(rp.AVPs, *avp)
	return rp
}

// Retrieves the first AVP with the specified name from the message
func (rp *RadiusPacket) GetAVP(avpName string) (RadiusAVP, error) {
	// Iterate through message avps
	for i := range rp.AVPs {
		if rp.AVPs[i].Name == avpName {
			return rp.AVPs[i], nil
		}
	}
	return RadiusAVP{}, fmt.Errorf("avp named %s not found", avpName)

}

// Retrieves all AVP with the specified name from the message
func (rp *RadiusPacket) GetAllAVP(avpName string) []RadiusAVP {

	// To be returned
	avpList := make([]RadiusAVP, 0)

	// Iterate through message avps
	for i := range rp.AVPs {
		if rp.AVPs[i].Name == avpName {
			avpList = append(avpList, rp.AVPs[i])
		}
	}
	return avpList
}

// Deletes all AVP with the specified name
func (rp *RadiusPacket) DeleteAllAVP(avpName string) *RadiusPacket {

	// To be rewritten to the message
	avpList := make([]RadiusAVP, 0)
	for i := range rp.AVPs {
		if rp.AVPs[i].Name != avpName {
			avpList = append(avpList, rp.AVPs[i])
		}
	}
	rp.AVPs = avpList
	return rp
}

// Retrieves the specified AVP name as a string, or the string default value
// if not found (instead of returning an error. Use with care)
func (rp *RadiusPacket) GetStringAVP(avpName string) string {
	avp, err := rp.GetAVP(avpName)
	if err != nil {
		return ""
	}

	return avp.GetString()
}

func (rp *RadiusPacket) GetPasswordStringAVP(avpName string) string {
	avp, err := rp.GetAVP(avpName)
	if err != nil {
		return ""
	}

	if pwd, err := avp.GetPasswordString(); err != nil {
		return ""
	} else {
		return pwd
	}
}

// Same, for int
func (rp *RadiusPacket) GetIntAVP(avpName string) int64 {
	avp, err := rp.GetAVP(avpName)
	if err != nil {
		return 0
	}
	return avp.GetInt()
}

// Same for IPAddress
func (rp *RadiusPacket) GetIPAddressAVP(avpName string) net.IP {
	avp, err := rp.GetAVP(avpName)
	if err != nil {
		return net.IP{}
	}
	return avp.GetIPAddress()
}

// Same for Time
func (rp *RadiusPacket) GetDateAVP(avpName string) time.Time {
	avp, err := rp.GetAVP(avpName)
	if err != nil {
		return time.Time{}
	}
	return avp.GetDate()
}

///////////////////////////////////////////////////////////////
// Packet creation
///////////////////////////////////////////////////////////////

// Creates a new radius request with the specified code
func NewRadiusRequest(code byte) *RadiusPacket {
	return &RadiusPacket{Code: code}
}

// Creates a radius answer for the specified packet
// id is the same as for the request
func NewRadiusResponse(request *RadiusPacket, isSuccess bool) *RadiusPacket {
	var code byte
	if isSuccess {
		code = request.Code + 1
	} else {
		code = request.Code + 2
	}
	return &RadiusPacket{Code: code, Identifier: request.Identifier, Authenticator: request.Authenticator}
}

///////////////////////////////////////////////////////////////
// Packet Validation
///////////////////////////////////////////////////////////////

// Checks the response authenticator
func ValidateResponseAuthenticator(packetBytes []byte, requestAuthenticator [16]byte, secret string) bool {

	hasher := md5.New()
	hasher.Write(packetBytes[0:4])
	hasher.Write(requestAuthenticator[:])
	hasher.Write(packetBytes[20:])
	hasher.Write([]byte(secret))
	auth := hasher.Sum(nil)

	// Compare by brute force, better than reflect
	for i, b := range packetBytes[4:20] {
		if auth[i] != b {
			return false
		}
	}

	return true
}

///////////////////////////////////////////////////////////////
// Serialization
///////////////////////////////////////////////////////////////

func (rp RadiusPacket) String() string {
	b, error := json.Marshal(rp)
	if error != nil {
		return "<error>"
	} else {
		return string(b)
	}
}
