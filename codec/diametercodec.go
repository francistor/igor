package codec

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"igor/config"
	"igor/diamdict"
)

var zeroTime, err = time.Parse("01/01/1900 00:00:00", "01/01/1900 00:00:00")

type DiameterAVP struct {
	Code     uint32
	Flags    uint8
	Len      uint32 // Only 24 bytes are relevant. Does not take into account 4 byte padding
	PadLen   uint32 // Taking into account pad length
	VendorId uint32

	Data        []byte
	StringValue string
	LongValue   int64
	FloatValue  float64
	DateValue   time.Time

	Group []DiameterAVP

	Name string

	// val dataOffset = if (isVendorSpecific) 12 else 8
}

func (avp *DiameterAVP) isVendorSpecific() bool {
	return avp.Flags&0x80 > 0
}

func (avp *DiameterAVP) isMandatory() bool {
	return avp.Flags&0x40 > 0
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

func NewDiameterAVP(inputBytes []byte) DiameterAVP {
	var avp = DiameterAVP{}
	var lenHigh uint16
	var lenLow uint8
	var dataLen uint32

	reader := bytes.NewReader(inputBytes)

	// Get Header
	if err := binary.Read(reader, binary.BigEndian, &avp.Code); err != nil {
		panic("Bad AVP Header")
	}

	// Get Flags
	if err := binary.Read(reader, binary.BigEndian, &avp.Flags); err != nil {
		panic("Bad AVP Header")
	}

	// Get Len
	if err := binary.Read(reader, binary.BigEndian, &lenHigh); err != nil {
		panic("Bad AVP Header")
	}
	if err := binary.Read(reader, binary.BigEndian, &lenLow); err != nil {
		panic("Bad AVP Header")
	}

	avp.Len = uint32(lenHigh)*256 + uint32(lenLow)
	if avp.Len%4 == 0 {
		avp.PadLen = avp.Len
	} else {
		avp.PadLen = avp.Len + 4 - (avp.Len % 4)
	}

	// Get VendorId
	if avp.isVendorSpecific() {
		if err := binary.Read(reader, binary.BigEndian, &avp.VendorId); err != nil {
			panic("Bad AVP Header")
		}
		dataLen = avp.Len - 12
	} else {
		dataLen = avp.Len - 8
	}

	avp.Data = make([]byte, dataLen)
	// Initialize to avoid nasty errors
	avp.Group = make([]DiameterAVP, 0) // Will Grow as needed

	// Parse according to type
	avpDict := config.DDict.GetFromCode(diamdict.AVPCode{VendorId: avp.VendorId, Code: avp.Code})
	avp.Name = avpDict.Name
	copy(avp.Data, inputBytes[avp.Len-dataLen:avp.Len])
	switch avpDict.DiameterType {
	// None
	case diamdict.None:

		// Int32
	case diamdict.Integer32:
		var value int32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad AVP Header")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(avp.Code)

		// Int64
	case diamdict.Integer64:
		var value int64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad AVP Header")
		}
		avp.LongValue = value
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(avp.Code)

		// UInt32
	case diamdict.Unsigned32:
		var value uint32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad AVP Header")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(avp.Code)

		// UInt64
	case diamdict.Unsigned64:
		var value uint64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad AVP Header")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(avp.Code)

		// Float32
	case diamdict.Float32:
		var value float32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad AVP Header")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(avp.Code)

		// Float64
	case diamdict.Float64:
		var value float64
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad AVP Header")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.StringValue = fmt.Sprint(avp.Code)

	case diamdict.Grouped:
		currentIndex := avp.Len - dataLen
		for currentIndex < avp.PadLen {
			nextAVP := NewDiameterAVP(inputBytes[currentIndex:])
			avp.Group = append(avp.Group, nextAVP)
			currentIndex += nextAVP.PadLen
		}
		avp.LongValue = -1
		avp.FloatValue = -1
		avp.StringValue = avp.buildStringValue()

	case diamdict.Time:
		var value uint32
		if err := binary.Read(reader, binary.BigEndian, &value); err != nil {
			panic("Bad AVP Header")
		}
		avp.LongValue = int64(value)
		avp.FloatValue = float64(value)
		avp.DateValue = zeroTime.Add(time.Second * time.Duration(value))
		// Magical reference date is Mon Jan 2 15:04:05 MST 2006
		avp.StringValue = avp.DateValue.Format("2006-02-01'T'15:04:05")

	}

	return avp
}
