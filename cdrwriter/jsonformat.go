package cdrwriter

import (
	"encoding/json"
	"igor/diamcodec"
	"igor/radiuscodec"

	"golang.org/x/exp/slices"
)

type JSONWriter struct {
	positiveFilter []string
	negativeFilter []string
}

// Creates a new instance of a Livinstone Writer
func NewJSONWriter(positiveFilter []string, negativeFilter []string) *JSONWriter {
	lw := JSONWriter{
		positiveFilter: positiveFilter,
		negativeFilter: negativeFilter,
	}

	return &lw
}

///---> What to write to ELASTIC?

// Not implemented
func (w *JSONWriter) GetDiameterCDRString(dm *diamcodec.DiameterMessage) string {
	toSerialize := make([]*diamcodec.DiameterAVP, 0)

	// Write AVPs
	for i := range dm.AVPs {

		// Apply filters
		if w.positiveFilter != nil && !slices.Contains(w.positiveFilter, dm.AVPs[i].Name) {
			continue
		} else if w.negativeFilter != nil && slices.Contains(w.negativeFilter, dm.AVPs[i].Name) {
			continue
		}

		toSerialize = append(toSerialize, &dm.AVPs[i])
	}

	jsonAttributes, _ := json.Marshal(toSerialize)
	return string(jsonAttributes)
}

// Write CDR as list with separators
// Ints are not tried to write as strings, even if an enum is defined
func (w *JSONWriter) GetRadiusCDRString(rp *radiuscodec.RadiusPacket) string {

	toSerialize := make([]*radiuscodec.RadiusAVP, 0)

	// Write AVPs
	for i := range rp.AVPs {

		// Apply filters
		if w.positiveFilter != nil && !slices.Contains(w.positiveFilter, rp.AVPs[i].Name) {
			continue
		} else if w.negativeFilter != nil && slices.Contains(w.negativeFilter, rp.AVPs[i].Name) {
			continue
		}

		toSerialize = append(toSerialize, &rp.AVPs[i])
	}

	jsonAttributes, _ := json.Marshal(toSerialize)
	return string(jsonAttributes)
}