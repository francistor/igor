package diamcodec

/*
Types for encoding and decoding diameter AVP

Requires that the configuration object has been initialized with config.Config.Init(boot, instance), since uses
the Diameter Dictionary
*/

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"igor/config"
	"igor/diamdict"
)

// Magical reference date is Mon Jan 2 15:04:05 MST 2006
// Time AVP is the number of seconds since 1/1/1900
var zeroTime, _ = time.Parse("02/01/2006 15:04:05 UTC", "01/01/1900 00:00:00 UTC")

// A DiameterAVP is created from the name, vendor and value using one of the Diameter<type>AVP() functions
// in this package, or unserialized using DiameterAVPFromValues(). It implements the MarshallBinary function, for
// the BinaryMarshaller interface.
type DiameterAVP struct {
	Code        uint32
	IsMandatory bool
	VendorId    uint32
	Name        string

	// Derived items
	OctetsValue    []byte
	StringValue    string
	LongValue      int64
	FloatValue     float64
	DateValue      time.Time
	IPAddressValue net.IP

	// Embedded AVPs
	Group []DiameterAVP

	// Dictionary item
	DictItem diamdict.AVPDictItem
}

// AVP Header is
//    code: 4 byte
//    flags: 1 byte (vendor, mandatory, proxy)
//    length: 3 byte
//    vendorId: 0 / 4 byte
//    data: rest of bytes

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
		binary.Write(buffer, binary.BigEndian, avp.OctetsValue)

	case diamdict.OctetString:
		binary.Write(buffer, binary.BigEndian, avp.OctetsValue)

	case diamdict.Integer32:
		binary.Write(buffer, binary.BigEndian, int32(avp.LongValue))

	case diamdict.Integer64:
		binary.Write(buffer, binary.BigEndian, int64(avp.LongValue))

	case diamdict.Unsigned32:
		binary.Write(buffer, binary.BigEndian, uint32(avp.LongValue))

	case diamdict.Unsigned64:
		binary.Write(buffer, binary.BigEndian, uint64(avp.LongValue))

	case diamdict.Float32:
		binary.Write(buffer, binary.BigEndian, float32(avp.FloatValue))

	case diamdict.Float64:
		binary.Write(buffer, binary.BigEndian, float64(avp.FloatValue))

	case diamdict.Grouped:
		for _, innerAVP := range avp.Group {
			data, _ := innerAVP.MarshalBinary()
			binary.Write(buffer, binary.BigEndian, data)
		}

	case diamdict.Address:
		if avp.IPAddressValue.To4() != nil {
			// Address Type
			binary.Write(buffer, binary.BigEndian, int16(1))
			binary.Write(buffer, binary.BigEndian, avp.IPAddressValue.To4())
		} else {
			// Address Type
			binary.Write(buffer, binary.BigEndian, int16(2))
			binary.Write(buffer, binary.BigEndian, avp.IPAddressValue.To16())
		}

	case diamdict.Time:
		binary.Write(buffer, binary.BigEndian, uint32(avp.LongValue))

	case diamdict.UTF8String:
		binary.Write(buffer, binary.BigEndian, []byte(avp.StringValue))

	case diamdict.DiamIdent:
		binary.Write(buffer, binary.BigEndian, []byte(avp.StringValue))

	case diamdict.DiameterURI:
		binary.Write(buffer, binary.BigEndian, []byte(avp.StringValue))

	case diamdict.Enumerated:
		binary.Write(buffer, binary.BigEndian, int32(avp.LongValue))

	case diamdict.IPFilterRule:
		binary.Write(buffer, binary.BigEndian, []byte(avp.StringValue))

	case diamdict.IPv4Address:
		binary.Write(buffer, binary.BigEndian, avp.IPAddressValue)

	case diamdict.IPv6Address:
		binary.Write(buffer, binary.BigEndian, avp.IPAddressValue)

	case diamdict.IPv6Prefix:
		addrPrefix := strings.Split(avp.StringValue, "/")
		if len(addrPrefix) == 2 {
			prefix, err := strconv.ParseUint(addrPrefix[0], 10, 2)
			ipv6 := net.ParseIP(addrPrefix[1])
			if err == nil && ipv6 != nil {
				// Prefix
				binary.Write(buffer, binary.BigEndian, uint16(prefix))
				// Address
				binary.Write(buffer, binary.BigEndian, ipv6)
			} else {
				config.IgorLogger.Errorf("bad IPv6 Prefix %s", avp.StringValue)
			}
		} else {
			config.IgorLogger.Errorf("bad IPv6 Prefix %s", avp.StringValue)
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
	isVendorSpecific = flags&0x80 > 0
	avp.IsMandatory = flags&0x40 > 0

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

	// Initialize to avoid nasty errors
	avp.Group = make([]DiameterAVP, 0) // Will Grow as needed

	// Get the relevant info from the dictionary
	// If not in the dictionary, will get some defaults
	avp.DictItem, _ = config.DDict.GetFromCode(diamdict.AVPCode{VendorId: avp.VendorId, Code: avp.Code})
	avp.Name = avp.DictItem.Name

	// Read
	// Verify first to avoid easy panics
	if int(avplen) > len(inputBytes) {
		config.IgorLogger.Errorf("len field too big %d, past bytes in slice %d", avplen, len(avpBytes))
		return avp, 0, fmt.Errorf("len field to big")
	}
	avpBytes = append(avpBytes, inputBytes[avplen-dataLen:avplen]...)

	// Parse according to type
	switch avp.DictItem.DiameterType {
	// None
	case diamdict.None:
		avp.OctetsValue = append(avp.OctetsValue, avpBytes...)

		// OctetString
	case diamdict.OctetString:
		avp.OctetsValue = append(avp.OctetsValue, avpBytes...)
		avp.StringValue = fmt.Sprintf("%x", avpBytes)

		// Int32
	case diamdict.Integer32:
		var value int32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad integer32 value")
			return avp, 0, err
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// Int64
	case diamdict.Integer64:
		var value int64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad integer64 value")
			return avp, 0, err
		}
		avp.LongValue = value
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// UInt32
	case diamdict.Unsigned32:
		var value uint32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad unsigned32 value")
			return avp, 0, err
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// UInt64
		// Stored internally as an int64. This is a limitation!
	case diamdict.Unsigned64:
		var value uint64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad unsigned64 value")
			return avp, 0, err
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// Float32
	case diamdict.Float32:
		var value float32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad float32 value")
			return avp, 0, err
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// Float64
	case diamdict.Float64:
		var value float64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad float64 value")
			return avp, 0, err
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// Grouped
	case diamdict.Grouped:
		currentIndex := avplen - dataLen
		for currentIndex < padLen {
			nextAVP, bytesRead, err := DiameterAVPFromBytes(inputBytes[currentIndex:])
			if err != nil {
				return avp, 0, err
			}
			avp.Group = append(avp.Group, nextAVP)
			currentIndex += bytesRead
		}
		avp.StringValue = avp.GetStringValue()

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
			avp.IPAddressValue = net.IP(ipv4Addr[:])
		} else {
			// IPv6
			var ipv6Addr [16]byte
			if err := binary.Read(reader, binary.BigEndian, &ipv6Addr); err != nil {
				config.IgorLogger.Error("bad address value (decoding ipv6 value)")
				return avp, 0, err
			}
			avp.IPAddressValue = net.IP(ipv6Addr[:])
		}

		avp.StringValue = avp.IPAddressValue.String()

		// Time
	case diamdict.Time:
		var value uint32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad time value")
			return avp, 0, err
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.DateValue = zeroTime.Add(time.Second * time.Duration(value))
		// Magical reference date is Mon Jan 2 15:04:05 MST 2006
		avp.StringValue = avp.DateValue.Format("2006-02-01'T'15:04:05 UTC")

		// UTF8 String
	case diamdict.UTF8String:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad utf8string value")
			return avp, 0, err
		}
		avp.StringValue = string(value)
		avp.LongValue, _ = strconv.ParseInt(avp.StringValue, 10, 64)

		// Diameter Identity
		// Just a string
	case diamdict.DiamIdent:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad diameterint value")
			return avp, 0, err
		}
		avp.StringValue = string(value)

		// Diameter URI
		// Just a string
	case diamdict.DiameterURI:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad diameter uri value")
			return avp, 0, err
		}
		avp.StringValue = string(value)

	case diamdict.Enumerated:
		var value int32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad enumerated value")
			return avp, 0, err
		}
		avp.LongValue = int64(value)
		avp.StringValue = avp.DictItem.EnumCodes[int(value)]

		// IPFilterRule
		// Just a string
	case diamdict.IPFilterRule:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			config.IgorLogger.Error("bad ip filter rule value")
			return avp, 0, err
		}
		avp.StringValue = string(value)

	case diamdict.IPv4Address:
		avp.IPAddressValue = net.IP(avpBytes)
		avp.StringValue = avp.IPAddressValue.String()

	case diamdict.IPv6Address:
		avp.IPAddressValue = net.IP(avpBytes)
		avp.StringValue = avp.IPAddressValue.String()

		// First byte is ignored
		// Second byte is prefix size
		// Rest is an IPv6 Address
	case diamdict.IPv6Prefix:
		var dummy byte
		var prefixLen byte
		address := make([]byte, 16)
		if err := binary.Read(reader, binary.BigEndian, &dummy); err != nil {
			config.IgorLogger.Error("could not write the dummy byte in ipv6 prefix")
			return avp, 0, err
		}
		if err := binary.Read(reader, binary.BigEndian, &prefixLen); err != nil {
			config.IgorLogger.Error("could not write the prefix len byte in ipv6 prefix")
			return avp, 0, err
		}
		if err := binary.Read(reader, binary.BigEndian, &address); err != nil {
			config.IgorLogger.Error("could not write the address in ipv6 prefix")
			return avp, 0, err
		}
		avp.IPAddressValue = net.IP(address)
		avp.StringValue = avp.IPAddressValue.String() + "/" + string(prefixLen)
	}

	return avp, padLen, nil
}

