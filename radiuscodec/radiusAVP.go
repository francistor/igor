package radiuscodec

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"igor/config"
	"igor/radiusdict"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Magical reference date is Mon Jan 2 15:04:05 MST 2006
// Time AVP is the number of seconds since 1/1/1900
var zeroTime, _ = time.Parse("2006-01-02T15:04:05 UTC", "1970-01-01T00:00:00 UTC")
var timeFormatString = "2006-01-02T15:04:05 UTC"
var ipv6PrefixRegex = regexp.MustCompile(`[0-9a-zA-z:.]+/[0-9]+`)

type RadiusAVP struct {
	Code     byte
	VendorId uint32
	Name     string
	Tag      byte

	// Type mapping
	// May be a []byte, string, int64, float64, net.IP, time.Time or []DiameterAVP
	// If set to any other type, an error will be reported
	Value interface{}

	// Dictionary item
	DictItem radiusdict.AVPDictItem
}

// AVP Header is
//    code: 1 byte
//    length: 1 byte
//    If code == 26
//      vendorId: 4 bytes
//      code: 1 byte
//      length: 1 byte - the lenght of the vendorId, code, length and value in the contents of the VSA
//      value - may be prepended by a byte tag
//    Else
//      value

// Returns the number of bytes read
func (avp *RadiusAVP) ReadFrom(reader io.Reader) (n int64, err error) {

	var avpLen byte
	var vendorCode byte = 0
	var vendorLen byte = 0
	var dataLen byte

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
		if !(vendorLen == avpLen-2) {
			return currentIndex, fmt.Errorf("bad avp coding. Expected length of vendor specific attribute does not match")
		}

		dataLen = vendorLen - 6 // Substracting 4 bytes for vendorId, 1 byte for vendorCode and 1 byte for vendorLen

	} else {
		dataLen = avpLen - 2 // Substracting 1 byte for code and 1 byte for length
	}

	// Get the relevant info from the dictionary
	// If not in the dictionary, will get some defaults
	avp.DictItem, _ = config.GetRDict().GetFromCode(radiusdict.AVPCode{VendorId: avp.VendorId, Code: avp.Code})
	avp.Name = avp.DictItem.Name

	// Extract tag if necessary
	if avp.DictItem.Tagged {
		if err := binary.Read(reader, binary.BigEndian, &avp.Tag); err != nil {
			return currentIndex, err
		}
		currentIndex += 1
		dataLen = dataLen - 1
	}

	// Sanity check
	if dataLen <= 0 {
		return currentIndex, fmt.Errorf("invalid AVP data length")
	}

	// Parse according to type
	switch avp.DictItem.RadiusType {
	case radiusdict.None, radiusdict.Octets:

		avpBytes := make([]byte, int(dataLen))
		_, err := io.ReadAtLeast(reader, avpBytes, int(dataLen))

		avp.Value = avpBytes

		return currentIndex + int64(dataLen), err

	case radiusdict.String:
		avpBytes := make([]byte, int(dataLen))
		_, err := io.ReadAtLeast(reader, avpBytes, int(dataLen))

		avp.Value = string(avpBytes)

		return currentIndex + int64(dataLen), err

	case radiusdict.Integer:
		var value int32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = int64(value)
		return currentIndex + 4, err

	case radiusdict.Address:
		if dataLen != 4 {
			return currentIndex, fmt.Errorf("address type is not 4 bytes long")
		}
		avpBytes := make([]byte, 4)
		_, err := io.ReadAtLeast(reader, avpBytes, 4)
		avp.Value = net.IP(avpBytes)
		return currentIndex + 4, err

	case radiusdict.IPv6Address:
		if dataLen != 16 {
			return currentIndex, fmt.Errorf("ipv6address type is not 16 bytes long")
		}
		avpBytes := make([]byte, 16)
		_, err := io.ReadAtLeast(reader, avpBytes, 16)
		avp.Value = net.IP(avpBytes)
		return currentIndex + 16, err

	case radiusdict.Time:
		var value uint32
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = zeroTime.Add(time.Second * time.Duration(value))
		return currentIndex + 4, err

	case radiusdict.IPv6Prefix:
		// Radius Type IPv6 prefix. Encoded as 1 byte padding, 1 byte prefix length, and 16 bytes with prefix.
		var dummy byte
		var prefixLen byte
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

		avp.Value = net.IP(address).String() + "/" + fmt.Sprintf("%d", prefixLen)

		return currentIndex + 18, err

	case radiusdict.InterfaceId:
		// 8 octets
		if dataLen != 8 {
			return currentIndex, fmt.Errorf("interfaceid type is not 8 bytes long")
		}
		// Read
		avpBytes := make([]byte, int(dataLen))
		_, err := io.ReadAtLeast(reader, avpBytes, int(dataLen))

		// Use only dataLen bytes. The rest is padding
		avp.Value = avpBytes

		return currentIndex + 8, err

	case radiusdict.Integer64:
		var value int64
		err := binary.Read(reader, binary.BigEndian, &value)
		avp.Value = int64(value)
		return currentIndex + 8, err

	}

	return currentIndex, fmt.Errorf("unknown type: %d", avp.DictItem.RadiusType)
}

