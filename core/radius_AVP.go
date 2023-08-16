package core

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ipv6PrefixRegex = regexp.MustCompile(`[0-9a-fA-F:\.]+/[0-9]+`)

// Represents the contents of a Radius Attribute-Value pair
type RadiusAVP struct {
	Code     byte
	VendorId uint32
	Name     string
	Tag      byte

	// Type mapping
	// May be a []byte, string, int64, float64, net.IP or time.Time
	// If set to any other type, an error will be reported
	Value interface{}

	// Dictionary item corresponding to this attribute
	DictItem *RadiusAVPDictItem
}

// AVP in the wire is
//    code: 1 byte
//    length: 1 byte
//    If code == 26
//      vendorId: 4 bytes
//      code: 1 byte
//      length: 1 byte - the length of the code, length and value in the contents of the VSA
//      value - may be prepended by a 1 byte tag and 2 byte salt
//    Else
//      value

// Encrypted attributes are padded with 0s when written, and those bytes are not removed when read from the Reader
// That means the contents will not match

// Builds a radius AVP read from the specified reader.
// Returns the number of bytes read
func (avp *RadiusAVP) FromReader(reader io.Reader, authenticator [16]byte, secret string) (n int64, err error) {

	var avpLen byte
	var vendorCode byte = 0 // If vendor specific, will be not 26 but the specific vendor code
	var vendorLen byte = 0
	var dataLen byte
	var salt [2]byte

	currentIndex := int64(0)

	// Get Code
	if err := binary.Read(reader, binary.BigEndian, &avp.Code); err != nil {
		return 0, err
	}
	currentIndex += 1

	// Get Length
	if err := binary.Read(reader, binary.BigEndian, &avpLen); err != nil {
		return currentIndex, err
	}
	currentIndex += 1

	// If is vendor specific
	if avp.Code == 26 {
		// Get vendorId
		if err := binary.Read(reader, binary.BigEndian, &avp.VendorId); err != nil {
			return currentIndex, err
		}
		currentIndex += 4

		// Get vendorCode
		if err := binary.Read(reader, binary.BigEndian, &vendorCode); err != nil {
			return currentIndex, err
		}
		currentIndex += 1

		// Get vendorLen
		if err := binary.Read(reader, binary.BigEndian, &vendorLen); err != nil {
			return currentIndex, err
		}
		currentIndex += 1

		avp.Code = vendorCode

		// SanityCheck
		if !(vendorLen == avpLen-6) { // Substracting 4 bytes for vendorId, 1 byte for vendorCode and 1 byte for vendorLen
			return currentIndex, fmt.Errorf("bad avp coding. Expected length of vendor specific attribute does not match")
		}

		dataLen = vendorLen - 2 // Substracting 1 byte for vendor code and 1 byte for vendor length

	} else {
		dataLen = avpLen - 2 // Substracting 1 byte for code and 1 byte for length
	}

	// Get the relevant info from the dictionary
	// If not in the dictionary, will get some defaults (unknown code, treated as octect string).
	// For this reason, the error is ignored
	avp.DictItem, _ = GetRDict().GetFromCode(RadiusAVPCode{VendorId: avp.VendorId, Code: avp.Code})
	avp.Name = avp.DictItem.Name

	// Extract tag if the attribute is tagged in the dictionary
	if avp.DictItem.Tagged {
		if err := binary.Read(reader, binary.BigEndian, &avp.Tag); err != nil {
			return currentIndex, err
		}
		currentIndex += 1
		dataLen = dataLen - 1
	}

	// Extract salt if necessary. A salt is used to make encryption more difficult to crack, introducing
	// randomness in each request
	if avp.DictItem.Salted {
		if err := binary.Read(reader, binary.BigEndian, &salt); err != nil {
			return currentIndex, err
		}
		currentIndex += 2
		dataLen = dataLen - 2
	}

	// Sanity check
	if dataLen <= 0 {
		return currentIndex, fmt.Errorf("invalid AVP data length")
	}

	// The contents of the following variables will be replaced if Encrypted or Salted, so that
	// we'll read from the intermediate buffer, with decrypted contents, instead of the original
	var payloadLen = int(dataLen)
	var payloadReader io.Reader = reader
	var drained = false

	// Parse encrypted/salted attributes
	// read to an intermediate buffer
	if avp.DictItem.Encrypted || avp.DictItem.Salted {

		// Read data
		avpBytes := make([]byte, int(dataLen))
		if n, err := io.ReadAtLeast(reader, avpBytes, int(dataLen)); err != nil {
			return currentIndex + int64(n), err
		}
		currentIndex += int64(dataLen)

		// Decrypt
		if avp.DictItem.Salted {
			avpBytes = decrypt1(avpBytes, authenticator, secret, salt[:])
		} else if avp.DictItem.Encrypted {
			avpBytes = decrypt1(avpBytes, authenticator, secret, nil)
		}

		// If attribute contains its size internally, adjust the length
		if avp.DictItem.WithLen {
			size := avpBytes[0]
			if len(avpBytes) < int(size)+1 {
				return currentIndex, fmt.Errorf("bad internal length value %d < int(%d)+1, Salted: %t", len(avpBytes), size, avp.DictItem.Salted)
			}
			avpBytes = avpBytes[1 : size+1] // The rest of the bytes are just padding

			payloadLen = int(size)
		}

		payloadReader = bytes.NewReader(avpBytes)
		drained = true
	}

	// Parse according to type
	switch avp.DictItem.RadiusType {
	case RadiusTypeNone, RadiusTypeOctets, RadiusTypeString:

		avpBytes := make([]byte, payloadLen)
		if _, err := io.ReadAtLeast(payloadReader, avpBytes, payloadLen); err != nil {
			return currentIndex, err
		}

		if avp.DictItem.RadiusType == RadiusTypeString {
			avp.Value = string(bytes.Trim(avpBytes, "\x00"))
		} else {
			avp.Value = avpBytes
		}

		if !drained {
			currentIndex += int64(payloadLen)
		}

		return currentIndex, nil

	case RadiusTypeInteger:
		if !avp.DictItem.Tagged || avp.DictItem.Salted {
			var value int32
			if err := binary.Read(payloadReader, binary.BigEndian, &value); err != nil {
				return currentIndex, err
			}

			avp.Value = int64(value)

			if !drained {
				currentIndex += 4
			}

			return currentIndex, err
		} else {
			// Standard attributes of this type have 3 bytes for the value only (tagged && not salted?)
			// TODO: Check that only positive integers are represented
			var valueHigh byte
			var valueLow uint16
			if err := binary.Read(payloadReader, binary.BigEndian, &valueHigh); err != nil {
				return currentIndex, err
			}
			if err := binary.Read(reader, binary.BigEndian, &valueLow); err != nil {
				return currentIndex, err
			}
			avp.Value = int64(65536)*int64(valueHigh) + int64(valueLow)

			if !drained {
				currentIndex += 3
			}
			return currentIndex, err
		}

	case RadiusTypeAddress:
		if payloadLen != 4 {
			return currentIndex, fmt.Errorf("address type is not 4 bytes long")
		}
		avpBytes := make([]byte, 4)
		if _, err := io.ReadAtLeast(payloadReader, avpBytes, 4); err != nil {
			return currentIndex, err
		}

		avp.Value = net.IP(avpBytes)

		if !drained {
			currentIndex += 4
		}

		return currentIndex, nil

	case RadiusTypeIPv6Address:
		if payloadLen != 16 {
			return currentIndex, fmt.Errorf("ipv6address type is not 16 bytes long")
		}
		avpBytes := make([]byte, 16)
		if n, err := io.ReadAtLeast(payloadReader, avpBytes, payloadLen); err != nil {
			return currentIndex + int64(n), err
		}
		avp.Value = net.IP(avpBytes)

		if !drained {
			currentIndex += 16
		}
		return currentIndex, nil

	case RadiusTypeTime:
		var value uint32
		if err := binary.Read(payloadReader, binary.BigEndian, &value); err != nil {
			return currentIndex, err
		}
		avp.Value = ZeroRadiusTime.Add(time.Second * time.Duration(value))

		if !drained {
			currentIndex += 4
		}
		return currentIndex, nil

	case RadiusTypeIPv6Prefix:
		// Radius Type IPv6 prefix. Encoded as 1 byte padding, 1 byte prefix length, and 16 bytes with prefix.
		var dummy byte
		var prefixLen byte
		address := make([]byte, 16)
		if err := binary.Read(payloadReader, binary.BigEndian, &dummy); err != nil {
			return currentIndex, err
		}
		if err := binary.Read(payloadReader, binary.BigEndian, &prefixLen); err != nil {
			return currentIndex + 1, err
		}
		if err := binary.Read(payloadReader, binary.BigEndian, &address); err != nil {
			return currentIndex + 2, err
		}

		avp.Value = net.IP(address).String() + "/" + fmt.Sprintf("%d", prefixLen)

		if !drained {
			currentIndex += 18
		}
		return currentIndex, err

	case RadiusTypeInterfaceId:
		// 8 octets
		if payloadLen != 8 {
			return currentIndex, fmt.Errorf("interfaceid type is not 8 bytes long")
		}
		// Read
		avpBytes := make([]byte, int(dataLen))
		if n, err := io.ReadAtLeast(payloadReader, avpBytes, payloadLen); err != nil {
			return currentIndex + int64(n), err
		}

		// Use only dataLen bytes. The rest is padding
		avp.Value = avpBytes

		if !drained {
			currentIndex += 8
		}
		return currentIndex, err

	case RadiusTypeInteger64:
		var value int64
		if err := binary.Read(payloadReader, binary.BigEndian, &value); err != nil {
			return currentIndex, err
		}
		avp.Value = int64(value)

		if !drained {
			currentIndex += 8
		}
		return currentIndex, err

	}

	return currentIndex, fmt.Errorf("unknown type: %d", avp.DictItem.RadiusType)
}

