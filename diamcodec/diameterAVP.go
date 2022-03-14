package diamcodec

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"igor/config"
	"igor/diamdict"
	"net"
	"strconv"
	"strings"
	"time"
)

// Magical reference date is Mon Jan 2 15:04:05 MST 2006
// Time AVP is the number of seconds since 1/1/1900
var zeroTime, _ = time.Parse("2006-01-02T15:04:05 UTC", "1900-01-01T00:00:00 UTC")
var timeFormatString = "2006-01-02T15:04:05 UTC"

type DiameterAVP struct {
	Code        uint32
	IsMandatory bool
	VendorId    uint32
	Name        string

	// Type mapping
	// May be a []byte, string, int64, float64, net.IP, time.Time or []DiameterAVP
	// If set to any other type, an error will be reported
	Value interface{}

	// Dictionary item
	DictItem diamdict.AVPDictItem
}

// AVP Header is
//    code: 4 byte
//    flags: 1 byte (vendor, mandatory, proxy)
//    length: 3 byte
//    vendorId: 0 / 4 byte
//    data: rest of bytes

// Returns the generated AVP and the bytes read
// If could not parse the bytes, will return an error and the
// stream of read should restart again
func DiameterAVPFromBytes(inputBytes []byte) (DiameterAVP, uint32, error) {
	var avp = DiameterAVP{}

	var lenHigh uint8
	var lenLow uint16
	var avplen uint32 // Only 24 bytes are relevant. Does not take into account 4 byte padding
	var padLen uint32 // Taking into account pad length
	var dataLen uint32
	var flags uint8
	var avpBytes []byte

	var isVendorSpecific bool

	reader := bytes.NewReader(inputBytes)

	// Get Header
	if err := binary.Read(reader, binary.BigEndian, &avp.Code); err != nil {
		config.IgorLogger.Error("could not decode the AVP code field")
		return avp, 0, err
	}

	// Get Flags
	if err := binary.Read(reader, binary.BigEndian, &flags); err != nil {
		config.IgorLogger.Error("could not decode the AVP flags field")
		return avp, 0, err
	}
	isVendorSpecific = flags&0x80 != 0
	avp.IsMandatory = flags&0x40 != 0

	// Get Len
	if err := binary.Read(reader, binary.BigEndian, &lenHigh); err != nil {
		config.IgorLogger.Error("could not decode the AVP len (high) field")
		return avp, 0, err
	}
	if err := binary.Read(reader, binary.BigEndian, &lenLow); err != nil {
		config.IgorLogger.Error("could not decode the len (low) code field")
		return avp, 0, err
	}

	// The Len field contains the full size of the AVP, but not considering the padding
	// Pad until the total length is a multiple of 4
	avplen = uint32(lenHigh)*65535 + uint32(lenLow)
	if avplen%4 == 0 {
		padLen = avplen
	} else {
		padLen = avplen + 4 - (avplen % 4)
	}

	// Get VendorId and data length
	// The size of the data is the size of the AVP minus size of the the headers, which is
	// different depending on whether the attribute is vendor specific or not.

	if isVendorSpecific {
		if err := binary.Read(reader, binary.BigEndian, &avp.VendorId); err != nil {
			config.IgorLogger.Error("could not decode the vendor id code field")
			return avp, 0, err
		}
		dataLen = avplen - 12
	} else {
		dataLen = avplen - 8
	}

	// Get the relevant info from the dictionary
	// If not in the dictionary, will get some defaults
	avp.DictItem, _ = config.DDict.GetFromCode(diamdict.AVPCode{VendorId: avp.VendorId, Code: avp.Code})
	avp.Name = avp.DictItem.Name

	// Read
	// Verify first to avoid easy panics
	if int(avplen) > len(inputBytes) {
		config.IgorLogger.Errorf("len field too big %d, past bytes in slice %d", avplen, len(inputBytes))
		return avp, 0, fmt.Errorf("len field to big")
	}
	avpBytes = append(avpBytes, inputBytes[avplen-dataLen:avplen]...)

	// Parse according to type
	switch avp.DictItem.DiameterType {
	// None
	case diamdict.None:
		// Treat as octetstring
		avp.Value = avpBytes

		// OctetString
	case diamdict.OctetString:
		avp.Value = avpBytes

		// Int32
	case diamdict.Integer32:
		var value int32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad integer32 value")
			return avp, 0, err
		}
		avp.Value = int64(value)

		// Int64
	case diamdict.Integer64:
		var value int64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad integer64 value")
			return avp, 0, err
		}
		avp.Value = int64(value) // Redundant, but nice

		// UInt32
	case diamdict.Unsigned32:
		var value uint32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad unsigned32 value")
			return avp, 0, err
		}
		avp.Value = int64(value)

		// UInt64
		// Stored internally as an int64. This is a limitation!
	case diamdict.Unsigned64:
		var value uint64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad unsigned64 value")
			return avp, 0, err
		}
		avp.Value = int64(value)

		// Float32
	case diamdict.Float32:
		var value float32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad float32 value")
			return avp, 0, err
		}
		avp.Value = float64(value)

		// Float64
	case diamdict.Float64:
		var value float64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad float64 value")
			return avp, 0, err
		}
		avp.Value = float64(value)

		// Grouped
	case diamdict.Grouped:
		currentIndex := avplen - dataLen
		for currentIndex < padLen {
			nextAVP, bytesRead, err := DiameterAVPFromBytes(inputBytes[currentIndex:])
			if err != nil {
				return avp, 0, err
			}
			if avp.Value == nil {
				avp.Value = make([]DiameterAVP, 0)
			}
			avp.Value = append(avp.Value.([]DiameterAVP), nextAVP)
			currentIndex += bytesRead
		}

		// Address
		// Two bytes for address type, and 4 /16 bytes for address
	case diamdict.Address:
		var addrType uint16
		if err := binary.Read(reader, binary.BigEndian, &addrType); err != nil {
			config.IgorLogger.Error("bad address value (decoding type)")
			return avp, 0, err
		}
		if addrType == 1 {
			var ipv4Addr [4]byte
			// IPv4
			if err := binary.Read(reader, binary.BigEndian, &ipv4Addr); err != nil {
				config.IgorLogger.Error("bad address value (decoding ipv4 value)")
				return avp, 0, err
			}
			avp.Value = net.IP(ipv4Addr[:])
		} else {
			// IPv6
			var ipv6Addr [16]byte
			if err := binary.Read(reader, binary.BigEndian, &ipv6Addr); err != nil {
				config.IgorLogger.Error("bad address value (decoding ipv6 value)")
				return avp, 0, err
			}
			avp.Value = net.IP(ipv6Addr[:])
		}

		// Time
	case diamdict.Time:
		var value uint32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad time value")
			return avp, 0, err
		}
		avp.Value = zeroTime.Add(time.Second * time.Duration(value))

		// UTF8 String
	case diamdict.UTF8String:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad utf8string value")
			return avp, 0, err
		}
		avp.Value = string(value)

		// Diameter Identity
		// Just a string
	case diamdict.DiamIdent:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad diameterint value")
			return avp, 0, err
		}
		avp.Value = string(value)

		// Diameter URI
		// Just a string
	case diamdict.DiameterURI:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad diameter uri value")
			return avp, 0, err
		}
		avp.Value = string(value)

	case diamdict.Enumerated:
		var value int32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad enumerated value")
			return avp, 0, err
		}
		avp.Value = int64(value)

		// IPFilterRule
		// Just a string
	case diamdict.IPFilterRule:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad ip filter rule value")
			return avp, 0, err
		}
		avp.Value = string(value)

	case diamdict.IPv4Address:
		avp.Value = net.IP(avpBytes)

	case diamdict.IPv6Address:
		avp.Value = net.IP(avpBytes)

		// First byte is ignored
		// Second byte is prefix size
		// Rest is an IPv6 Address
	case diamdict.IPv6Prefix:
		var dummy byte
		var prefixLen byte
		address := make([]byte, 16)
		if err := binary.Read(reader, binary.BigEndian, &dummy); err != nil {
			config.IgorLogger.Error("could not read the dummy byte in ipv6 prefix")
			return avp, 0, err
		}
		if err := binary.Read(reader, binary.BigEndian, &prefixLen); err != nil {
			config.IgorLogger.Error("could not read the prefix len byte in ipv6 prefix")
			return avp, 0, err
		}
		if err := binary.Read(reader, binary.BigEndian, &address); err != nil {
			config.IgorLogger.Error("could not write the address in ipv6 prefix")
			return avp, 0, err
		}
		avp.Value = net.IP(address).String() + "/" + fmt.Sprintf("%d", prefixLen)
	}

	return avp, padLen, nil
}