// Reads a DiameterAVP from a buffer
func RadiusAVPFromBytes(inputBytes []byte) (RadiusAVP, uint32, error) {
	r := bytes.NewReader(inputBytes)

	avp := RadiusAVP{}
	n, err := avp.ReadFrom(r)
	return avp, uint32(n), err
}

// Writes the AVP to the specified writer
// Returns the number of bytes written including padding
func (avp *RadiusAVP) WriteTo(buffer io.Writer) (int64, error) {

	var bytesWritten = 0
	var err error

	// Write Code
	var code byte
	if avp.VendorId == 0 {
		code = avp.Code
	} else {
		code = 26
	}
	if err = binary.Write(buffer, binary.BigEndian, code); err != nil {
		return int64(bytesWritten), err
	}
	bytesWritten += 1

	// Write Length
	avpLen := avp.DataLen()
	if err = binary.Write(buffer, binary.BigEndian, avpLen); err != nil {
		return int64(bytesWritten), err
	}
	bytesWritten += 1

	// If vendor specific
	if avp.VendorId != 0 {
		// Write vendorId
		if err = binary.Write(buffer, binary.BigEndian, avp.VendorId); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

		// Write vendorCode
		if err = binary.Write(buffer, binary.BigEndian, avp.Code); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 1

		// Write length. This is the length of the embedded AVP
		if err = binary.Write(buffer, binary.BigEndian, avpLen-2); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 1
	}

	// Write tag
	if avp.DictItem.Tagged {
		if err = binary.Write(buffer, binary.BigEndian, avp.Tag); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 1
	}

	// Write data
	switch avp.DictItem.RadiusType {

	case radiusdict.None, radiusdict.Octets:
		var octetsValue, ok = avp.Value.([]byte)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, octetsValue); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += len(octetsValue)

	case radiusdict.String:
		var stringValue, ok = avp.Value.(string)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, []byte(stringValue)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += len(stringValue)

	case radiusdict.Integer:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, int32(value)); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case radiusdict.Address:
		var ipAddress, ok = avp.Value.(net.IP)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}

		var ipAddressBytes = ipAddress.To4()
		if ipAddressBytes == nil {
			// Was not an IPv4 address
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, ipAddressBytes); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case radiusdict.IPv6Address:
		var ipAddress, ok = avp.Value.(net.IP)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}

		var ipAddressBytes = ipAddress.To16()
		if ipAddressBytes == nil {
			// Was not an IPv6 address
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, ipAddressBytes); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 16

	case radiusdict.Time:
		var timeValue, ok = avp.Value.(time.Time)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, uint32(timeValue.Sub(zeroTime).Seconds())); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 4

	case radiusdict.IPv6Prefix:
		var ipv6Prefix, ok = avp.Value.(string)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
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
				return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
			}
		} else {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}

	case radiusdict.InterfaceId:
		var interfaceIdValue, ok = avp.Value.([]byte)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if len(interfaceIdValue) != 8 {
			return int64(bytesWritten), fmt.Errorf("error marshalling interfaceId. length is not 8 bytes")
		}
		if err = binary.Write(buffer, binary.BigEndian, interfaceIdValue); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += len(interfaceIdValue)

	case radiusdict.Integer64:
		var value, ok = avp.Value.(int64)
		if !ok {
			return int64(bytesWritten), fmt.Errorf("error marshaling radius type %d and value %T %v", avp.DictItem.RadiusType, avp.Value, avp.Value)
		}
		if err = binary.Write(buffer, binary.BigEndian, value); err != nil {
			return int64(bytesWritten), err
		}
		bytesWritten += 8
	}

	// Saninty check
	if byte(bytesWritten) != avpLen {
		panic(fmt.Sprintf("Bad AVP size. Bytes Written: %d, reported size: %d", bytesWritten, avpLen))
	}

	return int64(bytesWritten), nil
}

