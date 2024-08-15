package core

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Types and utility methods for Diameter AVP manipulation
//
// AVP Header in the wire is
//    code: 4 byte
//    flags: 1 byte (vendor, mandatory, protected)
//    length: 3 byte
//    vendorId: 0 / 4 byte
//    data: rest of bytes
// Total length is always padded to a multiple of 4 bytes

// Represents a Diameter AVP. Can be serialized and unserialized
type DiameterAVP struct {
	Code        uint32
	IsMandatory bool
	IsProtected bool
	VendorId    uint32
	Name        string

	// Type mapping
	// May be a []byte, string, int64, float64, net.IP, time.Time or []DiameterAVP
	// If set to any other type, an error will be reported
	Value interface{}

	// Dictionary item
	DictItem *DiameterAVPDictItem
}

// Build a DiameterAVP from a binary byte reader.
// Returns the number of bytes read, plus padding
func (avp *DiameterAVP) ReadFrom(reader io.Reader) (n int64, err error) {
	// The RFC specifies 24 bytes for the length
	// This is the first byte of the length
	var lenHigh uint8
	// The last two bytes of the length
	var lenLow uint16
	// Only 24 bytes are relevant. Does not take into account 4 byte padding
	var avpLen uint32
	// Length of paddding to make multiple of 4 bytes
	var padLen uint32
	// Length of the data
	var dataLen uint32
	// Flags according to the RFC
	var flags uint8

	var avpBytes []byte

	var isVendorSpecific bool

	// We'll increment this as we are reading bytes from the source
	currentIndex := int64(0)

	// Get Code
	if err := binary.Read(reader, binary.BigEndian, &avp.Code); err != nil {
		return 0, err
	}
	currentIndex += 4

	// Get Flags
	if err := binary.Read(reader, binary.BigEndian, &flags); err != nil {
		return currentIndex, err
	}
	// Decode the flags
	isVendorSpecific = flags&0x80 != 0
	avp.IsMandatory = flags&0x40 != 0
	avp.IsProtected = flags&0x20 != 0
	currentIndex += 1

	// Get Len
	if err := binary.Read(reader, binary.BigEndian, &lenHigh); err != nil {
		return currentIndex, err
	}
	currentIndex += 1
	if err := binary.Read(reader, binary.BigEndian, &lenLow); err != nil {
		return currentIndex, err
	}
	currentIndex += 2

	// The Len field contains the full size of the AVP, but not considering the padding
	// Calculate the padding to make total length a multiple of 4
	avpLen = uint32(lenHigh)*65535 + uint32(lenLow)
	if avpLen%4 == 0 {
		padLen = 0
	} else {
		padLen = 4 - (avpLen % 4)
	}

	// Get VendorId and data length
	// The size of the data is the size of the AVP minus size of the the headers, which is
	// different depending on whether the attribute is vendor specific or not.
	if isVendorSpecific {
		if err := binary.Read(reader, binary.BigEndian, &avp.VendorId); err != nil {
			return currentIndex, err
		}
		currentIndex += 4
		dataLen = avpLen - 12
	} else {
		dataLen = avpLen - 8
	}

	// Get the relevant info from the dictionary
	// If not in the dictionary, will get some defaults (error is ignored)
	avp.DictItem, _ = GetDDict().GetAVPFromCode(DiameterAVPCode{VendorId: avp.VendorId, Code: avp.Code})
	avp.Name = avp.DictItem.Name

	// Parse according to type
	switch avp.DictItem.DiameterType {

	// OctetString
	case DiameterTypeNone, DiameterTypeOctetString:
		// Read including padding
		avpBytes = make([]byte, int(dataLen+padLen))
		if bRead, err := io.ReadAtLeast(reader, avpBytes, int(dataLen+padLen)); err != nil {
			return currentIndex + int64(bRead), err
		}

		// Use only dataLen bytes. The rest is padding
		avp.Value = avpBytes[0:dataLen]

		return currentIndex + int64(dataLen+padLen), nil

	// Int32
	case DiameterTypeInteger32:
		var value int32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = int64(value)
		return currentIndex + 4, err

	// Int64
	case DiameterTypeInteger64:
		var value int64
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = int64(value)
		return currentIndex + 8, err

	// UInt32
	case DiameterTypeUnsigned32:
		var value uint32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = int64(value)
		return currentIndex + 4, err

	// UInt64
	// Stored internally as an int64. This is a limitation!
	case DiameterTypeUnsigned64:
		var value uint64
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = int64(value)
		return currentIndex + 8, err

	// Float32
	case DiameterTypeFloat32:
		var value float32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = float64(value)
		return currentIndex + 4, err

	// Float64
	case DiameterTypeFloat64:
		var value float64
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = value
		return currentIndex + 8, err

	// Grouped
	case DiameterTypeGrouped:
		for currentIndex < int64(avpLen+padLen) {
			nextAVP := DiameterAVP{}
			bytesRead, err := nextAVP.ReadFrom(reader)
			if err != nil {
				return currentIndex + bytesRead, err
			}
			// Make array if this is the first attribute we are inserting
			if avp.Value == nil {
				avp.Value = make([]DiameterAVP, 0)
			}
			avp.Value = append(avp.Value.([]DiameterAVP), nextAVP)
			currentIndex += bytesRead
		}

		return currentIndex, err

	// Address
	// Two bytes for address type, and 4 / 16 bytes for address (ipv4 or ipv6)
	case DiameterTypeAddress:
		var addrType uint16
		var padding uint16

		if err := binary.Read(reader, binary.BigEndian, &addrType); err != nil {
			return currentIndex, err
		}
		if addrType == 1 {
			var ipv4Addr [4]byte

			if err := binary.Read(reader, binary.BigEndian, &ipv4Addr); err != nil {
				return currentIndex + 2, err // 2 bytes addrType read
			}
			avp.Value = net.IP(ipv4Addr[:])
			// Drain 2 bytes
			binary.Read(reader, binary.BigEndian, &padding)

			return currentIndex + 8, nil // 2 bytes addrType, 4 bytes IP address, 2 bytes padding
		} else {
			// IPv6
			var ipv6Addr [16]byte
			if err := binary.Read(reader, binary.BigEndian, &ipv6Addr); err != nil {
				return currentIndex + 2, err
			}
			avp.Value = net.IP(ipv6Addr[:])
			// Drain 2 bytes
			binary.Read(reader, binary.BigEndian, &padding)

			return currentIndex + 20, nil // 2 bytes addrType, 16 bytes address, 2 bytes padding
		}

	// Time. Seconds since 1 January 1900
	case DiameterTypeTime:
		var value uint32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = ZeroDiameterTime.Add(time.Second * time.Duration(value))
		return currentIndex + 4, err

	// UTF8 String
	case DiameterTypeUTF8String, DiameterTypeDiamIdent, DiameterTypeDiameterURI, DiameterTypeIPFilterRule:
		// Read including padding
		avpBytes = make([]byte, int(dataLen+padLen))
		if nRead, err := io.ReadAtLeast(reader, avpBytes, int(dataLen+padLen)); err != nil {
			return currentIndex + int64(nRead), err
		}

		// Use only dataLen bytes. The rest is padding
		avp.Value = string(avpBytes[0:dataLen])

		return currentIndex + int64(dataLen+padLen), nil

	case DiameterTypeEnumerated:
		var value int32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = int64(value)
		return currentIndex + 4, err

	case DiameterTypeIPv4Address, DiameterTypeIPv6Address:
		avpBytes = make([]byte, int(dataLen+padLen))
		if nRead, err := io.ReadAtLeast(reader, avpBytes, int(dataLen+padLen)); err != nil {
			return currentIndex + int64(nRead), err
		}
		avp.Value = net.IP(avpBytes)
		return currentIndex + int64(dataLen+padLen), nil

	// First byte is ignored
	// Second byte is prefix size
	// Rest is an IPv6 Address
	case DiameterTypeIPv6Prefix:
		var dummy byte
		var prefixLen byte
		var padding uint16
		address := make([]byte, 16)
		if err := binary.Read(reader, binary.BigEndian, &dummy); err != nil {
			return currentIndex, err
		}
		if err := binary.Read(reader, binary.BigEndian, &prefixLen); err != nil {
			return currentIndex + 1, err
		}
		if err := binary.Read(reader, binary.BigEndian, &address); err != nil {
			return currentIndex + 2, err
		}

		// Drain 2 bytes
		binary.Read(reader, binary.BigEndian, &padding)

		avp.Value = net.IP(address).String() + "/" + fmt.Sprintf("%d", prefixLen)

		return currentIndex + 20, err
	}

	return currentIndex, fmt.Errorf("unknown type: %d", avp.DictItem.DiameterType)
}