// Includes the padding bytes
func (avp *DiameterAVP) MarshalBinary() (data []byte, err error) {

	// Will write the output here
	var buffer = new(bytes.Buffer)

	// Write Code
	binary.Write(buffer, binary.BigEndian, avp.Code)

	// Write Flags
	var flags uint8
	if avp.VendorId > 0 {
		flags += 0x80
	}
	if avp.IsMandatory {
		flags += 0x40
	}
	binary.Write(buffer, binary.BigEndian, flags)

	// Write Len as 0. Will be overriden later
	binary.Write(buffer, binary.BigEndian, uint8(0))
	binary.Write(buffer, binary.BigEndian, uint16(0))
	var avpDataSize = 0

	// Write vendor Id
	if avp.VendorId > 0 {
		binary.Write(buffer, binary.BigEndian, avp.VendorId)
	}

	var initialLen = buffer.Len()

	switch avp.DictItem.DiameterType {

	case diamdict.None:
		// Treat as octetString
		var octetsValue, ok = avp.Value.([]byte)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, octetsValue)

	case diamdict.OctetString:
		var octetsValue, ok = avp.Value.([]byte)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, octetsValue)

	case diamdict.Integer32:
		var value, ok = avp.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, int32(value))

	case diamdict.Integer64:
		var value, ok = avp.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, int64(value))

	case diamdict.Unsigned32:
		var value, ok = avp.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, uint32(value))

	case diamdict.Unsigned64:
		var value, ok = avp.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, uint64(value))

	case diamdict.Float32:
		var value, ok = avp.Value.(float64)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, float32(value))

	case diamdict.Float64:
		var value, ok = avp.Value.(float64)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, float64(value))

	case diamdict.Grouped:
		var groupedValue, ok = avp.Value.([]DiameterAVP)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		for i, _ := range groupedValue {
			data, err := groupedValue[i].MarshalBinary()
			if err != nil {
				return nil, err
			}
			binary.Write(buffer, binary.BigEndian, data)
		}

	case diamdict.Address:
		var addressValue, ok = avp.Value.(net.IP)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if addressValue.To4() != nil {
			// Address Type
			binary.Write(buffer, binary.BigEndian, int16(1))
			binary.Write(buffer, binary.BigEndian, addressValue.To4())
		} else {
			// Address Type
			binary.Write(buffer, binary.BigEndian, int16(2))
			binary.Write(buffer, binary.BigEndian, addressValue.To16())
		}

	case diamdict.Time:
		var timeValue, ok = avp.Value.(time.Time)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, uint32(timeValue.Sub(zeroTime).Seconds()))

	case diamdict.UTF8String:
		var stringValue, ok = avp.Value.(string)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, []byte(stringValue))

	case diamdict.DiamIdent:
		var stringValue, ok = avp.Value.(string)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, []byte(stringValue))

	case diamdict.DiameterURI:
		var stringValue, ok = avp.Value.(string)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, []byte(stringValue))

	case diamdict.Enumerated:
		var value, ok = avp.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, int32(value))

	case diamdict.IPFilterRule:
		var stringValue, ok = avp.Value.(string)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, []byte(stringValue))

	case diamdict.IPv4Address:
		var ipAddress, ok = avp.Value.(net.IP)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, ipAddress.To4())

	case diamdict.IPv6Address:
		var ipAddress, ok = avp.Value.(net.IP)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		binary.Write(buffer, binary.BigEndian, ipAddress.To16())

	case diamdict.IPv6Prefix:
		var ipv6Prefix, ok = avp.Value.(string)
		if !ok {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		addrPrefix := strings.Split(ipv6Prefix, "/")
		if len(addrPrefix) == 2 {
			prefix, err := strconv.ParseUint(addrPrefix[1], 10, 8) // base 10, 8 bits
			ipv6 := net.ParseIP(addrPrefix[0])
			if err == nil && ipv6 != nil {
				// To ignore
				binary.Write(buffer, binary.BigEndian, byte(0))
				// Prefix
				binary.Write(buffer, binary.BigEndian, uint8(prefix))
				// Address
				binary.Write(buffer, binary.BigEndian, ipv6.To16())
			} else {
				return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
			}
		} else {
			return nil, fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
	}
	avpDataSize = buffer.Len() - initialLen

	// Padding
	var padSize = 0
	if avpDataSize%4 != 0 {
		padSize = 4 - (avpDataSize % 4)
		padBytes := make([]byte, padSize)
		binary.Write(buffer, binary.BigEndian, padBytes)
	}

	// Get the total length
	var avpLen = avpDataSize + 8
	if avp.VendorId > 0 {
		avpLen += 4
	}

	// Patch the length
	b := buffer.Bytes()
	b[5] = byte(avpLen / 65535)
	binary.BigEndian.PutUint16(b[6:8], uint16(avpLen%65535))

	return b, nil
}