// Writes the AVP to the specified writer
// Returns the number of bytes written including padding
func (avp *RadiusAVP) ToWriter(writer io.Writer, authenticator [16]byte, secret string) (int64, error) {

	var bytesWritten int = 0
	var err error
	var salt [2]byte

	// Write Code
	var code byte
	if avp.VendorId == 0 {
		code = avp.Code
	} else {
		code = 26
	}
	if err = binary.Write(writer, binary.BigEndian, code); err != nil {
		return int64(bytesWritten), err
	}
	bytesWritten += 1

	// Write Length
	avpLen := avp.Len()
	if avpLen > 255 {
		return 0, fmt.Errorf("size of AVP %s is bigger than 255 bytes", avp.Name)
	}
	if err = binary.Write(writer, binary.BigEndian, byte(avpLen)); err != nil {
		return int64(bytesWritten), err
	}
	bytesWritten += 1

	// If vendor specific
	if avp.VendorId != 0 {
		// Write vendorId
		if err = binary.Write(writer, binary.BigEndian, avp.VendorId); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

		// Write vendorCode
		if err = binary.Write(writer, binary.BigEndian, avp.Code); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 1

		// Write length. This is the length of the embedded AVP, which is 6 bytes
		// less, discounting code, len and vendorId
		if err = binary.Write(writer, binary.BigEndian, byte(avpLen-6)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 1
	}

	// Write tag
	if avp.DictItem.Tagged {
		if err = binary.Write(writer, binary.BigEndian, avp.Tag); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 1
	}

	// Write salt
	if avp.DictItem.Salted {
		// Generate random value for salt
		salt = GetSalt()
		if err = binary.Write(writer, binary.BigEndian, salt); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 2
	}

	// If Encrypted or Salted, write to an intermediate buffer instead
	var payloadWriter io.Writer
	var encryptBuffer bytes.Buffer
	var written bool
	if avp.DictItem.Encrypted || avp.DictItem.Salted {
		payloadWriter = &encryptBuffer
		written = false
	} else {
		payloadWriter = writer
		written = true
	}

	// Write data
	switch avp.DictItem.RadiusType {

	case RadiusTypeNone, RadiusTypeOctets, RadiusTypeString:

		var octetsValue []byte
		var stringValue string
		var ok bool
		if avp.DictItem.RadiusType == RadiusTypeString {
			stringValue, ok = avp.Value.(string)
			octetsValue = []byte(stringValue)
		} else {
			octetsValue, ok = avp.Value.([]byte)
		}
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}

		if err = binary.Write(payloadWriter, binary.BigEndian, octetsValue); err != nil {
			return int64(bytesWritten), err
		}

		if written {
			bytesWritten += len(octetsValue)
		}

	case RadiusTypeInteger:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}

		if !avp.DictItem.Tagged || avp.DictItem.Salted {
			if err = binary.Write(payloadWriter, binary.BigEndian, int32(value)); err != nil {
				return int64(bytesWritten), err
			}
			if written {
				bytesWritten += 4
			}

		} else {
			// Use only 3 bytes for the value if tagged
			var highByte = value / 65536
			var lowByte = value % 65536
			if err = binary.Write(payloadWriter, binary.BigEndian, byte(highByte)); err != nil {
				return int64(bytesWritten), err
			}

			if err = binary.Write(payloadWriter, binary.BigEndian, uint32(lowByte)); err != nil {
				return int64(bytesWritten), err
			}

			if written {
				bytesWritten += 3
			}
		}

	case RadiusTypeAddress:
		var ipAddress, ok = avp.Value.(net.IP)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}

		var ipAddressBytes = ipAddress.To4()
		if ipAddressBytes == nil {
			// Was not an IPv4 address
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(payloadWriter, binary.BigEndian, ipAddressBytes); err != nil {
			return int64(bytesWritten), err
		}

		if written {
			bytesWritten += 4
		}

	case RadiusTypeIPv6Address:
		var ipAddress, ok = avp.Value.(net.IP)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}

		var ipAddressBytes = ipAddress.To16()
		if ipAddressBytes == nil {
			// Was not an IPv6 address
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(payloadWriter, binary.BigEndian, ipAddressBytes); err != nil {
			return int64(bytesWritten), err
		}

		if written {
			bytesWritten += 16
		}

	case RadiusTypeTime:
		var timeValue, ok = avp.Value.(time.Time)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(payloadWriter, binary.BigEndian, uint32(timeValue.Sub(ZeroRadiusTime).Seconds())); err != nil {
			return int64(bytesWritten), err
		}

		if written {
			bytesWritten += 4
		}

	case RadiusTypeIPv6Prefix:
		var ipv6Prefix, ok = avp.Value.(string)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		addrPrefix := strings.Split(ipv6Prefix, "/")
		if len(addrPrefix) == 2 {
			prefix, err := strconv.ParseUint(addrPrefix[1], 10, 8) // base 10, 8 bits
			ipv6 := net.ParseIP(addrPrefix[0])
			if err == nil && ipv6 != nil {
				// Dummy byte
				if err = binary.Write(payloadWriter, binary.BigEndian, byte(0)); err != nil {
					return int64(bytesWritten), err
				}

				// Prefix
				if err = binary.Write(payloadWriter, binary.BigEndian, uint8(prefix)); err != nil {
					return int64(bytesWritten), err
				}

				// Address
				binary.Write(payloadWriter, binary.BigEndian, ipv6.To16())

				if written {
					bytesWritten += 18
				}
			} else {
				return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
			}
		} else {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}

	case RadiusTypeInterfaceId:
		var interfaceIdValue, ok = avp.Value.([]byte)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if len(interfaceIdValue) != 8 {
			return int64(bytesWritten), fmt.Errorf("error marshalling interfaceId. length is not 8 bytes")
		}
		if err = binary.Write(payloadWriter, binary.BigEndian, interfaceIdValue); err != nil {
			return int64(bytesWritten), err
		}

		if written {
			bytesWritten += len(interfaceIdValue)
		}

	case RadiusTypeInteger64:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(payloadWriter, binary.BigEndian, value); err != nil {
			return int64(bytesWritten), err
		}

		if written {
			bytesWritten += 8
		}
	}

	// If it was written to intermediate buffer because encryption is needed
	if !written {
		var octetsValue []byte = encryptBuffer.Bytes()

		// Write internal length if so required
		if avp.DictItem.WithLen {
			octetsValue = append([]byte{byte(len(octetsValue))}, octetsValue...)
		}

		// Replace value with encrypted one
		if avp.DictItem.Salted {
			octetsValue = encrypt1(octetsValue, authenticator, secret, salt[:])
		} else if avp.DictItem.Encrypted {
			octetsValue = encrypt1(octetsValue, authenticator, secret, nil)
		}

		// Final write
		if err = binary.Write(writer, binary.BigEndian, octetsValue); err != nil {
			return int64(len(octetsValue)), err
		}

		bytesWritten += len(octetsValue)
	}

	// Saninty check
	if bytesWritten != avpLen {
		panic(fmt.Sprintf("Bad AVP size. Bytes Written: %d, reported size: %d", bytesWritten, avpLen))
	}

	return int64(bytesWritten), nil
}

