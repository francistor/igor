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
	DictItem *DiameterAVPDictItem
}

// AVP Header is
//    code: 4 byte
//    flags: 1 byte (vendor, mandatory, proxy)
//    length: 3 byte
//    vendorId: 0 / 4 byte
//    data: rest of bytes

// Returns the number of bytes read, including padding
func (avp *DiameterAVP) ReadFrom(reader io.Reader) (n int64, err error) {
	var lenHigh uint8
	var lenLow uint16
	var avpLen uint32 // Only 24 bytes are relevant. Does not take into account 4 byte padding
	var padLen uint32 // Length of paddding for multiple of 4 bytes
	var dataLen uint32
	var flags uint8
	var avpBytes []byte

	var isVendorSpecific bool

	currentIndex := int64(0)

	// Get Header
	if err := binary.Read(reader, binary.BigEndian, &avp.Code); err != nil {
		return 0, err
	}
	currentIndex += 4

	// Get Flags
	if err := binary.Read(reader, binary.BigEndian, &flags); err != nil {
		return currentIndex, err
	}
	isVendorSpecific = flags&0x80 != 0
	avp.IsMandatory = flags&0x40 != 0
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
	// Pad until the total length is a multiple of 4
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
	// If not in the dictionary, will get some defaults
	avp.DictItem, _ = GetDDict().GetAVPFromCode(DiameterAVPCode{VendorId: avp.VendorId, Code: avp.Code})
	avp.Name = avp.DictItem.Name

	// Parse according to type
	switch avp.DictItem.DiameterType {

	// OctetString
	case DiameterTypeNone, DiameterTypeOctetString:
		// Read including padding
		avpBytes = make([]byte, int(dataLen+padLen))
		_, err := io.ReadAtLeast(reader, avpBytes, int(dataLen+padLen))

		// Use only dataLen bytes. The rest is padding
		avp.Value = avpBytes[0:dataLen]

		return currentIndex + int64(dataLen+padLen), err

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
			if avp.Value == nil {
				avp.Value = make([]DiameterAVP, 0)
			}
			avp.Value = append(avp.Value.([]DiameterAVP), nextAVP)
			currentIndex += bytesRead
		}

		return currentIndex, err

	// Address
	// Two bytes for address type, and 4 /16 bytes for address
	case DiameterTypeAddress:
		var addrType uint16
		var padding uint16
		if err := binary.Read(reader, binary.BigEndian, &addrType); err != nil {
			return currentIndex, err
		}
		if addrType == 1 {
			var ipv4Addr [4]byte
			// IPv4
			if err := binary.Read(reader, binary.BigEndian, &ipv4Addr); err != nil {
				return currentIndex + 2, err
			}
			avp.Value = net.IP(ipv4Addr[:])
			// Drain 2 bytes
			binary.Read(reader, binary.BigEndian, &padding)

			return currentIndex + 8, nil
		} else {
			// IPv6
			var ipv6Addr [16]byte
			if err := binary.Read(reader, binary.BigEndian, &ipv6Addr); err != nil {
				return currentIndex + 2, err
			}
			avp.Value = net.IP(ipv6Addr[:])
			// Drain 2 bytes
			binary.Read(reader, binary.BigEndian, &padding)

			return currentIndex + 20, nil
		}

	// Time
	case DiameterTypeTime:
		var value uint32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = ZeroDiameterTime.Add(time.Second * time.Duration(value))
		return currentIndex + 4, err

	// UTF8 String
	case DiameterTypeUTF8String, DiameterTypeDiamIdent, DiameterTypeDiameterURI, DiameterTypeIPFilterRule:
		// Read including padding
		avpBytes = make([]byte, int(dataLen+padLen))
		_, err := io.ReadAtLeast(reader, avpBytes, int(dataLen+padLen))

		// Use only dataLen bytes. The rest is padding
		avp.Value = string(avpBytes[0:dataLen])

		return currentIndex + int64(dataLen+padLen), err

	case DiameterTypeEnumerated:
		var value int32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = int64(value)
		return currentIndex + 4, err

	case DiameterTypeIPv4Address, DiameterTypeIPv6Address:
		avpBytes = make([]byte, int(dataLen+padLen))
		_, err := io.ReadAtLeast(reader, avpBytes, int(dataLen+padLen))
		avp.Value = net.IP(avpBytes)
		return currentIndex + int64(dataLen+padLen), err

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

// Reads a DiameterAVP from a buffer
func DiameterAVPFromBytes(inputBytes []byte) (DiameterAVP, uint32, error) {
	r := bytes.NewReader(inputBytes)

	avp := DiameterAVP{}
	n, err := avp.ReadFrom(r)
	return avp, uint32(n), err
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
	if err = binary.Write(buffer, binary.BigEndian, flags); err != nil {
		return int64(bytesWritten), err
	}
	bytesWritten += 1

	// Write Len (this is without padding)
	avpLen := avp.DataLen()
	if err = binary.Write(buffer, binary.BigEndian, uint8(avpLen/65535)); err != nil {
		return int64(bytesWritten), err
	}
	if err = binary.Write(buffer, binary.BigEndian, uint16(avpLen%65535)); err != nil {
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
		if len(addrPrefix) == 2 {
			prefix, err := strconv.ParseUint(addrPrefix[1], 10, 8) // base 10, 8 bits
			ipv6 := net.ParseIP(addrPrefix[0])
			if err == nil && ipv6 != nil {
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
			} else {
				return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
			}
		} else {
			return int64(bytesWritten), fmt.Errorf("error marshaling diameter type %d and value %T %v", avp.DictItem.DiameterType, avp.Value, avp.Value)
		}
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

func (avp *DiameterAVP) MarshalBinary() (data []byte, err error) {

	// Will write the output here
	var buffer = new(bytes.Buffer)
	if _, err := avp.WriteTo(buffer); err != nil {
		return buffer.Bytes(), err
	}

	return buffer.Bytes(), nil
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
			dataSize += values[i].Len()
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
		var sb strings.Builder

		sb.WriteString("{")
		keyValues := make([]string, 0, len(groupedValue))
		for i := range groupedValue {
			keyValues = append(keyValues, groupedValue[i].Name+"="+groupedValue[i].GetString())
		}
		sb.WriteString(strings.Join(keyValues, ","))
		sb.WriteString("}")

		return sb.String()

	case DiameterTypeAddress:
		var addressValue, _ = avp.Value.(net.IP)
		return addressValue.String()

	case DiameterTypeTime:
		var timeValue = avp.Value.(time.Time)
		return timeValue.Format(TimeFormatString)

	case DiameterTypeUTF8String:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case DiameterTypeDiamIdent:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case DiameterTypeDiameterURI:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case DiameterTypeEnumerated:
		var intValue, _ = avp.Value.(int64)
		return avp.DictItem.EnumCodes[int(intValue)]

	case DiameterTypeIPFilterRule:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case DiameterTypeIPv4Address:
		var ipAddress, _ = avp.Value.(net.IP)
		return ipAddress.String()

	case DiameterTypeIPv6Address:
		var ipAddress, _ = avp.Value.(net.IP)
		return ipAddress.String()

	case DiameterTypeIPv6Prefix:
		var stringValue, _ = avp.Value.(string)
		return stringValue
	}

	return ""
}

// Returns the value of the AVP as a number
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
func (avp *DiameterAVP) GetDate() time.Time {

	var value, ok = avp.Value.(time.Time)
	if !ok {
		GetLogger().Errorf("cannot convert %T %v to time", avp.Value, avp.Value)
		return time.Time{}
	}

	return value
}

// Returns the value of the AVP as IP address
func (avp *DiameterAVP) GetIPAddress() net.IP {

	var value, ok = avp.Value.(net.IP)
	if !ok {
		GetLogger().Errorf("cannot convert %T %v to ip address", avp.Value, avp.Value)
		return net.IP{}
	}

	return value
}

// Creates a new AVP
// If the type of value is not compatible with the Diameter type in the dictionary, an error is returned
func NewDiameterAVP(name string, value interface{}) (*DiameterAVP, error) {
	var avp = DiameterAVP{}

	di, e := GetDDict().GetAVPFromName(name)
	if e != nil {
		return &avp, fmt.Errorf("%s not found in dictionary", name)
	}

	avp.DictItem = di
	avp.Name = name
	avp.Code = avp.DictItem.Code
	avp.VendorId = avp.DictItem.VendorId

	switch avp.DictItem.DiameterType {

	case DiameterTypeOctetString:
		var octetsValue, ok = value.([]byte)
		if !ok {
			var stringValue, ok = value.(string)
			if !ok {
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
			var err error
			avp.Value, err = hex.DecodeString(stringValue)
			if err != nil {
				return &avp, fmt.Errorf("could not decode %s as hex string", value)
			}
		} else {
			avp.Value = octetsValue
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
			var groupedValue, ok = value.([]DiameterAVP)
			if !ok {
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
			avp.Value = groupedValue
		}

	case DiameterTypeAddress, DiameterTypeIPv4Address, DiameterTypeIPv6Address:
		// Address and string are allowed
		var addressValue, ok = value.(net.IP)
		if !ok {
			// Try with string
			var stringValue, ok = value.(string)
			if !ok {
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
			avp.Value = net.ParseIP(stringValue)
			if avp.Value == nil {
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
		} else {
			// Type address
			avp.Value = addressValue
		}

	case DiameterTypeTime:
		// Time and string are allowed
		var timeValue, ok = value.(time.Time)
		if !ok {
			var stringValue, ok = value.(string)
			if !ok {
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
			}
			var err error
			avp.Value, err = time.Parse(TimeFormatString, stringValue)
			if err != nil {
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T %s: %s", name, avp.DictItem.DiameterType, value, value, err)
			}
		} else {
			avp.Value = timeValue
		}

	case DiameterTypeUTF8String:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case DiameterTypeDiamIdent:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case DiameterTypeDiameterURI:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case DiameterTypeEnumerated:
		// Both number and string are allowed
		var int64Value, error = toInt64(value)
		if error != nil {
			var stringValue, ok = value.(string)
			if !ok {
				// Not an int or string
				return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
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

	case DiameterTypeIPFilterRule:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}
		avp.Value = stringValue

	case DiameterTypeIPv6Prefix:
		var stringValue, ok = value.(string)
		if !ok {
			return &avp, fmt.Errorf("error creating diameter avp %s with type %d and value of type %T", name, avp.DictItem.DiameterType, value)
		}
		if !ipv6PrefixRegex.Match([]byte(stringValue)) {
			return &avp, fmt.Errorf("ipv6 prefix %s does not match expected format", stringValue)
		}
		avp.Value = stringValue

	default:
		return &avp, fmt.Errorf("%d diameter type not known", avp.DictItem.DiameterType)
	}

	return &avp, nil
}

// If grouped, checks that the embedded AVPs are in the dictionary
func (avp *DiameterAVP) Check() error {

	// Do something only if grouped
	if avp.DictItem.DiameterType == DiameterTypeGrouped {
		avps := avp.Value.([]DiameterAVP)
		group := avp.DictItem.Group

		// Check number of occurences as specified in the group
		for attrName, groupSpec := range group {
			nOfInstances := len(avp.GetAllAVP(attrName))
			if groupSpec.MinOccurs > 0 && nOfInstances < groupSpec.MinOccurs {
				return fmt.Errorf("%s has %d instances which is less than the minimum %d", attrName, nOfInstances, groupSpec.MinOccurs)
			} else if groupSpec.MaxOccurs > 0 && nOfInstances > groupSpec.MaxOccurs {
				return fmt.Errorf("%s has %d instances which is more than the maximum %d", attrName, nOfInstances, groupSpec.MaxOccurs)
			}
		}

		// Check that all attribues are allowed
		for i := range avps {
			if _, found := group[avps[i].Name]; !found {
				return fmt.Errorf("%s is not allowed in %s", avps[i].Name, avp.Name)
			}
			if err := avps[i].Check(); err != nil {
				return err
			}
		}
	}

	return nil
}

///////////////////////////////////////////////////////////////
// Grouped
///////////////////////////////////////////////////////////////

// Adds a new AVP to the Grouped AVP. Does nothing if the current value is not grouped
func (avp *DiameterAVP) AddAVP(gavp DiameterAVP) *DiameterAVP {
	var groupedValue, ok = avp.Value.([]DiameterAVP)
	if !ok {
		GetLogger().Error("value is not of type grouped")
		return avp
	}
	// TODO: verify allowed in dictionary
	avp.Value = append(groupedValue, gavp)
	return avp
}

// Adds a new AVP to the Grouped AVP, specified using name and value. Does nothing if the current value is not grouped
func (avp *DiameterAVP) Add(name string, value interface{}) *DiameterAVP {
	avp, err := NewDiameterAVP(name, value)
	if err != nil {
		avp.AddAVP(*avp)
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
		GetLogger().Error("value is not of type grouped")
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

///////////////////////////////////////////////////////////////
// JSON Encoding
///////////////////////////////////////////////////////////////

// Generate a map for JSON encoding
func (avp *DiameterAVP) ToMap() map[string]interface{} {
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
			groupedAVP, e1 := NewDiameterAVP(name, nil)
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