// Returns the size of the AVP
func (avp *DiameterAVP) Len() int {
	var dataSize = 0

	switch avp.DictItem.DiameterType {

	case diamdict.None:
		dataSize = len(avp.Value.([]byte))

	case diamdict.OctetString:
		dataSize = len(avp.Value.([]byte))

	case diamdict.Integer32:
		dataSize = 4

	case diamdict.Integer64:
		dataSize = 8

	case diamdict.Unsigned32:
		dataSize = 4

	case diamdict.Unsigned64:
		dataSize = 8

	case diamdict.Float32:
		dataSize = 4

	case diamdict.Float64:
		dataSize = 8

	case diamdict.Grouped:
		values := avp.Value.([]DiameterAVP)
		for i := range values {
			dataSize += values[i].Len()
		}

	case diamdict.Address:
		if avp.Value.(net.IP).To4() != nil {
			dataSize = 6
		} else {
			dataSize = 18
		}

	case diamdict.Time:
		dataSize = 4

	case diamdict.UTF8String:
		dataSize = len(avp.Value.(string))

	case diamdict.DiamIdent:
		dataSize = len(avp.Value.(string))

	case diamdict.DiameterURI:
		dataSize = len(avp.Value.(string))

	case diamdict.Enumerated:
		dataSize = 4

	case diamdict.IPFilterRule:
		dataSize = len(avp.Value.(string))

	case diamdict.IPv4Address:
		dataSize = 4

	case diamdict.IPv6Address:
		dataSize = 16

	case diamdict.IPv6Prefix:
		dataSize = 18
	}

	if avp.DictItem.VendorId == 0 {
		dataSize += 8
	} else {
		dataSize += 12
	}

	// Fix to 4 byte boundary
	if dataSize%4 == 0 {
		return dataSize
	} else {
		return dataSize + 4 - (dataSize % 4)
	}
}

