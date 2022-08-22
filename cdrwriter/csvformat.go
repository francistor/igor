package cdrwriter

import (
	"fmt"
	"igor/radiuscodec"
	"igor/radiusdict"
	"strings"
)

type CSVWriter struct {
	fields              []string
	fieldSeparator      string
	attributeSeparator  string
	attributeDateFormat string
	quoteStrings        bool
}

// Creates a new instance of a Livinstone Writer
func NewCSVWriter(fields []string, fieldSeparator string, attributeSeparator string, attributeDateFormat string, quoteStrings bool) *CSVWriter {
	lw := CSVWriter{
		fields:              fields,
		fieldSeparator:      fieldSeparator,
		attributeSeparator:  attributeSeparator,
		attributeDateFormat: attributeDateFormat,
		quoteStrings:        quoteStrings,
	}

	return &lw
}

func (w *CSVWriter) WriteCDRString(rp *radiuscodec.RadiusPacket) string {
	var builder strings.Builder

	for i, field := range w.fields {

		avps := rp.GetAllAVP(field)
		if len(avps) > 0 {
			switch avps[0].DictItem.RadiusType {
			case radiusdict.None, radiusdict.Octets, radiusdict.String, radiusdict.InterfaceId, radiusdict.Address, radiusdict.IPv6Address, radiusdict.IPv6Prefix, radiusdict.Time:
				if w.quoteStrings {
					builder.WriteString("\"")
				}
				for j := range avps {
					builder.WriteString(avps[j].GetTaggedString())
					if j < len(avps)-1 {
						builder.WriteString(w.attributeSeparator)
					}
				}
				if w.quoteStrings {
					builder.WriteString("\"")
				}

			case radiusdict.Integer:
				for j := range avps {
					builder.WriteString(fmt.Sprintf("%d", avps[j].GetInt()))
					if j < len(avps)-1 {
						builder.WriteString(w.attributeSeparator)
					}
				}
			}
		}

		if i < len(w.fields)-1 {
			builder.WriteString(w.fieldSeparator)
		}
	}

	builder.WriteString("\n")

	return builder.String()
}