// Reads a Radius AVP from a buffer
func RadiusAVPFromBytes(inputBytes []byte, authenticator [16]byte, secret string) (RadiusAVP, uint32, error) {
	r := bytes.NewReader(inputBytes)

	avp := RadiusAVP{}
	n, err := avp.FromReader(r, authenticator, secret)
	return avp, uint32(n), err
}

// Returns a byte slice with the contents of the AVP
func (avp *RadiusAVP) ToBytes(authenticator [16]byte, secret string) ([]byte, error) {

	// Will write the output here
	var buffer bytes.Buffer
	_, err := avp.ToWriter(&buffer, authenticator, secret)

	return buffer.Bytes(), err
}

// Returns the size of the AVP
func (avp *RadiusAVP) Len() int {
	var dataSize = 0

	switch avp.DictItem.RadiusType {

	case RadiusTypeNone, RadiusTypeOctets, RadiusTypeString:
		if avp.DictItem.RadiusType == RadiusTypeString {
			dataSize = len(avp.Value.(string))
		} else {
			dataSize = len(avp.Value.([]byte))
		}

		// Add the internal length attribute
		if avp.DictItem.WithLen {
			dataSize += 1
		}

	case RadiusTypeInteger:
		dataSize = 4

	case RadiusTypeAddress:
		dataSize = 4

	case RadiusTypeTime:
		dataSize = 4

	case RadiusTypeIPv6Address:
		dataSize = 16

	case RadiusTypeIPv6Prefix:
		dataSize = 18

	case RadiusTypeInterfaceId:
		dataSize = 8

	case RadiusTypeInteger64:
		dataSize = 8
	}

	// Add the padding that will be introduced by the Encrypt function
	if (avp.DictItem.Encrypted || avp.DictItem.Salted) && dataSize%16 != 0 {
		dataSize = dataSize + (16 - dataSize%16)
	}

	// Add the salt bytes
	if avp.DictItem.Salted {
		dataSize += 2
	}

	// Add the Tag byte. The tag is not encrypted and subject to %16 payload size
	if avp.DictItem.Tagged {
		// If intetger, consumes from the payload (i.e., the integer is 3 bytes only)
		if avp.DictItem.RadiusType != RadiusTypeInteger {
			dataSize += 1
		}
	}

	// Add the header bytes. Only 2 if not VSA and 6 more if VSA
	if avp.VendorId == 0 {
		dataSize += 2
	} else {
		dataSize += 8
	}

	return dataSize
}