/////////////////////////////////////////////
// Value Getters
/////////////////////////////////////////////

func (avp *DiameterAVP) GetOctets() []byte {

	var value, ok = avp.Value.([]byte)
	if !ok {
		config.IgorLogger.Errorf("cannot convert %v to []byte")
		return nil
	}

	return value
}

// Returns the value of the AVP as an string
func (avp *DiameterAVP) GetString() string {

	switch avp.DictItem.DiameterType {

	case diamdict.None:
		// Treat as octetString
		var octetsValue, _ = avp.Value.([]byte)
		return fmt.Sprintf("%x", octetsValue)

	case diamdict.OctetString:
		var octetsValue, _ = avp.Value.([]byte)
		return fmt.Sprintf("%x", octetsValue)

	case diamdict.Integer32, diamdict.Integer64, diamdict.Unsigned32, diamdict.Unsigned64:
		var value, _ = avp.Value.(int64)
		return fmt.Sprintf("%d", value)

	case diamdict.Float32, diamdict.Float64:
		var value, _ = avp.Value.(float64)
		return fmt.Sprintf("%f", value)

	case diamdict.Grouped:
		var groupedValue, _ = avp.Value.([]DiameterAVP)
		var sb strings.Builder

		sb.WriteString("{")
		keyValues := make([]string, 0, len(groupedValue))
		for i := range groupedValue {
			keyValues = append(keyValues, groupedValue[i].Name+"="+groupedValue[i].GetString())
		}
		sb.WriteString(strings.Join(keyValues, ","))
		sb.WriteString("}")

		return sb.String()

	case diamdict.Address:
		var addressValue, _ = avp.Value.(net.IP)
		return addressValue.String()

	case diamdict.Time:
		var timeValue, _ = avp.Value.(time.Time)
		return timeValue.Format(timeFormatString)

	case diamdict.UTF8String:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case diamdict.DiamIdent:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case diamdict.DiameterURI:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case diamdict.Enumerated:
		var intValue, _ = avp.Value.(int)
		return avp.DictItem.EnumCodes[intValue]

	case diamdict.IPFilterRule:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case diamdict.IPv4Address:
		var ipAddress, _ = avp.Value.(net.IP)
		return ipAddress.String()

	case diamdict.IPv6Address:
		var ipAddress, _ = avp.Value.(net.IP)
		return ipAddress.String()

	case diamdict.IPv6Prefix:
		var stringValue, _ = avp.Value.(string)
		return stringValue
	}

	return ""
}