// Builds the string value for Grouped AVP. Works transparently returning the string value for other types
// The format is bracket enclosed name=value pairs, comma separated.
func (avp *DiameterAVP) GetStringValue(level ...int) string {

	var indent = 0
	if len(level) > 0 {
		indent = level[0]
	}

	if len(avp.Group) > 0 {
		var sb strings.Builder

		sb.WriteString("{")
		keyValues := make([]string, 0, len(avp.Group))
		for i := range avp.Group {
			keyValues = append(keyValues, avp.Group[i].Name+"="+avp.Group[i].GetStringValue(indent+1))
		}
		sb.WriteString(strings.Join(keyValues, ","))
		sb.WriteString("}")

		return sb.String()
	} else {

		return avp.StringValue
	}
}

// Assigns the name to an AVP, if found in the dictionary. Also sets the code and vendorid, as
// found in the dictionary.
func (avp *DiameterAVP) SetName(name string) (*DiameterAVP, error) {
	var err error
	avp.DictItem, err = config.DDict.GetFromName(name)
	if err != nil {
		return nil, fmt.Errorf("%s not found in diameter dictionary", name)
	}
	avp.Name = avp.DictItem.Name
	avp.Code = avp.DictItem.Code
	avp.VendorId = avp.DictItem.VendorId

	return avp, nil
}

