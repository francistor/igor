package handler

import (
	"encoding/json"
	"fmt"

	"github.com/francistor/igor/core"
)

/////////////////////////////////////////////////////////////////////////////
// Radius Packet Attribute Filter
/////////////////////////////////////////////////////////////////////////////

// Entry in AVPFilter file
type RadiusAVPFilter struct {
	Allow  []string    // List of attributes to allow
	Remove []string    // List of attributes to remove. Makes sense either Allow or Remove, but not both
	Force  [][2]string // List of attributes to set a specific value. Contents of the list are 2 element arrays (attribute, value)
}

// Contents of the AVPFilters file. Set of AVPFilters by key
type RadiusAVPFilters map[string]*RadiusAVPFilter

// Creates a copy of the radius packet with the attributes filtered as specified in the filter for the passed key
func (fs RadiusAVPFilters) FilteredPacket(packet *core.RadiusPacket, key string) (*core.RadiusPacket, error) {
	if filter, ok := fs[key]; !ok {
		return &core.RadiusPacket{}, fmt.Errorf("%s filter not found", key)
	} else {
		return filter.FilteredPacket(packet), nil
	}
}

// Copy the radius packet with the attributes modified as defined in the specified filter
func (f *RadiusAVPFilter) FilteredPacket(packet *core.RadiusPacket) *core.RadiusPacket {
	var rp *core.RadiusPacket
	if len(f.Allow) > 0 {
		rp = packet.Copy(f.Allow, nil)
	} else if len(f.Remove) > 0 {
		rp = packet.Copy(nil, f.Remove)
	} else {
		rp = packet.Copy(nil, nil)
	}

	for _, forceSpec := range f.Force {
		if len(forceSpec) == 2 {
			rp.DeleteAllAVP(forceSpec[0])
			rp.Add(forceSpec[0], forceSpec[1])
		}
	}

	return rp
}

// Returns an object representing the configured AVPFilters
func NewAVPFilters(configObjectName string, ci *core.PolicyConfigurationManager) (RadiusAVPFilters, error) {

	filters := make(RadiusAVPFilters)

	// If we pass nil as last parameter, use the default
	var myCi *core.PolicyConfigurationManager
	if ci == nil {
		myCi = core.GetPolicyConfig()
	} else {
		myCi = ci
	}

	// Read the configuration object
	fs, err := myCi.CM.GetBytesConfigObject(configObjectName)
	if err != nil {
		return filters, err
	}

	if err = json.Unmarshal(fs, &filters); err != nil {
		return filters, fmt.Errorf(err.Error())
	}

	return filters, nil
}