// Returns a byte slice with the contents of the AVP
func (avp *RadiusAVP) MarshalBinary() (data []byte, err error) {

	// Will write the output here
	var buffer = new(bytes.Buffer)
	if _, err := avp.WriteTo(buffer); err != nil {
		return buffer.Bytes(), err
	}

	return buffer.Bytes(), nil
}

// Returns the size of the AVP without padding
func (avp *RadiusAVP) DataLen() byte {
	var dataSize = 0

	switch avp.DictItem.RadiusType {

	case radiusdict.None, radiusdict.Octets:
		dataSize = len(avp.Value.([]byte))

	case radiusdict.String:
		dataSize = len(avp.Value.(string))

	case radiusdict.Integer:
		dataSize = 4

	case radiusdict.Address:
		dataSize = 4

	case radiusdict.Time:
		dataSize = 4

	case radiusdict.IPv6Address:
		dataSize = 16

	case radiusdict.IPv6Prefix:
		dataSize = 18

	case radiusdict.InterfaceId:
		dataSize = 8

	case radiusdict.Integer64:
		dataSize = 8
	}

	if avp.DictItem.Tagged {
		dataSize += 1
	}

	if avp.VendorId == 0 {
		dataSize += 2
	} else {
		dataSize += 8
	}

	return byte(dataSize)
}

/////////////////////////////////////////////
// Value Getters
/////////////////////////////////////////////

func (avp *RadiusAVP) GetOctets() []byte {

	var value, ok = avp.Value.([]byte)
	if !ok {
		config.GetLogger().Errorf("cannot convert %T %v to []byte", avp.Value, avp.Value)
		return nil
	}

	return value
}

// Returns the value of the AVP as an string
func (avp *RadiusAVP) GetString() string {

	switch avp.DictItem.RadiusType {

	case radiusdict.None, radiusdict.Octets, radiusdict.InterfaceId:
		// Treat as octetString
		var octetsValue, _ = avp.Value.([]byte)
		return fmt.Sprintf("%x", octetsValue)

	case radiusdict.Integer, radiusdict.Integer64:
		var intValue, _ = avp.Value.(int64)
		if stringValue, ok := avp.DictItem.EnumCodes[int(intValue)]; ok {
			return stringValue
		} else {
			return fmt.Sprintf("%d", intValue)
		}

	case radiusdict.String, radiusdict.IPv6Prefix:
		var stringValue, _ = avp.Value.(string)
		return stringValue

	case radiusdict.Address, radiusdict.IPv6Address:
		var addressValue, _ = avp.Value.(net.IP)
		return addressValue.String()

	case radiusdict.Time:
		var timeValue, _ = avp.Value.(time.Time)
		return timeValue.Format(timeFormatString)
	}

	return ""
}

// Returns the value of the AVP as a number
func (avp *RadiusAVP) GetInt() int64 {

	switch avp.DictItem.RadiusType {
	case radiusdict.Integer, radiusdict.Integer64:

		return avp.Value.(int64)
	default:
		config.GetLogger().Errorf("cannot convert value to int64 %T %v", avp.Value, avp.Value)
		return 0
	}
}

// Returns the value of the AVP as date
func (avp *RadiusAVP) GetDate() time.Time {

	var value, ok = avp.Value.(time.Time)
	if !ok {
		config.GetLogger().Errorf("cannot convert %T %v to time", avp.Value, avp.Value)
		return time.Time{}
	}

	return value
}

// Returns the value of the AVP as IP address
func (avp *RadiusAVP) GetIPAddress() net.IP {
	var value, ok = avp.Value.(net.IP)
	if !ok {
		config.GetLogger().Errorf("cannot convert %T %v to ip address", avp.Value, avp.Value)
		return net.IP{}
	}

	return value
}