// All setters for the value of the AVP need to have previously assigned the
// diameter dictionary value, possibly using setName()

// Sets the values of the AVP for all possible types, when the available one
// is an integer
func (avp *DiameterAVP) SetLongValue(value int64) (*DiameterAVP, error) {

	var t int = avp.DictItem.DiameterType

	if t == diamdict.Integer32 ||
		t == diamdict.Integer64 ||
		t == diamdict.Unsigned32 ||
		t == diamdict.Unsigned64 ||
		t == diamdict.Float32 ||
		t == diamdict.Float64 {

		avp.LongValue = value
		avp.FloatValue = float64(value)
		avp.StringValue = strconv.FormatInt(value, 10)
	} else if t == diamdict.Enumerated {
		avp.LongValue = value
		avp.StringValue = avp.DictItem.EnumCodes[int(value)]
	} else {
		return nil, fmt.Errorf("%s is not of numeric type or not found in dictionary", avp.Name)
	}

	return avp, nil
}

// Sets the values of the AVP for all possible types, when the available one
// is a float
func (avp *DiameterAVP) SetFloatValue(value float64) (*DiameterAVP, error) {

	var t int = avp.DictItem.DiameterType

	if t == diamdict.Float32 ||
		t == diamdict.Float64 {

		avp.FloatValue = value
		avp.StringValue = fmt.Sprint(value)

		return avp, nil
	} else {
		return nil, fmt.Errorf("%s is not of float type or not found in dictionary", avp.Name)
	}
}