// Writes the AVP to the specified writer
// Returns the number of bytes written including padding
func (avp *DiameterAVP) WriteTo(buffer io.Writer) (int64, error) {

	var bytesWritten = 0
	var err error

	// Write Code
	if err = binary.Write(buffer, binary.BigEndian, avp.Code); err != nil {
		return int64(bytesWritten), err
	}
	bytesWritten += 4

	// Write Flags
	var flags uint8
	if avp.VendorId > 0 {
		flags += 0x80
	}
	if avp.IsMandatory {
		flags += 0x40
	}
	if avp.IsProtected {
		flags += 0x20
	}
	if err = binary.Write(buffer, binary.BigEndian, flags); err != nil {
		return int64(bytesWritten), err
	}
	bytesWritten += 1

	// Write Len (this is without padding)
	avpLen := avp.DataLen()

	// Optimization to avoid division in most cases
	var lenHighByte uint8
	var lenLowWord uint16
	if avpLen < 65535 {
		lenHighByte = 0
		lenLowWord = uint16(avpLen)
	} else {
		lenHighByte = uint8(avpLen / 65535)
		lenLowWord = uint16(avpLen % 65535)
	}
	if err = binary.Write(buffer, binary.BigEndian, lenHighByte); err != nil {
		return int64(bytesWritten), err
	}
	if err = binary.Write(buffer, binary.BigEndian, lenLowWord); err != nil {
		return int64(bytesWritten), err
	}
	bytesWritten += 3

	// Write vendor Id
	if avp.VendorId > 0 {
		if err = binary.Write(buffer, binary.BigEndian, avp.VendorId); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4
	}

	switch avp.DictItem.DiameterType {

	case DiameterTypeNone, DiameterTypeOctetString:
		var octetsValue, ok = avp.Value.([]byte)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, octetsValue); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += len(octetsValue)

	case DiameterTypeInteger32:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, int32(value)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case DiameterTypeInteger64:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, int64(value)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 8

	case DiameterTypeUnsigned32:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, uint32(value)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case DiameterTypeUnsigned64:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, uint64(value)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 8

	case DiameterTypeFloat32:
		var value, ok = avp.Value.(float64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, float32(value)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case DiameterTypeFloat64:
		var value, ok = avp.Value.(float64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, float64(value)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 8

	case DiameterTypeGrouped:
		var groupedValue, ok = avp.Value.([]DiameterAVP)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		for i := range groupedValue {
			if n, err := groupedValue[i].WriteTo(buffer); err != nil {
				return int64(bytesWritten) + n, err
			} else {
				bytesWritten += int(n)
			}
		}

	case DiameterTypeAddress:
		var addressValue, ok = avp.Value.(net.IP)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if addressValue.To4() != nil {
			// Address Type
			if err = binary.Write(buffer, binary.BigEndian, int16(1)); err != nil {
				return int64(bytesWritten), err
			}
			if err = binary.Write(buffer, binary.BigEndian, addressValue.To4()); err != nil {
				return int64(bytesWritten), err
			}
			bytesWritten += 6
		} else {
			// Address Type
			if err = binary.Write(buffer, binary.BigEndian, int16(2)); err != nil {
				return int64(bytesWritten), err
			}
			if err = binary.Write(buffer, binary.BigEndian, addressValue.To16()); err != nil {
				return int64(bytesWritten), err
			}
			bytesWritten += 18
		}

	case DiameterTypeTime:
		var timeValue, ok = avp.Value.(time.Time)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, uint32(timeValue.Sub(ZeroDiameterTime).Seconds())); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case DiameterTypeUTF8String, DiameterTypeDiamIdent, DiameterTypeDiameterURI, DiameterTypeIPFilterRule:
		var stringValue, ok = avp.Value.(string)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, []byte(stringValue)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += len(stringValue)

	case DiameterTypeEnumerated:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, int32(value)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case DiameterTypeIPv4Address:
		var ipAddress, ok = avp.Value.(net.IP)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, ipAddress.To4()); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case DiameterTypeIPv6Address:
		var ipAddress, ok = avp.Value.(net.IP)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, ipAddress.To16()); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 16

	case DiameterTypeIPv6Prefix:
		var ipv6Prefix, ok = avp.Value.(string)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		addrPrefix := strings.Split(ipv6Prefix, "/")
		if len(addrPrefix) != 2 {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
		prefix, err := strconv.ParseUint(addrPrefix[1], 10, 8) // base 10, 8 bits
		ipv6 := net.ParseIP(addrPrefix[0])
		if err != nil || ipv6 == nil {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}

		// To ignore
		if err = binary.Write(buffer, binary.BigEndian, byte(0)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 1
		// Prefix
		if err = binary.Write(buffer, binary.BigEndian, uint8(prefix)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 1
		// Address
		binary.Write(buffer, binary.BigEndian, ipv6.To16())
		bytesWritten += 16
	}

	// Saninty check
	if bytesWritten != avpLen {
		panic(fmt.Sprintf("Bad AVP size. Bytes Written: %d, reported size: %d", bytesWritten, avpLen))
	}

	// Padding
	var padSize = 0
	if avpLen%4 != 0 {
		padSize = 4 - (avpLen % 4)
		padBytes := make([]byte, padSize)
		if err = binary.Write(buffer, binary.BigEndian, padBytes); err != nil {
			return int64(bytesWritten), err
		}
	}

	return int64(avpLen + padSize), nil
}

// To implement the BinaryMarshaler interface
func (avp *DiameterAVP) MarshalBinary() (data []byte, err error) {

	// Will write the output here
	var buffer bytes.Buffer
	if _, err := avp.WriteTo(&buffer); err != nil {
		return buffer.Bytes(), err
	}

	return buffer.Bytes(), nil
}

// To implement the BinaryUnmarshaler interface
func (avp *DiameterAVP) UnmarshalBinary(data []byte) error {
	_, err := avp.ReadFrom(bytes.NewReader(data))
	return err
}

// Reads a DiameterAVP from a buffer
func DiameterAVPFromBytes(inputBytes []byte) (*DiameterAVP, uint32, error) {

	avp := DiameterAVP{}
	n, err := avp.ReadFrom(bytes.NewReader(inputBytes))
	return &avp, uint32(n), err
}

// Returns the size of the AVP without padding
func (avp *DiameterAVP) DataLen() int {
	var dataSize = 0

	switch avp.DictItem.DiameterType {

	case DiameterTypeNone, DiameterTypeOctetString:
		dataSize = len(avp.Value.([]byte))

	case DiameterTypeInteger32:
		dataSize = 4

	case DiameterTypeInteger64:
		dataSize = 8

	case DiameterTypeUnsigned32:
		dataSize = 4

	case DiameterTypeUnsigned64:
		dataSize = 8

	case DiameterTypeFloat32:
		dataSize = 4

	case DiameterTypeFloat64:
		dataSize = 8

	case DiameterTypeGrouped:
		values := avp.Value.([]DiameterAVP)
		for i := range values {
			dataSize += values[i].Len() // With padding
		}

	case DiameterTypeAddress:
		if avp.Value.(net.IP).To4() != nil {
			dataSize = 6
		} else {
			dataSize = 18
		}

	case DiameterTypeTime:
		dataSize = 4

	case DiameterTypeUTF8String:
		dataSize = len(avp.Value.(string))

	case DiameterTypeDiamIdent:
		dataSize = len(avp.Value.(string))

	case DiameterTypeDiameterURI:
		dataSize = len(avp.Value.(string))

	case DiameterTypeEnumerated:
		dataSize = 4

	case DiameterTypeIPFilterRule:
		dataSize = len(avp.Value.(string))

	case DiameterTypeIPv4Address:
		dataSize = 4

	case DiameterTypeIPv6Address:
		dataSize = 16

	case DiameterTypeIPv6Prefix:
		dataSize = 18
	}

	if avp.VendorId == 0 {
		dataSize += 8
	} else {
		dataSize += 12
	}

	return dataSize
}

// Returns the size of the AVP with padding
func (avp *DiameterAVP) Len() int {

	dataSize := avp.DataLen()

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

// Returns the value of the AVP as an octet string
func (avp *DiameterAVP) GetOctets() []byte {

	var value, ok = avp.Value.([]byte)
	if !ok {
		GetLogger().Errorf("cannot convert %T %v to []byte", avp.Value, avp.Value)
		return nil
	}

	return value
}

// Returns the value of the AVP as an string
func (avp *DiameterAVP) GetString() string {

	switch avp.DictItem.DiameterType {

	case DiameterTypeNone, DiameterTypeOctetString:
		// Treat as octetString
		var octetsValue, _ = avp.Value.([]byte)
		return fmt.Sprintf("%x", octetsValue)

	case DiameterTypeInteger32, DiameterTypeInteger64, DiameterTypeUnsigned32, DiameterTypeUnsigned64:
		var value, _ = avp.Value.(int64)
		return fmt.Sprintf("%d", value)

	case DiameterTypeFloat32, DiameterTypeFloat64:
		var value, _ = avp.Value.(float64)
		return fmt.Sprintf("%f", value)

	case DiameterTypeGrouped:
		var groupedValue, _ = avp.Value.([]DiameterAVP)

		stringValues := make([]string, 0, len(groupedValue))
		for i := range groupedValue {
			stringValues = append(stringValues, groupedValue[i].Name+"="+groupedValue[i].GetString())
		}

		return "{" + strings.Join(stringValues, ",") + "}"

	case DiameterTypeAddress:
		var addressValue, _ = avp.Value.(net.IP)
		return addressValue.String()

	case DiameterTypeTime:
		var timeValue = avp.Value.(time.Time)
		return timeValue.Format(TimeFormatString)

	case DiameterTypeUTF8String, DiameterTypeDiamIdent, DiameterTypeDiameterURI, DiameterTypeIPFilterRule, DiameterTypeIPv6Prefix:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case DiameterTypeEnumerated:
		var intValue, _ = avp.Value.(int64)
		if value, found := avp.DictItem.EnumCodes[int(intValue)]; !found {
			// If no string representation defined, return the string representation as it is
			return fmt.Sprintf("%d", intValue)
		} else {
			return value
		}

	case DiameterTypeIPv4Address, DiameterTypeIPv6Address:
		var ipAddress, _ = avp.Value.(net.IP)
		return ipAddress.String()
	}

	GetLogger().Errorf("unknown diameter type %d", avp.DictItem.DiameterType)
	return ""
}

// Returns the value of the AVP as a number
// If the value cannot be coerced to an int, returns the zero value
func (avp *DiameterAVP) GetInt() int64 {

	switch avp.DictItem.DiameterType {
	case DiameterTypeInteger32, DiameterTypeInteger64, DiameterTypeUnsigned32, DiameterTypeUnsigned64, DiameterTypeEnumerated:

		return avp.Value.(int64)
	default:
		GetLogger().Errorf("cannot convert value to int64 %T %v", avp.Value, avp.Value)
		return 0
	}
}

// Returns the value of the AVP as a float
// If the value cannot be coerced to float, returns the zero value
func (avp *DiameterAVP) GetFloat() float64 {

	switch avp.DictItem.DiameterType {
	case DiameterTypeFloat32, DiameterTypeFloat64:
		return avp.Value.(float64)
	default:
		GetLogger().Errorf("cannot convert value to float64 %T %v", avp.Value, avp.Value)
		return 0
	}
}

// Returns the value of the AVP as date
// If the value cannot be coerced to a date, returns the zero value
func (avp *DiameterAVP) GetDate() time.Time {

	var value, ok = avp.Value.(time.Time)
	if !ok {
		GetLogger().Errorf("cannot convert %T %v to time", avp.Value, avp.Value)
		return time.Time{}
	}

	return value
}

// Returns the value of the AVP as IP address.
// If the value cannot be coerced to an IP address, returns the zero value
func (avp *DiameterAVP) GetIPAddress() net.IP {

	var value, ok = avp.Value.(net.IP)
	if !ok {
		GetLogger().Errorf("cannot convert %T %v to ip address", avp.Value, avp.Value)
		return net.IP{}
	}

	return value
}

// Creates a new AVP.
// If the type of value is not compatible with the Diameter type in the dictionary, an error is returned.
// If grouped, the value may be an array of AVPs or a single AVP or a pointer to an AVP.
// Time, if passed as string, must be formatted as TimeFormatString
func NewDiameterAVP(name string, value interface{}) (*DiameterAVP, error) {
	var avp = DiameterAVP{}

	di, err := GetDDict().GetAVPFromName(name)
	if err != nil {
		return &avp, fmt.Errorf("%s not found in dictionary", name)
	}

	avp.DictItem = di
	avp.Name = name
	avp.Code = avp.DictItem.Code
	avp.VendorId = avp.DictItem.VendorId

	// Will try to coerce the value to the most appropriate type in turn, before giving up
	switch avp.DictItem.DiameterType {

	case DiameterTypeOctetString:
		var octetsValue, ok = value.([]byte)
		if ok {
			// Native octet string
			avp.Value = octetsValue
		} else {
			// Try to build from string, which should be hex encoded
			if stringValue, ok := value.(string); ok {
				var err error
				avp.Value, err = hex.DecodeString(stringValue)
				if err != nil {
					return &avp, fmt.Errorf("could not decode %s as hex string", value)
				}
			} else {
				// No viable alternative
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
		}

	case DiameterTypeInteger32, DiameterTypeInteger64, DiameterTypeUnsigned32, DiameterTypeUnsigned64:
		var value, error = toInt64(value)

		if error != nil {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}
		avp.Value = value

	case DiameterTypeFloat32, DiameterTypeFloat64:
		var value, error = toFloat64(value)
		if error != nil {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}
		avp.Value = value

	case DiameterTypeGrouped:
		if value == nil {
			avp.Value = make([]DiameterAVP, 0)
		} else {
			if groupedValue, ok := value.([]DiameterAVP); ok {
				// Value was an array of DiameterAVP
				avp.Value = groupedValue
			} else {
				// Try with a single value instead of an array
				if singleValue, ok := value.(DiameterAVP); ok {
					avp.Value = []DiameterAVP{singleValue}
				} else {
					// Try with pointer to DiameterAVP
					if ptrValue, ok := value.(*DiameterAVP); ok {
						avp.Value = []DiameterAVP{*ptrValue}
					}
				}
			}
		}

	case DiameterTypeAddress, DiameterTypeIPv4Address, DiameterTypeIPv6Address:
		// Address and string are allowed
		if addressValue, ok := value.(net.IP); ok {
			// Type address
			avp.Value = addressValue
		} else {
			// Try with string
			if stringValue, ok := value.(string); ok {
				avp.Value = net.ParseIP(stringValue)
				if avp.Value == nil {
					return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
				}
			} else {
				// No viable alternative
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
		}

	case DiameterTypeTime:
		// Time and string are allowed
		if timeValue, ok := value.(time.Time); ok {
			avp.Value = timeValue
		} else {
			// Try with string
			if stringValue, ok := value.(string); ok {
				var err error
				avp.Value, err = time.Parse(TimeFormatString, stringValue)
				if err != nil {
					return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T %s: %s", name, avp.DictItem.DiameterType, value, value, err)
				}
			} else {
				// No viable alternative
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
		}

	case DiameterTypeUTF8String, DiameterTypeDiamIdent, DiameterTypeDiameterURI, DiameterTypeIPFilterRule:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case DiameterTypeEnumerated:
		// Both number and string are allowed
		if int64Value, error := toInt64(value); error == nil {
			// Specified as an int
			avp.Value = int64Value
		} else {
			// Try with string
			if stringValue, ok := value.(string); ok {
				if intValue, ok := avp.DictItem.EnumNames[stringValue]; ok {
					avp.Value = int64(intValue)
				} else {
					return &avp, fmt.Errorf("%s value not in dictionary for %s", stringValue, name)
				}
			} else {
				// Give up
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
		}

	case DiameterTypeIPv6Prefix:
		// Must be a string
		if stringValue, ok := value.(string); ok {
			if !ipv6PrefixRegex.Match([]byte(stringValue)) {
				return &avp, fmt.Errorf("ipv6 prefix %s does not match expected format", stringValue)
			}
			avp.Value = stringValue
		} else {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}

	default:
		return &avp, fmt.Errorf("%d diameter type not known", avp.DictItem.DiameterType)
	}

	return &avp, nil
}

// Diameter AVP Builder that does not return error but nil
func BuildDiameterAVP(name string, value interface{}) *DiameterAVP {
	if avp, err := NewDiameterAVP(name, value); err != nil {
		return nil
	} else {
		return avp
	}
}

// Check conformance of the grouped AVP to the GroupedProperties specification
// If not grouped, only checks that the dictionary item exists
func (avp *DiameterAVP) Check() error {

	diameterType := avp.DictItem.DiameterType

	if diameterType == DiameterTypeGrouped {
		groupedProps := avp.DictItem.Group

		avps := avp.Value.([]DiameterAVP)

		// Build map of number of occurrences of each attribute name and check that name is allowd
		occurs := make(map[string]int, 10)
		for k := range avps {
			if _, found := groupedProps[avps[k].Name]; !found {
				return fmt.Errorf("%s is not allowed in %s", avps[k].Name, avp.Name)
			}
			occurs[avps[k].Name] = occurs[avps[k].Name] + 1
		}

		// Check that occurences are within bounds for each attribute spec
		for attrName, groupSpec := range groupedProps {
			if groupSpec.MinOccurs > 0 && occurs[attrName] < groupSpec.MinOccurs {
				return fmt.Errorf("%s has %d instances which is less than the minimum %d", attrName, occurs[attrName], groupSpec.MinOccurs)
			} else if groupSpec.MaxOccurs > 0 && occurs[attrName] > groupSpec.MaxOccurs {
				return fmt.Errorf("%s has %d instances which is more than the maximum %d", attrName, occurs[attrName], groupSpec.MaxOccurs)
			}
		}

		// Check recursively
		for i := range avps {
			if err := avps[i].Check(); err != nil {
				return err
			}
		}

	} else {
		// If not grouped, check that it is in the dictonary
		if diameterType == DiameterTypeNone {
			return fmt.Errorf("code %d and vendor %d not found in dictionary", avp.Code, avp.VendorId)
		}

		return nil
	}

	return nil
}

///////////////////////////////////////////////////////////////
// Grouped
///////////////////////////////////////////////////////////////

// Adds a new AVP to the Grouped AVP. Does nothing if the current value is not grouped.
// Attributes not specified in the group may be added. Check() should be called on the
// final result.
func (avp *DiameterAVP) AddAVP(gavp *DiameterAVP) *DiameterAVP {

	if gavp == nil {
		return avp
	}

	var groupedValue, ok = avp.Value.([]DiameterAVP)
	if !ok {
		GetLogger().Error("value is not of type grouped")
		return avp
	}

	avp.Value = append(groupedValue, *gavp)
	return avp
}

// Adds multiple AVP to a grouped AVP
func (avp *DiameterAVP) AddAVPs(avps ...*DiameterAVP) *DiameterAVP {
	for _, v := range avps {
		avp.AddAVP(v)
	}
	return avp
}

// Adds a new AVP to the Grouped AVP, specified using name and value. Does nothing if the current value is not grouped
// or if the attribute could not be built
func (avp *DiameterAVP) Add(name string, value interface{}) *DiameterAVP {
	avp, err := NewDiameterAVP(name, value)
	if err != nil {
		avp.AddAVP(avp)
	}
	return avp
}

// Deletes all AVP with the specified name
func (avp *DiameterAVP) DeleteAll(name string) *DiameterAVP {
	var groupedValue, ok = avp.Value.([]DiameterAVP)
	if !ok {
		GetLogger().Error("value is not of type grouped")
		return avp
	}

	avpList := make([]DiameterAVP, 0, len(groupedValue))
	for i := range groupedValue {
		if groupedValue[i].Name != name {
			avpList = append(avpList, groupedValue[i])
		}
	}
	avp.Value = avpList
	return avp
}

// Finds and returns the first AVP found in the group with the specified name.
// Notice that a copy is returned.
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

// Returns a slice with all AVP with the specified name.
// Notice that a copy is returned
func (avp *DiameterAVP) GetAllAVP(name string) []DiameterAVP {
	var groupedValue, ok = avp.Value.([]DiameterAVP)
	if !ok {
		GetLogger().Error("value is not of type grouped")
		return nil
	}
	avpList := make([]DiameterAVP, 0, 1)
	for i := range groupedValue {
		if groupedValue[i].Name == name {
			avpList = append(avpList, groupedValue[i])
		}
	}
	return avpList
}

///////////////////////////////////////////////////////////////
// JSON Encoding
///////////////////////////////////////////////////////////////

// Generate a map of name to the underlaying object for JSON encoding. It is a map with a single key (the name of the AVP)
func (avp *DiameterAVP) toMap() map[string]interface{} {
	theMap := map[string]interface{}{}

	switch avp.DictItem.DiameterType {
	case DiameterTypeNone, DiameterTypeOctetString, DiameterTypeUTF8String, DiameterTypeEnumerated, DiameterTypeDiamIdent, DiameterTypeDiameterURI, DiameterTypeAddress, DiameterTypeIPv4Address, DiameterTypeIPv6Address, DiameterTypeIPv6Prefix, DiameterTypeTime, DiameterTypeIPFilterRule:
		theMap[avp.Name] = avp.GetString()
	case DiameterTypeInteger32, DiameterTypeInteger64, DiameterTypeUnsigned32, DiameterTypeUnsigned64:
		theMap[avp.Name] = avp.GetInt()
	case DiameterTypeFloat32, DiameterTypeFloat64:
		theMap[avp.Name] = avp.GetFloat()
	case DiameterTypeGrouped:
		// Grouped AVP. The value is an array of JSON
		if avpGroup, ok := avp.Value.([]DiameterAVP); ok {
			targetGroup := make([]map[string]interface{}, 0, len(avpGroup))
			for i := range avpGroup {
				targetGroup = append(targetGroup, avpGroup[i].toMap())
			}
			theMap[avp.Name] = targetGroup
		}
	}
	return theMap
}

// Encode as JSON using the map representation
func (avp *DiameterAVP) MarshalJSON() ([]byte, error) {
	return json.Marshal(avp.toMap())
}

// Generates a DiameterAVP from a map of name to the contents of the AVP. Used for JSON decoding
func fromMap(avpMap map[string]interface{}) (DiameterAVP, error) {

	if len(avpMap) != 1 {
		return DiameterAVP{}, fmt.Errorf("map contains more than one key in JSON representation of Diameter AVP")
	}

	// There will be only one entry
	for name := range avpMap {

		switch avpValue := avpMap[name].(type) {
		case []interface{}:
			// AVP is Grouped
			groupedAVP, e1 := NewDiameterAVP(name, nil)
			if e1 != nil {
				return DiameterAVP{}, fmt.Errorf("could not create AVP with name %s", name)
			}
			// Add inner AVPs
			for i := range avpValue {
				innerAVP, e2 := fromMap(avpValue[i].(map[string]interface{}))
				if e2 != nil {
					return DiameterAVP{}, e2
				}
				groupedAVP.AddAVP(&innerAVP)
			}
			return *groupedAVP, nil
		default:
			avp, err := NewDiameterAVP(name, avpValue)
			return *avp, err
		}
	}

	return DiameterAVP{}, fmt.Errorf("empty JSON representation of Diameter AVP")
}

// Get a DiameterAVP from JSON
func (avp *DiameterAVP) UnmarshalJSON(b []byte) error {

	var err error

	theMap := make(map[string]interface{})
	if err = json.Unmarshal(b, &theMap); err != nil {
		return err
	}

	*avp, err = fromMap(theMap)
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