// Returns the value of the AVP as a number
func (avp *DiameterAVP) GetInt() int64 {

	switch avp.DictItem.DiameterType {
	case diamdict.Integer32, diamdict.Integer64, diamdict.Unsigned32, diamdict.Unsigned64, diamdict.Enumerated:

		return avp.Value.(int64)
	default:
		config.IgorLogger.Errorf("cannot convert value to int64 %T", avp.Value)
		return 0
	}
}

// Returns the value of the AVP as a float
func (avp *DiameterAVP) GetFloat() float64 {

	switch avp.DictItem.DiameterType {
	case diamdict.Float32, diamdict.Float64:
		return avp.Value.(float64)
	default:
		config.IgorLogger.Errorf("cannot convert value to float64 %T", avp.Value)
		return 0
	}
}

// Returns the value of the AVP as date
func (avp *DiameterAVP) GetDate() time.Time {

	var value, ok = avp.Value.(time.Time)
	if !ok {
		config.IgorLogger.Errorf("cannot convert %v to time", avp.Value)
		return zeroTime
	}

	return value
}

// Returns the value of the AVP as IP address
func (avp *DiameterAVP) GetIPAddress() net.IP {

	var value, ok = avp.Value.(net.IP)
	if !ok {
		config.IgorLogger.Errorf("cannot convert %v to ip address", avp.Value)
		return net.IP{}
	}

	return value
}