// Sets the values of the AVP for all possible types, when the available one
// is an IP Address (v4 or v6)
func (avp *DiameterAVP) SetAddressValue(value net.IP) (*DiameterAVP, error) {

	switch avp.DictItem.DiameterType {
	case diamdict.Address:
		avp.IPAddressValue = value
		avp.StringValue = value.String()
		return avp, nil

	case diamdict.IPv4Address:
		if value.To4() != nil {
			avp.IPAddressValue = value
			avp.StringValue = value.String()
			return avp, nil
		} else {
			return nil, fmt.Errorf("%s is not of type IPv4 Address", avp.Name)
		}

	case diamdict.IPv6Address:
		if value.To16() != nil {
			avp.IPAddressValue = value
			avp.StringValue = value.String()
			return avp, nil
		} else {
			return nil, fmt.Errorf("%s is not of type IPv6 Address", avp.Name)
		}
	}

	return nil, fmt.Errorf("%s is not of address type or not found in dictionary", avp.Name)

}

// Sets the values of the AVP for all possible types, when the available one
// is a string
func (avp *DiameterAVP) SetStringValue(value string) (*DiameterAVP, error) {

	var t int = avp.DictItem.DiameterType

	if t == diamdict.None {
		return new(DiameterAVP), fmt.Errorf("%s not found in dictionary", avp.Name)
	}

	switch t {

	case diamdict.OctetString:
		avp.OctetsValue, _ = hex.DecodeString(value)

	case diamdict.Integer32:
		avp.LongValue, _ = strconv.ParseInt(value, 10, 64)
		avp.FloatValue, _ = strconv.ParseFloat(value, 64)

	case diamdict.Integer64:
		avp.LongValue, _ = strconv.ParseInt(value, 10, 64)
		avp.FloatValue, _ = strconv.ParseFloat(value, 64)

	case diamdict.Unsigned32:
		avp.LongValue, _ = strconv.ParseInt(value, 10, 64)
		avp.FloatValue, _ = strconv.ParseFloat(value, 64)

	case diamdict.Unsigned64:
		avp.LongValue, _ = strconv.ParseInt(value, 10, 64)
		avp.FloatValue, _ = strconv.ParseFloat(value, 64)

	case diamdict.Float32:
		avp.FloatValue, _ = strconv.ParseFloat(value, 64)

	case diamdict.Float64:
		avp.FloatValue, _ = strconv.ParseFloat(value, 64)

	case diamdict.Grouped:
		return nil, fmt.Errorf("%s is of grouped type and cannot have a single string value", avp.DictItem.Name)

	case diamdict.Address:
		avp.IPAddressValue = net.ParseIP(value)

	case diamdict.Time:
		var err error
		avp.DateValue, err = time.Parse("2006-01-02T15:04:05", value)
		if err != nil {
			config.IgorLogger.Errorf("Bad AVP Time value: %s", value)
		} else {
			var dateDiff = avp.DateValue.Sub(zeroTime)
			avp.LongValue = int64(dateDiff.Seconds())
		}

	case diamdict.UTF8String:

	case diamdict.DiamIdent:

	case diamdict.DiameterURI:

	case diamdict.Enumerated:
		lv, ok := avp.DictItem.EnumValues[value]
		if ok {
			avp.LongValue = int64(lv)
		} else {
			config.IgorLogger.Errorf("Bad AVP Enumeration value %s for attribute %s", value, avp.DictItem.Name)
		}

	case diamdict.IPFilterRule:

	case diamdict.IPv4Address:
		avp.IPAddressValue = net.ParseIP(value)
		if avp.IPAddressValue == nil {
			config.IgorLogger.Errorf("Bad AVP IPAddress value %s", value)
		}

	case diamdict.IPv6Address:
		avp.IPAddressValue = net.ParseIP(value)
		if avp.IPAddressValue == nil {
			config.IgorLogger.Errorf("Bad AVP IPAddress value %s", value)
		}

	case diamdict.IPv6Prefix:

	}

	avp.StringValue = value

	return avp, nil
}

