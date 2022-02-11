package diamcodec

import (
	"bytes"
	"encoding/binary"
	"igor/config"
)

const (
	// Success
	DIAMETER_SUCCESS         = 2001
	DIAMETER_LIMITED_SUCCESS = 2002

	// Protocol Errors
	DIAMETER_UNKNOWN_PEER     = 3010
	DIAMETER_REALM_NOT_SERVED = 3003

	// Transient Failures
	DIAMETER_AUTHENTICATION_REJECTED = 4001

	// Permanent failures
	DIAMETER_UNKNOWN_SESSION_ID = 5002
	DIAMETER_UNABLE_TO_COMPLY   = 5012
)

type DiameterMessage struct {
	// Diameter Message is
	// 1 byte version
	// 3 byte message length
	// 1 byte flags
	//   request, proxyable, error, retransmission
	// 3 byte command code
	// 4 byte applicationId
	// 4 byte End-2-End Identifier
	// 4 byte Hop-by-Hop Identifier
	// ... AVP

	IsRequest        bool // 128
	IsProxyable      bool // 64
	IsError          bool // 32
	IsRetransmission bool // 16

	CommandCode   uint32
	ApplicationId uint32
	E2EId         uint32
	HopByHopId    uint32

	CommandName     string
	ApplicationName string

	avps []DiameterAVP
}

// Builds a DiameterMessage from it string representation
func DiameterMessageFromBytes(inputBytes []byte) (DiameterMessage, uint32, error) {

	diameterMessage := DiameterMessage{}

	var version byte
	var lenHigh uint8
	var lenLow uint16
	var messageLength uint32
	var flags uint8
	var commandCodeHigh uint8
	var commandCodeLow uint16

	reader := bytes.NewReader(inputBytes)

	// Get Version
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil {
		config.IgorLogger.Error("could not decode the version")
		return diameterMessage, 0, err
	}

	// Get Length
	if err := binary.Read(reader, binary.BigEndian, &lenHigh); err != nil {
		config.IgorLogger.Error("could not decode the Diameter message len (high) field")
		return diameterMessage, 0, err
	}
	if err := binary.Read(reader, binary.BigEndian, &lenLow); err != nil {
		config.IgorLogger.Error("could not decode the Diameter message len (high) field")
		return diameterMessage, 0, err
	}
	messageLength = uint32(lenHigh)*65535 + uint32(lenLow)

	// Get flags
	if err := binary.Read(reader, binary.BigEndian, &flags); err != nil {
		config.IgorLogger.Error("could not decode the Diameter message flags")
		return diameterMessage, 0, err
	}
	diameterMessage.IsRequest = flags&128 != 0
	diameterMessage.IsProxyable = flags&64 != 0
	diameterMessage.IsError = flags&32 != 0
	diameterMessage.IsRetransmission = flags&16 != 0

	// Get CommandCode
	if err := binary.Read(reader, binary.BigEndian, &commandCodeHigh); err != nil {
		config.IgorLogger.Error("could not decode the Command code (high) field")
		return diameterMessage, 0, err
	}
	if err := binary.Read(reader, binary.BigEndian, &commandCodeLow); err != nil {
		config.IgorLogger.Error("could not decode the Command code (high) field")
		return diameterMessage, 0, err
	}
	diameterMessage.CommandCode = uint32(commandCodeHigh)*65535 + uint32(commandCodeLow)

	// Get the applicationId
	if err := binary.Read(reader, binary.BigEndian, &diameterMessage.ApplicationId); err != nil {
		config.IgorLogger.Error("could not decode the Applicationid field")
		return diameterMessage, 0, err
	}

	// Get the E2EndId
	if err := binary.Read(reader, binary.BigEndian, &diameterMessage.E2EId); err != nil {
		config.IgorLogger.Error("could not decode the E2EId field")
		return diameterMessage, 0, err
	}

	// Get the HopByHopId
	if err := binary.Read(reader, binary.BigEndian, &diameterMessage.HopByHopId); err != nil {
		config.IgorLogger.Error("could not decode the HopByHopId field")
		return diameterMessage, 0, err
	}

	// Get the AVPs
	diameterMessage.avps = make([]DiameterAVP, 0)
	currentIndex := messageLength - 20 // The header is always 20 bytes
	for currentIndex < messageLength {
		nextAVP, bytesRead, err := DiameterAVPFromBytes(inputBytes[currentIndex:])
		if err != nil {
			return diameterMessage, 0, err
		}
		diameterMessage.avps = append(diameterMessage.avps, nextAVP)
		currentIndex += bytesRead
	}

	return diameterMessage, messageLength, nil
}

// Serializes the message. TODO: The message needs to have all its fields set => Call Tidy()
func (m *DiameterMessage) MarshalBinary() (data []byte, err error) {
	// Will write the output here
	var buffer = new(bytes.Buffer)

	// Write Version
	binary.Write(buffer, binary.BigEndian, byte(1))

	// Write Len as 0. Will be overriden later
	binary.Write(buffer, binary.BigEndian, uint8(0))
	binary.Write(buffer, binary.BigEndian, uint16(0))

	// Write flags
	var flags byte
	if m.IsRequest {
		flags += 128
	}
	if m.IsProxyable {
		flags += 64
	}
	if m.IsError {
		flags += 32
	}
	if m.IsRetransmission {
		flags += 16
	}
	binary.Write(buffer, binary.BigEndian, flags)

	// Write command code
	binary.Write(buffer, binary.BigEndian, byte(m.CommandCode/65535))
	binary.Write(buffer, binary.BigEndian, uint16(m.CommandCode%65535))

	// Write the rest of the fields
	binary.Write(buffer, binary.BigEndian, m.ApplicationId)
	binary.Write(buffer, binary.BigEndian, m.E2EId)
	binary.Write(buffer, binary.BigEndian, m.HopByHopId)

	// Write avps
	for i := range m.avps {
		// TODO: Need to enforce mandatory here
		avpBytes, err := m.avps[i].MarshalBinary()
		if err != nil {
			return nil, err
		}
		binary.Write(buffer, binary.BigEndian, avpBytes)
	}

	// Patch the length
	b := buffer.Bytes()
	b[1] = byte(len(b) / 65535)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)%65535))

	return b, nil
}

func (m *DiameterMessage) AddAVP(avp *DiameterAVP) {
	m.avps = append(m.avps, *avp)
}
