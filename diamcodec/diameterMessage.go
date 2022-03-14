package diamcodec

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

	AVPs []DiameterAVP
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

	diameterApplication, ok := config.DDict.AppByCode[diameterMessage.ApplicationId]
	if ok {
		diameterMessage.ApplicationName = diameterApplication.Name
		diameterMessage.CommandName = diameterApplication.CommandByCode[diameterMessage.CommandCode].Name
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
	diameterMessage.AVPs = make([]DiameterAVP, 0)
	var currentIndex uint32 = 20 // The header is always 20 bytes
	for currentIndex < messageLength {
		nextAVP, bytesRead, err := DiameterAVPFromBytes(inputBytes[currentIndex:])
		if err != nil {
			return diameterMessage, 0, err
		}
		diameterMessage.AVPs = append(diameterMessage.AVPs, nextAVP)
		currentIndex += bytesRead
	}

	return diameterMessage, messageLength, nil
}

// Makes sure both codes and names are set for ApplicationId and CommandCode
func (m *DiameterMessage) Tidy() *DiameterMessage {

	if m.ApplicationId == 0 && m.ApplicationName != "" {
		m.ApplicationId = config.DDict.AppByName[m.ApplicationName].Code
	}

	if m.ApplicationId != 0 && m.ApplicationName == "" {
		m.ApplicationName = config.DDict.AppByCode[m.ApplicationId].Name
	}

	if m.CommandCode == 0 && m.CommandName != "" {
		m.CommandCode = config.DDict.AppByCode[m.ApplicationId].CommandByName[m.CommandName].Code
	}

	if m.CommandCode != 0 && m.CommandName == "" {
		m.CommandName = config.DDict.AppByCode[m.ApplicationId].CommandByCode[m.CommandCode].Name
	}

	return m
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
	for i := range m.AVPs {
		// TODO: Need to enforce mandatory here
		avpBytes, err := m.AVPs[i].MarshalBinary()
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

func (dm *DiameterMessage) Len() int {
	var avpLen = 0
	for i := range dm.AVPs {
		avpLen += dm.AVPs[i].Len()
	}

	return 20 + avpLen
}

///////////////////////////////////////////////////////////////
// AVP manipulation
///////////////////////////////////////////////////////////////

// Adds a new AVP to the message
func (m *DiameterMessage) AddAVP(avp *DiameterAVP) *DiameterMessage {
	// TODO: Check dictionary
	m.AVPs = append(m.AVPs, *avp)
	return m
}

// Adds a new AVP specified by name to the diameter message
func (m *DiameterMessage) Add(name string, value interface{}) *DiameterMessage {
	avp, error := NewAVP(name, value)

	if error != nil {
		config.IgorLogger.Errorf("avp could not be added %s: %v", name, value)
		return m
	}

	m.AVPs = append(m.AVPs, *avp)
	return m
}

// Retrieves the first AVP with the specified name from the message
func (m *DiameterMessage) GetAVP(avpName string) (DiameterAVP, error) {
	// Iterate through message avps
	for i := range m.AVPs {
		if m.AVPs[i].Name == avpName {
			return m.AVPs[i], nil
		}
	}
	return DiameterAVP{}, fmt.Errorf("avp named %s not found", avpName)
}

// Retrieves all AVP with the specified name from the message
func (m *DiameterMessage) GetAllAVP(avpName string) []DiameterAVP {

	// To be returned
	avpList := make([]DiameterAVP, 0)

	// Iterate through message avps
	for i := range m.AVPs {
		if m.AVPs[i].Name == avpName {
			avpList = append(avpList, m.AVPs[i])
		}
	}
	return avpList
}

func (m *DiameterMessage) DeleteAllAVP(avpName string) *DiameterMessage {

	// To be rewritten to the message
	avpList := make([]DiameterAVP, 0)
	for i := range m.AVPs {
		if m.AVPs[i].Name != avpName {
			avpList = append(avpList, m.AVPs[i])
		}
	}
	m.AVPs = avpList
	return m
}

///////////////////////////////////////////////////////////////
// Message constructors
///////////////////////////////////////////////////////////////

func NewDiameterRequest(appName string, commandName string) (DiameterMessage, error) {

	diameterMessage := DiameterMessage{IsRequest: true}

	// Find element in dictionary
	appDict, ok := config.DDict.AppByName[appName]
	if !ok {
		return diameterMessage, fmt.Errorf("application %s not found", appName)
	}

	commandDict, ok := appDict.CommandByName[commandName]
	if !ok {
		return diameterMessage, fmt.Errorf("command %s not found in application %s", commandName, appName)
	}

	diameterMessage.ApplicationName = appName
	diameterMessage.ApplicationId = appDict.Code
	diameterMessage.CommandName = commandDict.Name
	diameterMessage.CommandCode = commandDict.Code

	// Add mandatory parameters
	diameterMessage.Add("Origin-Host", config.DiameterServerConf().DiameterHost)
	diameterMessage.Add("Origin-Realm", config.DiameterServerConf().DiameterRealm)

	// E2EId and HopByHopId are filled out later
	return diameterMessage, nil

}

func NewDiameterAnswer(diameterRequest DiameterMessage) DiameterMessage {

	diameterMessage := DiameterMessage{}

	diameterMessage.ApplicationId = diameterRequest.ApplicationId
	diameterMessage.ApplicationName = diameterRequest.ApplicationName
	diameterMessage.CommandCode = diameterRequest.CommandCode
	diameterMessage.CommandName = diameterRequest.CommandName

	diameterMessage.E2EId = diameterRequest.E2EId
	diameterMessage.HopByHopId = diameterRequest.E2EId

	// Add mandatory parameters
	diameterMessage.Add("Origin-Host", config.DiameterServerConf().DiameterHost)
	diameterMessage.Add("Origin-Realm", config.DiameterServerConf().DiameterRealm)

	return diameterMessage
}

// TODO:
func CopyDiameterMessage(diameterMessage DiameterMessage) DiameterMessage {

	copy := DiameterMessage{}
	return copy
}

///////////////////////////////////////////////////////////////
// JSON Encoding
///////////////////////////////////////////////////////////////