/////////////////////////////////////////////
// Value Getters
/////////////////////////////////////////////

// Returns the value of the AVP as a byte slice, nil in case of any errors.
func (avp *RadiusAVP) GetOctets() []byte {

	var value, ok = avp.Value.([]byte)
	if !ok {
		GetLogger().Errorf("cannot convert %T %v to []byte", avp.Value, avp.Value)
		return nil
	}

	return value
}

// Returns the value of the AVP as a string, empty in case of any errors
func (avp *RadiusAVP) GetString() string {

	switch avp.DictItem.RadiusType {

	case RadiusTypeNone, RadiusTypeOctets, RadiusTypeInterfaceId:
		// Treat as octetString
		var octetsValue, _ = avp.Value.([]byte)
		return fmt.Sprintf("%x", octetsValue)

	case RadiusTypeInteger, RadiusTypeInteger64:
		var intValue, _ = avp.Value.(int64)
		if stringValue, ok := avp.DictItem.EnumCodes[int(intValue)]; ok {
			return stringValue
		} else {
			return fmt.Sprintf("%d", intValue)
		}

	case RadiusTypeString, RadiusTypeIPv6Prefix:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case RadiusTypeAddress, RadiusTypeIPv6Address:
		var addressValue, _ = avp.Value.(net.IP)
		return addressValue.String()

	case RadiusTypeTime:
		var timeValue, _ = avp.Value.(time.Time)
		return timeValue.Format(TimeFormatString)
	}

	return ""
}

