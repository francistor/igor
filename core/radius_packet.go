package core

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

type RadiusPacketType byte

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

var Zero_authenticator = [16]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

// Type for functions that handle the diameter requests received
type RadiusPacketHandler func(request *RadiusPacket) (*RadiusPacket, error)

// Radius packet in the wire
// code: 1 byte
// identifier: 1 byte
// length: 2: 2 byte
// authenticator: 16 octets
// AVP[]

// Represents a Radius packet
type RadiusPacket struct {

	// Radius code
	Code RadiusPacketType

	// Identifier of the request/response pair.
	Identifier byte

	// The authenticator is auto-generated in an access request, used for encryption, and calculated
	// as the md5 of the packet fields in other requests.
	// In responses it is calculated as md5 of the response where the authenticator bytes are those
	// of the request
	Authenticator [16]byte

	// The AVPs of the radius packet
	AVPs []RadiusAVP
}

// Reads the RadiusPacket from a Reader interface, such as a network connection.
// If reading a response, the relevant authenticator for encryption will be the one of the request, passed in the ra paramter
func (rp *RadiusPacket) FromReader(reader io.Reader, secret string, ra [16]byte) (n int64, err error) {

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

	// Build the relevant authenticator, which is the one received if it is a request, and the
	// one passed as parameter otherwise
	var authenticator [16]byte
	if rp.IsRequest() {
		authenticator = rp.Authenticator
	} else {
		authenticator = ra
	}

	// Read the AVPs
	rp.AVPs = make([]RadiusAVP, 0)
	for currentIndex < int64(packetLen) {
		nextAVP := RadiusAVP{}

		bytesRead, err := nextAVP.FromReader(reader, authenticator, secret)
		if err != nil {
			return currentIndex, err
		}

		avpsLen := len(rp.AVPs)
		// Support for concat attributes
		if nextAVP.DictItem.Concat && // Current has concat attribute
			avpsLen > 0 && // There is a previous one
			rp.AVPs[avpsLen-1].DictItem.Code == nextAVP.DictItem.Code && // Of the same Code
			rp.AVPs[avpsLen-1].DictItem.VendorId == nextAVP.DictItem.VendorId { // And same vendor

			// Append the octets to the value
			// It has been checked in the dictionary that concat must be octets
			rp.AVPs[avpsLen-1].Value = append(rp.AVPs[avpsLen-1].Value.([]byte), nextAVP.Value.([]byte)...)

		} else {
			rp.AVPs = append(rp.AVPs, nextAVP)
		}

		currentIndex += bytesRead
	}

	if int64(packetLen) != currentIndex {
		panic("assert failed. Bad radius packet. Sizes do not match")
	}

	return int64(packetLen), nil
}