// Creates a new AVP
// If the type of value is not compatible with the Diameter type in the dictionary, an error is returned
func NewAVP(name string, value interface{}) (*DiameterAVP, error) {
	var avp = DiameterAVP{}

	avp.DictItem = config.DDict.AVPByName[name]
	if avp.DictItem.DiameterType == diamdict.None {
		return &avp, fmt.Errorf("%s not found in dictionary", name)
	}

	avp.Name = name
	avp.Code = avp.DictItem.Code
	avp.VendorId = avp.DictItem.VendorId

	switch avp.DictItem.DiameterType {

	case diamdict.OctetString:
		var octetsValue, ok = value.([]byte)
		if !ok {
			var stringValue, ok = value.(string)
			if !ok {
				return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
			}
			var err error
			avp.Value, err = hex.DecodeString(stringValue)
			if err != nil {
				return &avp, fmt.Errorf("could not decode %s as hex string", value)
			}
		} else {
			avp.Value = octetsValue
		}

	case diamdict.Integer32, diamdict.Integer64, diamdict.Unsigned32, diamdict.Unsigned64:
		var value, error = toInt64(value)

		if error != nil {
			return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
		}
		avp.Value = value

	case diamdict.Float32, diamdict.Float64:
		var value, error = toFloat64(value)
		if error != nil {
			return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
		}
		avp.Value = value

	case diamdict.Grouped:
		if value == nil {
			avp.Value = make([]DiameterAVP, 0)
		} else {
			var groupedValue, ok = value.([]DiameterAVP)
			if !ok {
				return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
			}
			avp.Value = groupedValue
		}

	case diamdict.Address, diamdict.IPv4Address, diamdict.IPv6Address:
		// Address and string are allowed
		var addressValue, ok = value.(net.IP)
		if !ok {
			// Try with string
			var stringValue, ok = value.(string)
			if !ok {
				return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
			}
			avp.Value = net.ParseIP(stringValue)
			if avp.Value == nil {
				return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
			}
		} else {
			// Type address
			avp.Value = addressValue
		}

	case diamdict.Time:
		// Time and string are allowed
		var timeValue, ok = value.(time.Time)
		if !ok {
			var stringValue, ok = value.(string)
			if !ok {
				return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
			}
			var err error
			avp.Value, err = time.Parse(timeFormatString, stringValue)
			if err != nil {
				return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T %s: %s", avp.DictItem.DiameterType, value, value, err)
			}
		} else {
			avp.Value = timeValue
		}

	case diamdict.UTF8String:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case diamdict.DiamIdent:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case diamdict.DiameterURI:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case diamdict.Enumerated:
		// Both number and string are allowed
		var int64Value, error = toInt64(value)
		if error != nil {
			var stringValue, ok = value.(string)
			if !ok {
				// Not an int or string
				return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
			}
			var intValue int
			intValue, ok = avp.DictItem.EnumValues[stringValue]
			if !ok {
				return &avp, fmt.Errorf("%s value not in dictionary for %s", stringValue, name)
			}
			avp.Value = int64(intValue)
		} else {
			// Specified as an int
			avp.Value = int64Value
		}

	case diamdict.IPFilterRule:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case diamdict.IPv6Prefix:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	default:
		return &avp, fmt.Errorf("%d diameter type not known", avp.DictItem.DiameterType)
	}

	return &avp, nil
}

func toInt64(value interface{}) (int64, error) {

	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return int64(v), nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	case float32:
		// Needed for unmarshaling JSON
		return int64(v), nil
	case float64:
		// Needed for unmarshaling JSON
		return int64(v), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", value)
	}
}