// Helper to get also the tag. Will not include the tag if
// the attribute is not tagged
func (avp *RadiusAVP) GetTaggedString() string {
	str := avp.GetString()
	if avp.DictItem.Tagged {
		return fmt.Sprintf("%s:%d", str, avp.Tag)
	} else {
		return str
	}
}

// Returns the value of the AVP as a number, 0 in case of any errors.
// Handle with care.
func (avp *RadiusAVP) GetInt() int64 {

	switch avp.DictItem.RadiusType {
	case RadiusTypeInteger, RadiusTypeInteger64:
		value, _ := avp.Value.(int64)
		return value

	case RadiusTypeTime:
		timeValue, _ := avp.Value.(time.Time)
		return int64(timeValue.Sub(ZeroRadiusTime).Seconds())

	case RadiusTypeString:
		value, _ := avp.Value.(string)
		i, _ := strconv.ParseInt(value, 10, 64)
		return i

	default:
		GetLogger().Errorf("cannot convert value to int64 %T %v", avp.Value, avp.Value)
		return 0
	}
}

// Returns the value of the AVP as date
func (avp *RadiusAVP) GetDate() time.Time {

	var value, ok = avp.Value.(time.Time)
	if !ok {
		GetLogger().Errorf("cannot convert %T %v to time", avp.Value, avp.Value)
		return time.Time{}
	}

	return value
}

