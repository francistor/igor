package core

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

// Uses the default configuration instance

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

// Type for functions that handle the diameter requests received
type DiameterMessageHandler func(request *DiameterMessage) (*DiameterMessage, error)

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

// Fills a DiameterMessage with the contents of the stream read in the argument
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

	diameterApplication, ok := GetDDict().AppByCode[dm.ApplicationId]
	if ok {
		dm.ApplicationName = diameterApplication.Name
		if command, found := diameterApplication.CommandByCode[dm.CommandCode]; found {
			dm.CommandName = command.Name
		}
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

// Returns a DiameterMessage decoded from the specified input bytes
func DiameterMessageFromBytes(inputBytes []byte) (DiameterMessage, uint32, error) {
	reader := bytes.NewReader(inputBytes)

	diameterMessage := DiameterMessage{}
	n, err := diameterMessage.ReadFrom(reader)

	return diameterMessage, uint32(n), err
}

// Makes sure both codes and names are set for ApplicationId and CommandCode
func (m *DiameterMessage) Tidy() *DiameterMessage {

	if m.ApplicationId == 0 && m.ApplicationName != "" {
		m.ApplicationId = GetDDict().AppByName[m.ApplicationName].Code
	}

	if m.ApplicationId != 0 && m.ApplicationName == "" {
		m.ApplicationName = GetDDict().AppByCode[m.ApplicationId].Name
	}

	if m.CommandCode == 0 && m.CommandName != "" {
		m.CommandCode = GetDDict().AppByCode[m.ApplicationId].CommandByName[m.CommandName].Code
	}

	if m.CommandCode != 0 && m.CommandName == "" {
		m.CommandName = GetDDict().AppByCode[m.ApplicationId].CommandByCode[m.CommandCode].Name
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
	// Optimization to avoid division in most cases
	var lenHighByte uint8
	var lenLowWord uint16
	if messageLen < 65535 {
		lenHighByte = 0
		lenLowWord = uint16(messageLen)
	} else {
		lenHighByte = uint8(messageLen / 65535)
		lenLowWord = uint16(messageLen % 65535)
	}
	if err = binary.Write(buffer, binary.BigEndian, lenHighByte); err != nil {
		return currentIndex, err
	}
	currentIndex += 1
	if err = binary.Write(buffer, binary.BigEndian, lenLowWord); err != nil {
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

	// Get the command to be used to enforce the mandatory bit
	command, errNotInDict := GetDDict().GetCommand(m.ApplicationId, m.CommandCode)

	// Write avps
	for i := range m.AVPs {
		// Enforce the mandatory bit
		if m.IsRequest && errNotInDict != nil {
			if group, found := command.Request[m.AVPs[i].Name]; found && group.Mandatory {
				m.AVPs[i].IsMandatory = true
			}
		}

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

// Implement the BinaryMarshaler interface
func (dm *DiameterMessage) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer
	_, err := dm.WriteTo(&buffer)
	return buffer.Bytes(), err
}

// Implement the BinaryUnmarshaler interface
func (dm *DiameterMessage) UnmarshalBinary(data []byte) error {
	_, err := dm.ReadFrom(bytes.NewReader(data))
	return err
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

// Checks that the attributes for this command are conforming to the dictionary specification
func (m *DiameterMessage) CheckAttributes() error {

	command, err := GetDDict().GetCommand(m.ApplicationId, m.CommandCode)
	if err != nil {
		return err
	}

	var attrSpec map[string]GroupedProperties
	if m.IsRequest {
		attrSpec = command.Request
	} else {
		attrSpec = command.Response
	}

	// Check that the number of instances of each atribute conforms to the specification
	for attrName, groupSpec := range attrSpec {
		nOfInstances := len(m.GetAllAVP(attrName))
		if groupSpec.MinOccurs > 0 && nOfInstances < groupSpec.MinOccurs {
			return fmt.Errorf("%s has %d instances which is less than the minimum %d", attrName, nOfInstances, groupSpec.MinOccurs)
		} else if groupSpec.MaxOccurs > 0 && nOfInstances > groupSpec.MaxOccurs {
			return fmt.Errorf("%s has %d instances which is more than the maximum %d", attrName, nOfInstances, groupSpec.MaxOccurs)
		}
	}

	// Check that all attributes in the message are valid
	for i := range m.AVPs {
		attrName := m.AVPs[i].Name
		if _, found := attrSpec[attrName]; !found {
			return fmt.Errorf("%s not valid for command %s and application %s", attrName, m.ApplicationName, m.CommandName)
		}

		// Check the AVP itself
		err := m.AVPs[i].Check()
		if err != nil {
			return err
		}
	}

	return nil
}

// Adds a new AVP to the message
func (m *DiameterMessage) AddAVP(avp *DiameterAVP) *DiameterMessage {
	if avp != nil {
		m.AVPs = append(m.AVPs, *avp)
	}
	return m
}

// Adds a new AVP specified by name to the diameter message
func (m *DiameterMessage) Add(name string, value interface{}) *DiameterMessage {

	// If avp to add is nil, do nothing
	if value == nil {
		return m
	}

	avp, err := NewDiameterAVP(name, value)

	if err != nil {
		GetLogger().Errorf("avp could not be added %s: %v, %s", name, value, err)
		return m
	}

	m.AVPs = append(m.AVPs, *avp)
	return m
}

// Retrieves the first AVP with the specified name from the message,
// and error if not found
func (m *DiameterMessage) GetAVP(avpName string) (DiameterAVP, error) {
	// Iterate through message avps
	for i := range m.AVPs {
		if m.AVPs[i].Name == avpName {
			return m.AVPs[i], nil
		}
	}
	return DiameterAVP{}, fmt.Errorf("avp named %s not found", avpName)
}

// Retrieves the first AVP with the specified path (dot separated) from the message
func (m *DiameterMessage) GetAVPFromPath(avpName string) (DiameterAVP, error) {
	pathComponents := strings.Split(avpName, ".")

	// The first iteration gets the AVP from the message, using the name until the
	// first dot, then the navigation is done on the successive AVP got from the
	// Group
	var avp DiameterAVP
	var err error
	for i, pathComponent := range pathComponents {
		if i == 0 {
			avp, err = m.GetAVP(pathComponent)
			if err != nil {
				return DiameterAVP{}, err
			}
		} else {
			avp, err = avp.GetAVP(pathComponent)
			if err != nil {
				return DiameterAVP{}, err
			}
		}
	}

	return avp, nil
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

// Deletes all AVP with the specified name
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

// Gets the Result-Code, or 0 if not found
func (m *DiameterMessage) GetResultCode() int {
	rc, err := m.GetAVP("Result-Code")
	if err != nil {
		return 0
	}

	return int(rc.GetInt())
}

// Retrieves the specified AVP name as a string, or the string default value
// if not found (instead of returning an error. Use with care).
// The AVP name may be a path including grouped attributes, that is
// avpname1.avpname2, etc.
func (m *DiameterMessage) GetStringAVP(avpName string) string {
	avp, err := m.GetAVPFromPath(avpName)
	if err != nil {
		return ""
	}

	return avp.GetString()
}

// Retrieves the specified AVP name as an integer, or 0
// if not found (instead of returning an error. Use with care).
// The AVP name may be a path including grouped attributes, that is
// avpname1.avpname2, etc.
func (m *DiameterMessage) GetIntAVP(avpName string) int64 {
	avp, err := m.GetAVPFromPath(avpName)
	if err != nil {
		return 0
	}
	return avp.GetInt()
}

// Retrieves the specified AVP name as a float, or 0
// if not found (instead of returning an error. Use with care).
// The AVP name may be a path including grouped attributes, that is
// avpname1.avpname2, etc.
func (m *DiameterMessage) GetFloatAVP(avpName string) float64 {
	avp, err := m.GetAVPFromPath(avpName)
	if err != nil {
		return 0
	}
	return avp.GetFloat()
}

// Retrieves the specified AVP name as a IP Address, or the IPAddr default value
// if not found (instead of returning an error. Use with care).
// The AVP name may be a path including grouped attributes, that is
// avpname1.avpname2, etc.
func (m *DiameterMessage) GetIPAddressAVP(avpName string) net.IP {
	avp, err := m.GetAVPFromPath(avpName)
	if err != nil {
		return net.IP{}
	}
	return avp.GetIPAddress()
}

// Retrieves the specified AVP name as a time.Time, or the Time default value
// if not found (instead of returning an error. Use with care).
// The AVP name may be a path including grouped attributes, that is
// avpname1.avpname2, etc.
func (m *DiameterMessage) GetDateAVP(avpName string) time.Time {
	avp, err := m.GetAVPFromPath(avpName)
	if err != nil {
		return time.Time{}
	}
	return avp.GetDate()
}

// Helper function to add Origin-Host and Origin-Realm attributes
func (dm *DiameterMessage) AddOriginAVPs(ci *PolicyConfigurationManager) *DiameterMessage {
	// Add mandatory parameters
	dm.Add("Origin-Host", ci.DiameterServerConf().DiameterHost)
	dm.Add("Origin-Realm", ci.DiameterServerConf().DiameterRealm)
	return dm
}

///////////////////////////////////////////////////////////////
// Message constructors
///////////////////////////////////////////////////////////////

// Builds a DiameterRequest with the specified application and command names
func NewDiameterRequest(appName string, commandName string) (*DiameterMessage, error) {

	diameterMessage := DiameterMessage{IsRequest: true}

	// Find element in dictionary
	appDict, ok := GetDDict().AppByName[appName]
	if !ok {
		return &diameterMessage, fmt.Errorf("application %s not found", appName)
	}

	commandDict, ok := appDict.CommandByName[commandName]
	if !ok {
		return &diameterMessage, fmt.Errorf("command %s not found in application %s", commandName, appName)
	}

	diameterMessage.ApplicationName = appName
	diameterMessage.ApplicationId = appDict.Code
	diameterMessage.CommandName = commandDict.Name
	diameterMessage.CommandCode = commandDict.Code

	diameterMessage.HopByHopId = getHopByHopId()
	diameterMessage.E2EId = getE2EId()

	// E2EId and HopByHopId are filled out later
	return &diameterMessage, nil
}

// Builds a new answer diameter message, corresponding to the request passed
// as attribute, that is, same application id and command code, same
// end-to-end id and hop-by-hop id
func NewDiameterAnswer(diameterRequest *DiameterMessage) *DiameterMessage {

	diameterMessage := DiameterMessage{IsRequest: false}

	diameterMessage.ApplicationId = diameterRequest.ApplicationId
	diameterMessage.ApplicationName = diameterRequest.ApplicationName
	diameterMessage.CommandCode = diameterRequest.CommandCode
	diameterMessage.CommandName = diameterRequest.CommandName

	diameterMessage.E2EId = diameterRequest.E2EId
	diameterMessage.HopByHopId = diameterRequest.HopByHopId

	return &diameterMessage
}

// Creates a copy of the diameter message but having only the AVPs in the positiveFilter argument
// or removing the attributes in the negativeFilter argument.If nil, no filter is applied.
func (dm *DiameterMessage) Copy(positiveFilter []string, negativeFilter []string) *DiameterMessage {

	// Build from scratch instead of copying, which will be less efficient
	copiedMessage := DiameterMessage{
		IsRequest:        dm.IsRequest,
		IsProxyable:      dm.IsProxyable,
		IsError:          dm.IsError,
		IsRetransmission: dm.IsRetransmission,
		CommandCode:      dm.CommandCode,
		ApplicationId:    dm.ApplicationId,
		E2EId:            dm.E2EId,
		HopByHopId:       dm.HopByHopId,
		CommandName:      dm.CommandName,
		ApplicationName:  dm.ApplicationName,
		AVPs:             make([]DiameterAVP, 0),
	}

	for i := range dm.AVPs {
		if positiveFilter != nil {
			if slices.Contains(positiveFilter, dm.AVPs[i].Name) {
				copiedMessage.AddAVP(&dm.AVPs[i])
			}
		} else if negativeFilter != nil {
			if !slices.Contains(negativeFilter, dm.AVPs[i].Name) {
				copiedMessage.AddAVP(&dm.AVPs[i])
			}
		} else {
			// If both are nil, copy all attributes
			copiedMessage.AddAVP(&dm.AVPs[i])
		}
	}

	return &copiedMessage
}

// Implementation of the stringer interface
// Prints the message as JSON
func (dm DiameterMessage) String() string {
	b, error := json.Marshal(dm)
	if error != nil {
		return ""
	} else {
		return string(b)
	}
}