// code: 1 byte
// identifier: 1 byte
// length: 2: 2 byte
// authenticator: 16 octets
// AVP[]
//
// Writes the radius message to the specified writer
// ACCESS_REQUEST
//
//	Authenticator is created from scratch, or reused from a previous instance of the same packet, in case of
//	re-transmission
//
// OTHER REQUEST
//
//	Authenticator is md5(code+identifier+length+zeroed_authenticator+request_attributes+secret)
//
// RESPONSE
//
//	Authenticator is md5(Code+ID+Length+RequestAuth+Attributes+Secret)
//
// id is ignored in responses, where the id from the request and stored in the avp will be used
func (rp *RadiusPacket) ToWriter(outWriter io.Writer, secret string, id byte, reuseAuthenticator [16]byte, reuse bool) (int64, error) {

	currentIndex := int64(0)
	var err error

	// If not an ACCESS_REQUEST, the authenticator is calculated based on a hash
	// of the full packet, so it cannot be written beforehand
	// Using this buffer as a temporary scratch pad
	var scratchWriter io.Writer
	if rp.Code == ACCESS_REQUEST {
		// Writer directly
		scratchWriter = outWriter
	} else {
		// Write to intermediate buffer
		scratchWriter = new(bytes.Buffer)
	}

	// Normalize AVPs to fit in 256 bytes
	// This mutates the RadiusPacket
	// First, check that this is going to be needed, to avoid copying everything if not necessary
	var doSplit = false
	for i := range rp.AVPs {
		if rp.AVPs[i].DictItem.Concat && rp.AVPs[i].Len() > 255 {
			doSplit = true
			break
		}
	}
	if doSplit {
		newAVPs := make([]RadiusAVP, 0)
		for i := range rp.AVPs {
			if rp.AVPs[i].DictItem.Concat && rp.AVPs[i].Len() > 255 {
				// Split into multiple attributes
				valueIndex := 0
				octetsValue := rp.AVPs[i].Value.([]byte)

				for valueIndex < len(octetsValue) {
					lastIndex := valueIndex + 240 // Play on the safe side
					if lastIndex > len(octetsValue) {
						lastIndex = len(octetsValue)
					}

					splitAVP := rp.AVPs[i]
					splitAVP.Value = octetsValue[valueIndex:lastIndex]
					newAVPs = append(newAVPs, splitAVP)

					valueIndex = lastIndex
				}
			} else {
				// Normal path
				newAVPs = append(newAVPs, rp.AVPs[i])
			}
		}

		rp.AVPs = newAVPs
	}

	// Write code
	if err = binary.Write(scratchWriter, binary.BigEndian, rp.Code); err != nil {
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
	if err = binary.Write(scratchWriter, binary.BigEndian, identifier); err != nil {
		return 0, err
	}
	currentIndex += 1

	// Write length
	packetLen := rp.Len()
	if err = binary.Write(scratchWriter, binary.BigEndian, packetLen); err != nil {
		return 0, err
	}
	currentIndex += 2

	// Write authenticator
	// If it is a response, authenticator will be set to the request authenticator.
	// Otherwise, set to a new one or to zero
	if rp.Code == ACCESS_REQUEST {
		if reuse {
			rp.Authenticator = reuseAuthenticator
		} else {
			rp.Authenticator = BuildRandomAuthenticator()
		}
	} else if rp.Code == ACCOUNTING_REQUEST || rp.Code == DISCONNECT_REQUEST || rp.Code == COA_REQUEST {
		rp.Authenticator = Zero_authenticator
	}
	// else Do nothing. Authenticator will be set to the one in the request
	if err = binary.Write(scratchWriter, binary.BigEndian, rp.Authenticator); err != nil {
		return 0, err
	}
	currentIndex += 16

	// Write all the AVP
	for i := 0; i < len(rp.AVPs); i++ {
		n, err := rp.AVPs[i].ToWriter(scratchWriter, rp.Authenticator, secret)
		if err != nil {
			return 0, err
		}
		currentIndex += int64(n)
	}

	// Saninty check
	if currentIndex != int64(packetLen) {
		panicString := fmt.Sprintf("assert failed. Bad message size. Current index: %d - Packetlen: %d", currentIndex, packetLen)
		panic(panicString)
	}

	// Calculate final authenticator and write to stream
	var writtenBytes int64
	if rp.Code == ACCESS_REQUEST {
		// Was already written directly to outwriter
		writtenBytes = currentIndex
	} else {
		// Authenticator is md5(code+identifier+(current authenticator in Authenticator field)+request_attributes+secret)
		//                                      (wich is a generated one (auth request) zero (other requests) or the authenticator in the request (response))

		// The writer is a bytes buffer but was casted as a simple writer. Uncast
		byteWriter := scratchWriter.(*bytes.Buffer)
		tmpPacketBytes := byteWriter.Bytes()

		// Calculate the authenticator, which is the md5 of the bytes in the packet with the secret appended
		hasher := md5.New()
		hasher.Write(tmpPacketBytes)
		hasher.Write([]byte(secret))
		auth := hasher.Sum(nil)

		// Write first bytes in the header
		n1, err := outWriter.Write(tmpPacketBytes[0:4])
		if err != nil {
			return int64(n1), err
		}
		// Write authenticator
		// And update the authenticator in the packet!
		n2, err := outWriter.Write(auth)
		if err != nil {
			return int64(n1 + n2), err
		}
		rp.Authenticator = *(*[16]byte)(auth)

		// Write AVPs
		n3, err := outWriter.Write(tmpPacketBytes[20:])
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

// Builds a Radius Packet from a Byte slice
func NewRadiusPacketFromBytes(inputBytes []byte, secret string, ra [16]byte) (*RadiusPacket, error) {
	reader := bytes.NewReader(inputBytes)

	radiusPacket := RadiusPacket{}
	_, err := radiusPacket.FromReader(reader, secret, ra)

	return &radiusPacket, err
}

// Returns a byte slice with the contents of the AVP
func (rp *RadiusPacket) ToBytes(secret string, id byte, reuseAuthenticator [16]byte, reuse bool) (data []byte, err error) {

	// Will write the output here
	var buffer bytes.Buffer
	if _, err := rp.ToWriter(&buffer, secret, id, reuseAuthenticator, reuse); err != nil {
		return buffer.Bytes(), err
	}

	return buffer.Bytes(), nil
}

// Returns the size of the Radius packet
func (rp *RadiusPacket) Len() uint16 {
	var avpLen uint16 = 0
	for i := range rp.AVPs {
		avpLen += uint16(rp.AVPs[i].Len())
	}

	// Header always has 20 bytes
	return 20 + avpLen
}

// Returns true if any type of access request
func (rp *RadiusPacket) IsRequest() bool {
	switch rp.Code {
	case ACCESS_REQUEST, ACCOUNTING_REQUEST, COA_REQUEST, DISCONNECT_REQUEST:
		return true
	default:
		return false
	}
}

///////////////////////////////////////////////////////////////
// AVP manipulation
///////////////////////////////////////////////////////////////

// Adds a new AVP to the message
func (rp *RadiusPacket) AddAVP(avp *RadiusAVP) *RadiusPacket {
	rp.AVPs = append(rp.AVPs, *avp)
	return rp
}

// Adds a new AVP to the message if does not already exist
func (rp *RadiusPacket) AddIfNotPresentAVP(avp *RadiusAVP) *RadiusPacket {

	// Iterate through message avps and do nothing if found
	for i := range rp.AVPs {
		if rp.AVPs[i].Name == avp.Name {
			return rp
		}
	}

	rp.AVPs = append(rp.AVPs, *avp)

	return rp
}

// Adds a new AVP to the message, replacing the existing one if already exists
func (rp *RadiusPacket) ReplaceAVP(avp *RadiusAVP) *RadiusPacket {

	// Iterate through message avps and delete the target avp if found
	for i := range rp.AVPs {
		if rp.AVPs[i].Name == avp.Name {
			rp.DeleteAllAVP(avp.Name)
			break
		}
	}

	rp.AVPs = append(rp.AVPs, *avp)

	return rp
}

// Adds a list of AVP to the message
func (rp *RadiusPacket) AddAVPs(avps []RadiusAVP) *RadiusPacket {
	for i := range avps {
		rp.AVPs = append(rp.AVPs, avps[i])
	}
	return rp
}

// Adds a list of AVP to the message, if the attrubite is not aleady present
func (rp *RadiusPacket) AddIfNotPresentAVPs(avps []RadiusAVP) *RadiusPacket {
	for i := range avps {
		rp.AddIfNotPresentAVP(&avps[i])
	}
	return rp
}

// Adds a list of AVP to the message using a replace strategy
func (rp *RadiusPacket) ReplaceAVPs(avps []RadiusAVP) *RadiusPacket {
	for i := range avps {
		rp.ReplaceAVP(&avps[i])
	}
	return rp
}

// Adds a new AVP specified by name to the packet
func (rp *RadiusPacket) Add(name string, value interface{}) *RadiusPacket {

	// If avp to add is nil, do nothing
	if value == nil {
		return rp
	}

	if avp, err := NewRadiusAVP(name, value); err != nil {
		GetLogger().Errorf("avp %s could not be added: %v, due to %s", name, value, err)
		return rp
	} else {
		rp.AVPs = append(rp.AVPs, *avp)
		return rp
	}
}

// Merges the AVP specified by name in the packet
func (rp *RadiusPacket) AddIfNotPresent(name string, value interface{}) *RadiusPacket {
	// If avp to add is nil, do nothing
	if value == nil {
		return rp
	}

	if avp, err := NewRadiusAVP(name, value); err != nil {
		GetLogger().Errorf("avp %s could not be merged: %v, due to %s", name, value, err)
		return rp
	} else {
		return rp.AddIfNotPresentAVP(avp)
	}
}

// Replaces the AVP specified by name in the packet
func (rp *RadiusPacket) Replace(name string, value interface{}) *RadiusPacket {
	// If avp to add is nil, do nothing
	if value == nil {
		return rp
	}
	if avp, err := NewRadiusAVP(name, value); err != nil {
		GetLogger().Errorf("avp %s could not be replaced: %v, due to %s", name, value, err)
		return rp
	} else {
		return rp.ReplaceAVP(avp)
	}
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

// Retrieves the specified AVP name as a string including the tag after
// a semicolon, or the string default value
// if not found (instead of returning an error. Use with care)
func (rp *RadiusPacket) GetTaggedStringAVP(avpName string) string {
	avp, err := rp.GetAVP(avpName)
	if err != nil {
		return ""
	}

	return avp.GetTaggedString()
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

// Same for octets
func (rp *RadiusPacket) GetOctetsAVP(avpName string) []byte {
	avp, err := rp.GetAVP(avpName)
	if err != nil {
		return nil
	}
	return avp.GetOctets()
}

///////////////////////////////////////////////////////////////
// Password validation
///////////////////////////////////////////////////////////////

// Performs the authentication using PAP or CHAP
func (rp *RadiusPacket) Auth(password string) (bool, error) {

	// PAP
	if p := rp.GetStringAVP("User-Password"); p != "" {
		return password == p, nil
	}

	// CHAP
	chapPwd := rp.GetOctetsAVP("CHAP-Password")
	if len(chapPwd) != 17 {
		return false, fmt.Errorf("no User-Password or invalid/unexisting CHAP-Password in request")
	}
	id := chapPwd[0]
	response := chapPwd[1:17]

	challenge := rp.GetOctetsAVP("CHAP-Challenge")
	if len(challenge) == 0 {
		challenge = rp.Authenticator[:]
	}

	// Calculate the desired output
	hasher := md5.New()
	hasher.Write([]byte{id})
	hasher.Write([]byte(password))
	hasher.Write(challenge)
	expected := hasher.Sum(nil)

	return bytes.Equal(expected, response), nil
}

///////////////////////////////////////////////////////////////
// Packet creation and manipulation
///////////////////////////////////////////////////////////////

// Creates a new radius request with the specified code
func NewRadiusRequest(code RadiusPacketType) *RadiusPacket {
	return &RadiusPacket{Code: code}
}

// Creates a radius answer for the specified packet
// id is the same as for the request
func NewRadiusResponse(request *RadiusPacket, isSuccess bool) *RadiusPacket {
	var code RadiusPacketType
	if isSuccess {
		code = request.Code + 1
	} else {
		code = request.Code + 2
	}
	return &RadiusPacket{Code: code, Identifier: request.Identifier, Authenticator: request.Authenticator}
}

// Converts a proxy response into the response to the original request, tweaking the Authenticator
func (rp *RadiusPacket) MakeResponseTo(request *RadiusPacket) *RadiusPacket {
	rp.Authenticator = request.Authenticator
	rp.Identifier = request.Identifier
	return rp
}

// Creates a copy of the radius packet but having only the AVPs in the positiveFilter argument
// or removing the attributes in the negativeFilter argument. If nil, no filter is applied.
// NOTICE that a request will be modified when sent (the authenticator is re-calculated). For
// this reason, a copy must be used when proxying or the answer generated from the original packet
// before being sent
func (rp *RadiusPacket) Copy(positiveFilter []string, negativeFilter []string) *RadiusPacket {
	copiedPacket := RadiusPacket{
		Code:          rp.Code,
		Identifier:    rp.Identifier,
		Authenticator: rp.Authenticator,
	}

	for i := range rp.AVPs {
		if positiveFilter != nil {
			if slices.Contains(positiveFilter, rp.AVPs[i].Name) {
				copiedPacket.AddAVP(&rp.AVPs[i])
			}
		} else if negativeFilter != nil {
			if !slices.Contains(negativeFilter, rp.AVPs[i].Name) {
				copiedPacket.AddAVP(&rp.AVPs[i])
			}
		} else {
			// If both are nil, copy all attributes
			copiedPacket.AddAVP(&rp.AVPs[i])
		}
	}

	return &copiedPacket
}

// Helper for Cisco-AVPair. Returns the AVP with the specified inner name
func (rp *RadiusPacket) GetCiscoAVPair(name string) string {
	avpairs := rp.GetAllAVP("Cisco-AVPair")
	for i := range avpairs {
		before, after, found := strings.Cut(avpairs[i].GetString(), "=")
		if found {
			pairName := strings.TrimSpace(before)
			if pairName == name {
				return strings.TrimSpace(after)
			}
		}
	}

	return ""
}

///////////////////////////////////////////////////////////////
// Packet Validation
///////////////////////////////////////////////////////////////

// Checks the response authenticator
// Response authenticator must be the md5 hash of the response bytes with the authenticator replaced by the
// request authenticator, followed by the secret
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

// Checks the request authenticator
// Request authenticator must be the md5 hash of the received bytes followed by the secret
func ValidateRequestAuthenticator(packetBytes []byte, secret string) bool {

	hasher := md5.New()
	hasher.Write(packetBytes[0:4])
	hasher.Write(Zero_authenticator[:])
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
		return ""
	} else {
		return string(b)
	}
}

func (rp *RadiusPacket) ToString() string {
	b, error := json.Marshal(rp)
	if error != nil {
		return ""
	} else {
		return string(b)
	}
}
