package diamcodec

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"igor/config"
	"io"
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

func (dm *DiameterMessage) ReadFrom(reader io.Reader) (n int64, err error) {
	var version byte
	var lenHigh uint8
	var lenLow uint16
	var messageLength uint32
	var flags uint8
	var commandCodeHigh uint8
	var commandCodeLow uint16

	currentIndex := int64(0)

	// Get Version
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil {
		return 0, err
	}
	currentIndex += 1

	// Get Length
	if err := binary.Read(reader, binary.BigEndian, &lenHigh); err != nil {
		return currentIndex, err
	}
	currentIndex += 1
	if err := binary.Read(reader, binary.BigEndian, &lenLow); err != nil {
		return currentIndex, err
	}
	currentIndex += 2
	messageLength = uint32(lenHigh)*65535 + uint32(lenLow)

	// Get flags
	if err := binary.Read(reader, binary.BigEndian, &flags); err != nil {
		return currentIndex, err
	}
	currentIndex += 1
	dm.IsRequest = flags&128 != 0
	dm.IsProxyable = flags&64 != 0
	dm.IsError = flags&32 != 0
	dm.IsRetransmission = flags&16 != 0

	// Get CommandCode
	if err := binary.Read(reader, binary.BigEndian, &commandCodeHigh); err != nil {
		return currentIndex, err
	}
	currentIndex += 1
	if err := binary.Read(reader, binary.BigEndian, &commandCodeLow); err != nil {
		return currentIndex, err
	}
	currentIndex += 2
	dm.CommandCode = uint32(commandCodeHigh)*65535 + uint32(commandCodeLow)

	// Get the applicationId
	if err := binary.Read(reader, binary.BigEndian, &dm.ApplicationId); err != nil {
		return currentIndex, err
	}
	currentIndex += 4

	diameterApplication, ok := config.DDict.AppByCode[dm.ApplicationId]
	if ok {
		dm.ApplicationName = diameterApplication.Name
		dm.CommandName = diameterApplication.CommandByCode[dm.CommandCode].Name
	}

	// Get the E2EndId
	if err := binary.Read(reader, binary.BigEndian, &dm.E2EId); err != nil {
		return currentIndex, err
	}
	currentIndex += 4

	// Get the HopByHopId
	if err := binary.Read(reader, binary.BigEndian, &dm.HopByHopId); err != nil {
		return currentIndex, err
	}
	currentIndex += 4

	// Get the AVPs
	dm.AVPs = make([]DiameterAVP, 0)
	if currentIndex != 20 {
		panic("assert failed. Bad header size in diameter message header")
	}
	// var currentIndex uint32 = 20 // The header is always 20 bytes
	for currentIndex < int64(messageLength) {
		nextAVP := DiameterAVP{}
		bytesRead, err := nextAVP.ReadFrom(reader)
		if err != nil {
			return currentIndex, err
		}
		dm.AVPs = append(dm.AVPs, nextAVP)
		currentIndex += bytesRead
	}

	if int64(messageLength) != currentIndex {
		panic("assert failed. Bad header size in diameter message")
	}

	return int64(messageLength), nil
}

func DiameterMessageFromBytes(inputBytes []byte) (DiameterMessage, uint32, error) {
	reader := bytes.NewReader(inputBytes)

	diameterMessage := DiameterMessage{}
	n, err := diameterMessage.ReadFrom(reader)

	return diameterMessage, uint32(n), err
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

// Writes the diameter message to the specified writer
func (m *DiameterMessage) WriteTo(buffer io.Writer) (int64, error) {

	currentIndex := int64(0)
	var err error

	// Write Version
	if err = binary.Write(buffer, binary.BigEndian, byte(1)); err != nil {
		return currentIndex, err
	}
	currentIndex += 1

	messageLen := m.Len()

	// Write Len
	if err = binary.Write(buffer, binary.BigEndian, byte(messageLen/65535)); err != nil {
		return currentIndex, err
	}
	currentIndex += 1
	if err = binary.Write(buffer, binary.BigEndian, uint16(messageLen%65535)); err != nil {
		return currentIndex, err
	}
	currentIndex += 2

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
	if err = binary.Write(buffer, binary.BigEndian, flags); err != nil {
		return currentIndex, err
	}
	currentIndex += 1

	// Write command code
	if err = binary.Write(buffer, binary.BigEndian, byte(m.CommandCode/65535)); err != nil {
		return currentIndex, err
	}
	currentIndex += 1
	if err = binary.Write(buffer, binary.BigEndian, uint16(m.CommandCode%65535)); err != nil {
		return currentIndex, err
	}
	currentIndex += 2

	// Write the rest of the fields
	if err = binary.Write(buffer, binary.BigEndian, m.ApplicationId); err != nil {
		return currentIndex, err
	}
	currentIndex += 4

	if err = binary.Write(buffer, binary.BigEndian, m.E2EId); err != nil {
		return currentIndex, err
	}
	currentIndex += 4

	if err = binary.Write(buffer, binary.BigEndian, m.HopByHopId); err != nil {
		return currentIndex, err
	}
	currentIndex += 4

	// Write avps
	for i := range m.AVPs {
		// TODO: Need to enforce mandatory here
		n, err := m.AVPs[i].WriteTo(buffer)
		if err != nil {
			return currentIndex, err
		}
		currentIndex += int64(n)
	}

	// Saninty check
	if currentIndex != int64(messageLen) {
		panic("assert failed. Bad message size")
	}

	return currentIndex, nil
}

func (dm *DiameterMessage) MarshalBinary() ([]byte, error) {
	var buffer = new(bytes.Buffer)
	_, err := dm.WriteTo(buffer)
	return buffer.Bytes(), err
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

	diameterMessage.HopByHopId = getHopByHopId()
	diameterMessage.E2EId = getE2EId()

	// Add mandatory parameters
	diameterMessage.Add("Origin-Host", config.DiameterServerConf().DiameterHost)
	diameterMessage.Add("Origin-Realm", config.DiameterServerConf().DiameterRealm)

	// E2EId and HopByHopId are filled out later
	return diameterMessage, nil

}

func NewDiameterAnswer(diameterRequest *DiameterMessage) DiameterMessage {

	diameterMessage := DiameterMessage{}

	diameterMessage.ApplicationId = diameterRequest.ApplicationId
	diameterMessage.ApplicationName = diameterRequest.ApplicationName
	diameterMessage.CommandCode = diameterRequest.CommandCode
	diameterMessage.CommandName = diameterRequest.CommandName

	diameterMessage.E2EId = diameterRequest.E2EId
	diameterMessage.HopByHopId = diameterRequest.HopByHopId

	// Add mandatory parameters
	diameterMessage.Add("Origin-Host", config.DiameterServerConf().DiameterHost)
	diameterMessage.Add("Origin-Realm", config.DiameterServerConf().DiameterRealm)

	return diameterMessage
}

// TODO:
func CopyDiameterMessage(diameterMessage *DiameterMessage) DiameterMessage {

	copy := DiameterMessage{}
	return copy
}

func (dm DiameterMessage) String() string {
	b, error := json.Marshal(dm)
	if error != nil {
		return "<error>"
	} else {
		return string(b)
	}
}

///////////////////////////////////////////////////////////////
// JSON Encoding
///////////////////////////////////////////////////////////////