// Returns the value of the AVP as IP address
func (avp *RadiusAVP) GetIPAddress() net.IP {
	var value, ok = avp.Value.(net.IP)
	if !ok {
		GetLogger().Errorf("cannot convert %T %v to ip address", avp.Value, avp.Value)
		return net.IP{}
	}

	return value
}

// Sets tag on attribute, making sure it is of the appropriate type in the dictionary
func (avp *RadiusAVP) SetTag(tag byte) *RadiusAVP {
	if avp.DictItem.Tagged {
		avp.Tag = tag
	} else {
		GetLogger().Errorf("tried to set tag to %s attribute", avp.DictItem.Name)
	}

	return avp
}

// Simple helper to get the tag
func (avp *RadiusAVP) GetTag() byte {
	return avp.Tag
}

// Creates a new AVP with the specified name and value
// Will return an error if the name is not found in the dictionary
func NewRadiusAVP(name string, value interface{}) (*RadiusAVP, error) {
	var avp = RadiusAVP{}

	di, e := GetRDict().GetFromName(name)
	if e != nil {
		return &avp, fmt.Errorf("%s not found in dictionary", name)
	}

	avp.DictItem = di
	avp.Name = name
	avp.Code = avp.DictItem.Code
	avp.VendorId = avp.DictItem.VendorId

	// Get the tag. Also leaves the work of parsing the interface{} values as string for later, when
	// processing per RadiusAttribute type.
	var stringValue, isString = value.(string)
	if avp.DictItem.Tagged {
		if !isString {
			return &RadiusAVP{}, fmt.Errorf("tried to create a tagged AVP from a non string value")
		}
		stringComponents := strings.Split(stringValue, ":")
		if len(stringComponents) == 2 {
			tag, err := strconv.ParseUint(stringComponents[1], 10, 8)
			if err != nil {
				return &RadiusAVP{}, fmt.Errorf("could not decode tag %s", stringComponents[1])
			}
			avp.Tag = byte(tag)
			stringValue = stringComponents[0]
		} else {
			return &RadiusAVP{}, fmt.Errorf("%s is tagged but no tag found", name)
		}
	}

	var err error
	switch avp.DictItem.RadiusType {
	case RadiusTypeOctets, RadiusTypeInterfaceId:
		if isString {
			avp.Value, err = hex.DecodeString(stringValue)
			if err != nil {
				return &RadiusAVP{}, fmt.Errorf("could not decode %s as hex string", value)
			}
		} else {
			var octetsValue, ok = value.([]byte)
			if !ok {
				return &RadiusAVP{}, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			}
			avp.Value = octetsValue
		}

	case RadiusTypeInteger, RadiusTypeInteger64:

		if isString {
			// Try dictionary
			if intValue, found := avp.DictItem.EnumValues[stringValue]; !found {
				// Try parse as number
				avp.Value, err = strconv.ParseInt(stringValue, 10, 64)
				if err != nil {
					return &RadiusAVP{}, fmt.Errorf("could not parse %s as integer", stringValue)
				}
			} else {
				avp.Value = int64(intValue)
			}
		} else {
			avp.Value, err = toInt64(value)
			if err != nil {
				return &RadiusAVP{}, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			}
		}

	case RadiusTypeString:
		if isString {
			avp.Value = stringValue
		} else {
			return &RadiusAVP{}, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
		}

	case RadiusTypeAddress, RadiusTypeIPv6Address:

		if isString {
			addressValue := net.ParseIP(stringValue)
			if addressValue == nil {
				return &RadiusAVP{}, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			} else {
				avp.Value = addressValue
			}
		} else {
			addressValue, ok := value.(net.IP)
			if !ok {
				return &RadiusAVP{}, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			} else {
				avp.Value = addressValue
			}
		}

	case RadiusTypeTime:
		if isString {
			avp.Value, err = time.Parse(TimeFormatString, stringValue)
			if err != nil {
				return &RadiusAVP{}, fmt.Errorf("error creating radius avp with type %d and value of type %T %s: %s", avp.DictItem.RadiusType, value, value, err)
			}
		} else {
			timeValue, ok := value.(time.Time)
			if !ok {
				return &RadiusAVP{}, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			}
			avp.Value = timeValue
		}

	case RadiusTypeIPv6Prefix:
		if isString {
			if !ipv6PrefixRegex.Match([]byte(stringValue)) {
				return &RadiusAVP{}, fmt.Errorf("ipv6 prefix %s does not match expected format", stringValue)
			}
			avp.Value = stringValue
		} else {
			return &RadiusAVP{}, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
		}

	default:
		return &RadiusAVP{}, fmt.Errorf("%d radius type not known", avp.DictItem.RadiusType)
	}

	return &avp, nil
}