// Sets tag on attribute, making sure it is of the appropriate type in the dictionary
func (avp *RadiusAVP) SetTag(tag byte) *RadiusAVP {
	if avp.DictItem.Tagged {
		avp.Tag = tag
	} else {
		config.GetLogger().Errorf("tried to set tag to %s attribute", avp.DictItem.Name)
	}

	return avp
}

func NewAVP(name string, value interface{}) (*RadiusAVP, error) {
	var avp = RadiusAVP{}

	avp.DictItem = config.GetRDict().AVPByName[name]
	if avp.DictItem.RadiusType == radiusdict.None {
		return &avp, fmt.Errorf("%s not found in dictionary", name)
	}

	avp.Name = name
	avp.Code = avp.DictItem.Code
	avp.VendorId = avp.DictItem.VendorId

	var stringValue, isString = value.(string)
	if avp.DictItem.Tagged {
		if !isString {
			return &avp, fmt.Errorf("tried to create a tagged AVP from a non string value")
		}
		stringComponents := strings.Split(stringValue, ":")
		if len(stringComponents) == 2 {
			tag, err := strconv.ParseUint(stringComponents[1], 10, 8)
			if err != nil {
				return &avp, fmt.Errorf("could not decode tag %s", stringComponents[1])
			}
			avp.Tag = byte(tag)
			stringValue = stringComponents[0]
		} else {
			return &avp, fmt.Errorf("%s is tagged but no tag found", name)
		}
	}

	var err error
	switch avp.DictItem.RadiusType {
	case radiusdict.Octets, radiusdict.InterfaceId:
		if isString {
			avp.Value, err = hex.DecodeString(stringValue)
			if err != nil {
				return &avp, fmt.Errorf("could not decode %s as hex string", value)
			}
		} else {
			var octetsValue, ok = value.([]byte)
			if !ok {
				return &avp, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			}
			avp.Value = octetsValue
		}

	case radiusdict.Integer, radiusdict.Integer64:

		if isString {
			avp.Value, err = strconv.ParseInt(stringValue, 10, 8)
			if err != nil {
				return &avp, fmt.Errorf("could not parse %s as integer", stringValue)
			}
		} else {
			avp.Value, err = toInt64(value)
			if err != nil {
				return &avp, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			}
		}

	case radiusdict.String:
		if isString {
			avp.Value = stringValue
		} else {
			return &avp, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
		}

	case radiusdict.Address, radiusdict.IPv6Address:

		if isString {
			avp.Value = net.ParseIP(stringValue)
			if avp.Value == nil {
				return &avp, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			}
		} else {
			addressValue, ok := value.(net.IP)
			if !ok {
				return &avp, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			} else {
				avp.Value = addressValue
			}
		}

	case radiusdict.Time:
		if isString {
			avp.Value, err = time.Parse(timeFormatString, stringValue)
			if err != nil {
				return &avp, fmt.Errorf("error creating radius avp with type %d and value of type %T %s: %s", avp.DictItem.RadiusType, value, value, err)
			}
		} else {
			timeValue, ok := value.(time.Time)
			if !ok {
				return &avp, fmt.Errorf("error creating radius avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
			}
			avp.Value = timeValue
		}

	case radiusdict.IPv6Prefix:
		if isString {
			if !ipv6PrefixRegex.Match([]byte(stringValue)) {
				return &avp, fmt.Errorf("ipv6 prefix %s does not match expected format", stringValue)
			}
			avp.Value = stringValue
		} else {
			return &avp, fmt.Errorf("error creating diameter avp with type %d and value of type %T", avp.DictItem.RadiusType, value)
		}

	default:
		return &avp, fmt.Errorf("%d radius type not known", avp.DictItem.RadiusType)
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

func Encrypt1(secret string, authenticator []byte, payload []byte) []byte {

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
		if b == nil {
			hasher.Write(authenticator)
		} else {
			hasher.Write(c)
			fmt.Println("<-", c)
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
func Decrypt1(secret string, authenticator []byte, payload []byte) []byte {

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
			hasher.Write(authenticator)
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
