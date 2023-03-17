package cdrwriter

import (
	"encoding/json"

	"github.com/francistor/igor/core"

	"golang.org/x/exp/slices"
)

type JSONFormat struct {
	positiveFilter []string
	negativeFilter []string
}

// Creates a new instance of a Livinstone Writer
func NewJSONFormat(positiveFilter []string, negativeFilter []string) *JSONFormat {
	lw := JSONFormat{
		positiveFilter: positiveFilter,
		negativeFilter: negativeFilter,
	}

	return &lw
}

// There is no specific field for the Timestamp. If needed, the attribute must be already present
// in the packet/message. A Timestamp attribute may be added in the handler if not sent by the BRAS

// Writes the Diameter CDR in JSON format
func (w *JSONFormat) GetDiameterCDRString(dm *core.DiameterMessage) string {
	toSerialize := make([]*core.DiameterAVP, 0)

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

// Writes the CDR in JSON format
func (w *JSONFormat) GetRadiusCDRString(rp *core.RadiusPacket) string {

	toSerialize := make([]*core.RadiusAVP, 0)

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