/*
	  On transmission, the password is hidden.  The password is first
      padded at the end with nulls to a multiple of 16 octets.  A one-
      way MD5 hash is calculated over a stream of octets consisting of
      the shared secret followed by the Request Authenticator.  This
      value is XORed with the first 16 octet segment of the password and
      placed in the first 16 octets of the String field of the User-
      Password Attribute.

      If the password is longer than 16 characters, a second one-way MD5
      hash is calculated over a stream of octets consisting of the
      shared secret followed by the result of the first xor.  That hash
      is XORed with the second 16 octet segment of the password and
      placed in the second 16 octets of the String field of the User-
      Password Attribute.

      If necessary, this operation is repeated, with each xor result
      being used along with the shared secret to generate the next hash
      to xor the next segment of the password, to no more than 128
      characters.

      The method is taken from the book "Network Security" by Kaufman,
      Perlman and Speciner [9] pages 109-110.  A more precise
      explanation of the method follows:

      Call the shared secret S and the pseudo-random 128-bit Request
      Authenticator RA.  Break the password into 16-octet chunks p1, p2,
      etc.  with the last one padded at the end with nulls to a 16-octet
      boundary.  Call the ciphertext blocks c(1), c(2), etc.  We'll need
      intermediate values b1, b2, etc.

         b1 = MD5(S + RA)       c(1) = p1 xor b1
         b2 = MD5(S + c(1))     c(2) = p2 xor b2
                .                       .
                .                       .
                .                       .
         bi = MD5(S + c(i-1))   c(i) = pi xor bi

      The String will contain c(1)+c(2)+...+c(i) where + denotes
      concatenation.

      On receipt, the process is reversed to yield the original
      password
*/
/*
	For salted, see https://www.ietf.org/archive/id/draft-ietf-radius-saltencrypt-00.txt
*/

