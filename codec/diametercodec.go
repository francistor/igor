package diamcodec

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

	"go.uber.org/zap"
)

// Logging
var sl *zap.SugaredLogger

func init() {
	// Logging
	logger, _ := zap.NewDevelopment()
	sl = logger.Sugar()
	sl.Infow("Logger initialized")
}

// Magical reference date is Mon Jan 2 15:04:05 MST 2006
var zeroTime, _ = time.Parse("02/01/2006 15:04:05 UTC", "01/01/1900 00:00:00 UTC")

type DiameterAVP struct {
	Code        uint32
	IsMandatory bool
	//Flags    uint8
	VendorId uint32

	// Got from the dictionary
	Name string

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

	// val dataOffset = if (isVendorSpecific) 12 else 8
}

// Builds the string value for Grouped AVP. Works transparently for other types
func (avp *DiameterAVP) buildStringValue() string {

	if len(avp.Group) > 0 {
		var sb strings.Builder

		sb.WriteString("{")
		keyValues := make([]string, len(avp.Group))
		for _, v := range avp.Group {
			keyValues = append(keyValues, v.Name+"="+avp.buildStringValue())
		}
		sb.WriteString(strings.Join(keyValues, ","))
		sb.WriteString("}")

		return sb.String()
	}

	return avp.StringValue
}

func (avp *DiameterAVP) SetName(name string) *DiameterAVP {
	avp.DictItem = config.DDict.GetFromName(name)
	avp.Name = avp.DictItem.Name
	avp.Code = avp.DictItem.Code
	avp.VendorId = avp.DictItem.VendorId

	return avp
}

func (avp *DiameterAVP) SetLongValue(value int64) *DiameterAVP {
	avp.LongValue = value
	avp.FloatValue = float64(value)
	avp.StringValue = strconv.FormatInt(value, 10)

	return avp
}

func (avp *DiameterAVP) SetStringValue(value string) *DiameterAVP {
	avp.StringValue = value

	switch avp.DictItem.DiameterType {

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

	case diamdict.Address:
		avp.IPAddressValue = net.ParseIP(value)

	case diamdict.Time:
		var err error
		avp.DateValue, err = time.Parse("02/01/2006T15:04:05", value)
		if err != nil {
			sl.Errorf("Bad AVP Time value: %s", value)
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
			sl.Errorf("Bad AVP Enumeration value %s for attribute %s", value, avp.DictItem.Name)
		}

	case diamdict.IPFilterRule:

	case diamdict.IPv4Address:
		avp.IPAddressValue = net.ParseIP(value)
		if avp.IPAddressValue == nil {
			sl.Errorf("Bad AVP IPAddress value %s", value)
		}

	case diamdict.IPv6Address:
		avp.IPAddressValue = net.ParseIP(value)
		if avp.IPAddressValue == nil {
			sl.Errorf("Bad AVP IPAddress value %s", value)
		}

	case diamdict.IPv6Prefix:

	}

	return avp
}

func (avp *DiameterAVP) SetOctetsValue(value []byte) *DiameterAVP {

	avp.OctetsValue = append(avp.OctetsValue, value...)
	avp.StringValue = fmt.Sprintf("%x", value)

	return avp
}

func DiameterStringAVP(name string, value string) DiameterAVP {
	return *new(DiameterAVP).SetName(name).SetStringValue(value)
}

func DiameterLongAVP(name string, value int64) DiameterAVP {
	return *new(DiameterAVP).SetName(name).SetLongValue(value)
}

func DiameterOctetsAVP(name string, value []byte) DiameterAVP {
	return *new(DiameterAVP).SetName(name).SetOctetsValue(value)
}

// AVP Header is
//    code: 4 byte
//    flags: 1 byte (vendor, mandatory, proxy)
//    length: 3 byte
//    vendorId: 0 / 4 byte
//    data: rest of bytes

// Includes the padding bytes
func (avp *DiameterAVP) MarshalBinary() (data []byte, err error) {

	if avp.DictItem.Code == 0 {
		return nil, fmt.Errorf("%s not found in dictionary", avp.Name)
	}

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
		if avp.IPAddressValue.To4() == nil {
			// Address Type
			binary.Write(buffer, binary.BigEndian, int16(1))
			binary.Write(buffer, binary.BigEndian, avp.IPAddressValue)
		} else {
			// Address Type
			binary.Write(buffer, binary.BigEndian, int16(2))
			binary.Write(buffer, binary.BigEndian, avp.IPAddressValue)
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
				sl.Errorf("Bad IPv6 Prefix %s", avp.StringValue)
			}
		} else {
			sl.Errorf("Bad IPv6 Prefix %s", avp.StringValue)
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

	// Total length
	var avpLen = padSize + avpDataSize + 8
	if avp.VendorId > 0 {
		avpLen += 4
	}

	b := buffer.Bytes()
	b[5] = byte((avpLen - padSize) / 65535)
	binary.BigEndian.PutUint16(b[6:8], uint16((avpLen-padSize)%65535))

	return b, nil
}