func toFloat64(value interface{}) (float64, error) {

	switch v := value.(type) {
	case float32:
		return float64(v), nil
	case float64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

///////////////////////////////////////////////////////////////
// Grouped
///////////////////////////////////////////////////////////////

// Adds a new AVP to the Grouped AVP. Does nothing if the current value is not grouped
func (avp *DiameterAVP) AddAVP(gavp DiameterAVP) *DiameterAVP {
	var groupedValue, ok = avp.Value.([]DiameterAVP)
	if !ok {
		config.IgorLogger.Error("value is not of type grouped")
		return avp
	}
	// TODO: verify allowed in dictionary
	avp.Value = append(groupedValue, gavp)
	return avp
}

// Adds a new AVP to the Grouped AVP, specified using name and value. Does nothing if the current value is not grouped
func (avp *DiameterAVP) Add(name string, value interface{}) *DiameterAVP {
	avp, err := NewAVP(name, value)
	if err != nil {
		avp.AddAVP(*avp)
	}
	return avp
}

// Deletes all AVP with the specified name
func (avp *DiameterAVP) DeleteAll(name string) *DiameterAVP {
	var groupedValue, ok = avp.Value.([]DiameterAVP)
	if !ok {
		config.IgorLogger.Error("value is not of type grouped")
		return avp
	}

	avpList := make([]DiameterAVP, 0)
	for i := range groupedValue {
		if groupedValue[i].Name != name {
			avpList = append(avpList, groupedValue[i])
		}
	}
	avp.Value = avpList
	return avp
}

// Finds and returns the first AVP found in the group with the specified name
// Notice that a COPY is returned
func (avp *DiameterAVP) GetAVP(name string) (DiameterAVP, error) {
	var groupedValue, ok = avp.Value.([]DiameterAVP)
	if !ok {
		return DiameterAVP{}, fmt.Errorf("value is not of type grouped %s", name)
	}

	for i := range groupedValue {
		if groupedValue[i].Name == name {
			return groupedValue[i], nil
		}
	}
	return DiameterAVP{}, fmt.Errorf("%s not found", name)
}

// Returns a slice with all AVP with the specified name
// Notice that a COPY is returned
func (avp *DiameterAVP) GetAllAVP(name string) []DiameterAVP {
	var groupedValue, ok = avp.Value.([]DiameterAVP)
	if !ok {
		config.IgorLogger.Error("value is not of type grouped")
		return nil
	}
	avpList := make([]DiameterAVP, 0)
	for i := range groupedValue {
		if groupedValue[i].Name == name {
			avpList = append(avpList, groupedValue[i])
		}
	}
	return avpList
}

// Check that minoccurs and maxoccurs are as specified
func (avp *DiameterAVP) Validate() error {
	return nil
}

///////////////////////////////////////////////////////////////
// JSON Encoding
///////////////////////////////////////////////////////////////

// Generate a map for JSON encoding
func (avp *DiameterAVP) ToMap() map[string]interface{} {
	theMap := map[string]interface{}{}

	switch avp.DictItem.DiameterType {
	case diamdict.None, diamdict.OctetString, diamdict.UTF8String, diamdict.Enumerated, diamdict.DiamIdent, diamdict.DiameterURI, diamdict.Address, diamdict.IPv4Address, diamdict.IPv6Address, diamdict.IPv6Prefix, diamdict.Time, diamdict.IPFilterRule:
		theMap[avp.Name] = avp.GetString()
	case diamdict.Integer32, diamdict.Integer64, diamdict.Unsigned32, diamdict.Unsigned64:
		theMap[avp.Name] = avp.GetInt()
	case diamdict.Float32, diamdict.Float64:
		theMap[avp.Name] = avp.GetFloat()
	case diamdict.Grouped:
		// Grouped AVP. The value is an array of JSON
		targetGroup := make([]map[string]interface{}, 0)
		avpGroup := avp.Value.([]DiameterAVP)
		for i := range avpGroup {
			targetGroup = append(targetGroup, avpGroup[i].ToMap())
		}
		theMap[avp.Name] = targetGroup
	}
	return theMap
}

// Encode as JSON using the map representation
func (avp *DiameterAVP) MarshalJSON() ([]byte, error) {
	return json.Marshal(avp.ToMap())
}

// Generates a DiameterAVP from its JSON representation
func FromMap(avpMap map[string]interface{}) (DiameterAVP, error) {

	if len(avpMap) != 1 {
		return DiameterAVP{}, fmt.Errorf("map contains more than one key in JSON representation of Diameter AVP")
	}

	// There will be only one entry
	for name := range avpMap {

		switch avpValue := avpMap[name].(type) {
		case []interface{}:
			// AVP is Grouped
			groupedAVP, e1 := NewAVP(name, nil)
			if e1 != nil {
				return DiameterAVP{}, fmt.Errorf("could not create AVP with name %s", name)
			}
			// Add inner AVPs
			for i := range avpValue {
				innerAVP, e2 := FromMap(avpValue[i].(map[string]interface{}))
				if e2 != nil {
					return DiameterAVP{}, e2
				}
				groupedAVP.AddAVP(innerAVP)
			}
			return *groupedAVP, nil
		default:
			avp, err := NewAVP(name, avpValue)
			return *avp, err
		}
	}

	return DiameterAVP{}, fmt.Errorf("empty JSON representation of Diameter AVP")
}

// Get a DiameterAVP from JSON
func (avp *DiameterAVP) UnmarshalJSON(b []byte) error {
	theMap := make(map[string]interface{})
	json.Unmarshal(b, &theMap)

	var err error
	*avp, err = FromMap(theMap)
	return err
}

// Stringer interface
func (avp DiameterAVP) String() string {
	b, error := avp.MarshalJSON()
	if error != nil {
		return "<error>"
	} else {
		return string(b)
	}
}