func encrypt1(payload []byte, authenticator [16]byte, secret string, salt []byte) []byte {

	// Calculate padded length
	var upLen = len(payload)
	var pLen int
	if upLen%16 == 0 {
		pLen = upLen
	} else {
		pLen = upLen + (16 - upLen%16)
	}

	var encryptedPayload []byte
	var b, c []byte
	for i := 0; i < pLen; i += 16 {
		// Get the b
		hasher := md5.New()
		hasher.Write([]byte(secret))
		// If first batch of 16 octets, concatenate with authenticator (+ salt), otherwise concatenate with previous c
		if b == nil {
			hasher.Write(authenticator[:])
			hasher.Write(salt)
		} else {
			hasher.Write(c)
		}
		b = hasher.Sum(nil)

		// Encrypt with the calculated c, which is the xor of the payload with the b
		c = make([]byte, 16)
		for j := 0; j < 16; j++ {
			if i+j < upLen {
				c[j] = b[j] ^ payload[i+j]
			} else {
				c[j] = b[j]
			}
		}
		encryptedPayload = append(encryptedPayload, c...)
	}

	return encryptedPayload
}

// The inverse of decrypt1
func decrypt1(payload []byte, authenticator [16]byte, secret string, salt []byte) []byte {

	// Calculate padded length
	var upLen = len(payload)
	var pLen int
	if upLen%16 == 0 {
		pLen = upLen
	} else {
		pLen = upLen + (16 - upLen%16)
	}

	var decryptedPayload []byte
	var b []byte

	// Proceed backwards
	for i := pLen - 16; i >= 0; i -= 16 {
		// Get the b
		hasher := md5.New()
		hasher.Write([]byte(secret))
		if i == 0 {
			// This is the last chunk
			hasher.Write(authenticator[:])
			hasher.Write(salt)
		} else {
			hasher.Write(payload[i-16 : i])
		}
		b = hasher.Sum(nil)

		// Decrypt with the calculated c, which is the xor of the payload with the b
		c := make([]byte, 16)
		for j := 0; j < 16; j++ {
			if i+j < upLen {
				c[j] = b[j] ^ payload[i+j]
			} else {
				c[j] = b[j]
			}
		}
		decryptedPayload = append(c, decryptedPayload...)
	}

	return decryptedPayload
}

///////////////////////////////////////////////////////////////
// JSON Encoding
///////////////////////////////////////////////////////////////

// Encode as JSON using the map representation. It will contain a single property with a single value.
func (avp *RadiusAVP) MarshalJSON() ([]byte, error) {
	return json.Marshal(avp.toMap())
}

// Get a RadiusAVP from JSON containing a single property with a single value.
func (avp *RadiusAVP) UnmarshalJSON(b []byte) error {

	theMap := make(map[string]interface{})

	if err := json.Unmarshal(b, &theMap); err != nil {
		return err
	} else {
		*avp, err = aVPFromMap(theMap)
		return err
	}
}

// Generate a map for JSON encoding
func (avp *RadiusAVP) toMap() map[string]interface{} {
	theMap := map[string]interface{}{}

	switch avp.DictItem.RadiusType {

	case RadiusTypeNone, RadiusTypeOctets, RadiusTypeString, RadiusTypeInterfaceId, RadiusTypeAddress, RadiusTypeIPv6Address, RadiusTypeIPv6Prefix, RadiusTypeTime:
		theMap[avp.Name] = avp.GetTaggedString()

	case RadiusTypeInteger, RadiusTypeInteger64:
		// Try dictionary, if not found use integer value
		var intValue, _ = avp.Value.(int64)
		if stringValue, ok := avp.DictItem.EnumCodes[int(intValue)]; ok {
			theMap[avp.Name] = stringValue
		} else {
			theMap[avp.Name] = avp.GetInt()
		}
	}
	return theMap
}

// Generates a RadiusAVP from its JSON representation
func aVPFromMap(avpMap map[string]interface{}) (RadiusAVP, error) {

	if len(avpMap) != 1 {
		return RadiusAVP{}, fmt.Errorf("map contains more than one key in JSON representation of Diameter AVP")
	}

	// There will be only one entry
	for name := range avpMap {
		avp, err := NewRadiusAVP(name, avpMap[name])
		return *avp, err
	}

	// Unreachable code
	return RadiusAVP{}, fmt.Errorf("ureachable code")
}

// Stringer interface
func (avp RadiusAVP) String() string {
	b, error := avp.MarshalJSON()
	if error != nil {
		return "<error>"
	} else {
		return string(b)
	}
}