// Returns the generated AVP and the bytes read
// TODO: If error, the bytes read will be zero. This is a non-recoverable error
// and the application should re-start
func DiameterAVPFromBytes(inputBytes []byte) (DiameterAVP, uint32) {
	var avp = DiameterAVP{}
	var lenHigh uint8
	var lenLow uint16
	var len uint32    // Only 24 bytes are relevant. Does not take into account 4 byte padding
	var padLen uint32 // Taking into account pad length
	var dataLen uint32
	var flags uint8
	var avpBytes []byte

	var isVendorSpecific bool

	reader := bytes.NewReader(inputBytes)

	// Get Header
	if err := binary.Read(reader, binary.BigEndian, &avp.Code); err != nil {
		panic("Bad AVP Header")
	}

	// Get Flags
	if err := binary.Read(reader, binary.BigEndian, &flags); err != nil {
		panic("Bad AVP Header")
	}
	isVendorSpecific = flags&0x80 > 0
	avp.IsMandatory = flags&0x40 > 0

	// Get Len
	if err := binary.Read(reader, binary.BigEndian, &lenHigh); err != nil {
		panic("Bad AVP Header")
	}
	if err := binary.Read(reader, binary.BigEndian, &lenLow); err != nil {
		panic("Bad AVP Header")
	}

	// The Len field contains the full size of the AVP, but not considering the padding
	len = uint32(lenHigh)*65535 + uint32(lenLow)
	if len%4 == 0 {
		padLen = len
	} else {
		padLen = len + 4 - (len % 4)
	}

	// Get VendorId and data length
	// The size of the data is the size of the AVP minus size of the the headers, which is
	// different depending on whether the attribute is vendor specific or not.

	if isVendorSpecific {
		if err := binary.Read(reader, binary.BigEndian, &avp.VendorId); err != nil {
			panic("Bad AVP Header")
		}
		dataLen = len - 12
	} else {
		dataLen = len - 8
	}

	// Initialize to avoid nasty errors
	avp.Group = make([]DiameterAVP, 0) // Will Grow as needed

	// Get the relevant info from the dictionary
	// If not in the dictionary, will get some defaults
	avp.DictItem = config.DDict.GetFromCode(diamdict.AVPCode{VendorId: avp.VendorId, Code: avp.Code})
	avp.Name = avp.DictItem.Name

	// Read
	avpBytes = append(avpBytes, inputBytes[len-dataLen:len]...)

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
			panic("Bad Integer32 AVP")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// Int64
	case diamdict.Integer64:
		var value int64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad Integer64 AVP")
		}
		avp.LongValue = value
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// UInt32
	case diamdict.Unsigned32:
		var value uint32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad Unsigned32 AVP")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// UInt64
	case diamdict.Unsigned64:
		var value uint64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad Unsigned64 AVP")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// Float32
	case diamdict.Float32:
		var value float32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad Flota32 AVP")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// Float64
	case diamdict.Float64:
		var value float64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad Float64 AVP")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(value)

		// Grouped
	case diamdict.Grouped:
		currentIndex := len - dataLen
		for currentIndex < padLen {
			nextAVP, bytesRead := DiameterAVPFromBytes(inputBytes[currentIndex:])
			avp.Group = append(avp.Group, nextAVP)
			currentIndex += bytesRead
		}
		avp.StringValue = avp.buildStringValue()

		// Address
		// Two bytes for address type, and 4 /16 bytes for address
	case diamdict.Address:
		var addrType uint16
		if err := binary.Read(reader, binary.BigEndian, &addrType); err != nil {
			panic("Bad Address Type")
		}
		if addrType == 1 {
			var ipv4Addr [4]byte
			// IPv4
			if err := binary.Read(reader, binary.BigEndian, &ipv4Addr); err != nil {
				panic("Bad IPv4 Address AVP")
			}
			avp.IPAddressValue = net.IP(ipv4Addr[:])
		} else {
			// IPv6
			var ipv6Addr [16]byte
			if err := binary.Read(reader, binary.BigEndian, &ipv6Addr); err != nil {
				panic("Bad IPv6 Address AVP")
			}
			avp.IPAddressValue = net.IP(ipv6Addr[:])
		}

		avp.StringValue = avp.IPAddressValue.String()

		// Time
	case diamdict.Time:
		var value uint32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad Time AVP")
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
			panic("Bad String AVP")
		}
		avp.StringValue = string(value)
		avp.LongValue, _ = strconv.ParseInt(avp.StringValue, 10, 64)

		// Diameter Identity
		// Just a string
	case diamdict.DiamIdent:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad DiamIdent AVP")
		}
		avp.StringValue = string(value)

		// Diameter URI
		// Just a string
	case diamdict.DiameterURI:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad DiameterURI AVP")
		}
		avp.StringValue = string(value)

	case diamdict.Enumerated:
		var value int32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad AVP Header")
		}
		avp.LongValue = int64(value)
		avp.StringValue = avp.DictItem.EnumCodes[int(value)]

		// IPFilterRule
		// Just a string
	case diamdict.IPFilterRule:
		value := make([]byte, dataLen)
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad IP Filter Rule AVP")
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
			panic("Bad IPV6Prefix")
		}
		if err := binary.Read(reader, binary.BigEndian, &prefixLen); err != nil {
			panic("Bad IPV6Prefix")
		}
		if err := binary.Read(reader, binary.BigEndian, &address); err != nil {
			panic("Bad IPV6Prefix")
		}
		avp.IPAddressValue = net.IP(address)
		avp.StringValue = avp.IPAddressValue.String() + "/" + string(prefixLen)
	}

	return avp, padLen
}