// Sets the values of the AVP for all possible types, when the available one
// is an octets string
// The string representation is in hex format
func (avp *DiameterAVP) SetOctetsValue(value []byte) (*DiameterAVP, error) {

	if avp.DictItem.DiameterType == diamdict.OctetString {
		avp.OctetsValue = append(avp.OctetsValue, value...)
		avp.StringValue = fmt.Sprintf("%x", value)
		return avp, nil
	} else {
		return nil, fmt.Errorf("%s is not of type OctetString", avp.DictItem.Name)
	}

}

///////////////////////////////////////////////////////////////
// Grouped
///////////////////////////////////////////////////////////////

// Adds a new AVP to the Grouped AVP. Does not check that the type is grouped
func (avp *DiameterAVP) AddAVP(gavp DiameterAVP) *DiameterAVP {
	avp.Group = append(avp.Group, gavp)
	return avp
}

// Deletes all AVP with the specified name
func (avp *DiameterAVP) DeleteAll(name string) *DiameterAVP {
	avpList := make([]DiameterAVP, 0)
	for i := range avp.Group {
		if avp.Group[i].Name != name {
			avpList = append(avpList, avp.Group[i])
		}
	}
	avp.Group = avpList
	return avp
}

// Finds and returns the first AVP found in the group with the specified name
// Notice that a COPY is returned
func (avp *DiameterAVP) GetOneAVP(name string) (DiameterAVP, error) {
	for i := range avp.Group {
		if avp.Group[i].Name == name {
			return avp.Group[i], nil
		}
	}
	return DiameterAVP{}, fmt.Errorf("%s not found", name)
}

// Returns a slice with all AVP with the specified name
// Notice that a COPY is returned
func (avp *DiameterAVP) GetAllAVP(name string) []DiameterAVP {
	avpList := make([]DiameterAVP, 0)
	for i := range avp.Group {
		if avp.Group[i].Name == name {
			avpList = append(avpList, avp.Group[i])
		}
	}
	return avpList
}

/*
AVP Creators from different types
*/

func DiameterStringAVP(name string, value string) (*DiameterAVP, error) {
	var d = new(DiameterAVP)
	d, err := d.SetName(name)
	if err != nil {
		return d, err
	} else {
		return d.SetStringValue(value)
	}
}

func DiameterLongAVP(name string, value int64) (*DiameterAVP, error) {
	var d = new(DiameterAVP)
	d, err := d.SetName(name)
	if err != nil {
		return d, err
	} else {
		return d.SetLongValue(value)
	}
}

func DiameterFloatAVP(name string, value float64) (*DiameterAVP, error) {
	var d = new(DiameterAVP)
	d, err := d.SetName(name)
	if err != nil {
		return d, err
	} else {
		return d.SetFloatValue(value)
	}
}

func DiameterOctetsAVP(name string, value []byte) (*DiameterAVP, error) {
	var d = new(DiameterAVP)
	d, err := d.SetName(name)
	if err != nil {
		return d, err
	} else {
		return d.SetOctetsValue(value)
	}
}

func DiameterIPAddressAVP(name string, value net.IP) (*DiameterAVP, error) {
	var d = new(DiameterAVP)
	d, err := d.SetName(name)
	if err != nil {
		return d, err
	} else {
		return d.SetAddressValue(value)
	}
}

func DiameterGroupedAVP(name string) (*DiameterAVP, error) {
	var d = new(DiameterAVP)
	d, err := d.SetName(name)
	return d, err
}
