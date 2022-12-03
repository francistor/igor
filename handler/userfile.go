package handler

import (
	"encoding/json"
	"strings"

	"github.com/francistor/igor/config"
	"github.com/francistor/igor/radiuscodec"
)

/////////////////////////////////////////////////////////////////////////////
// User File read helpers
/////////////////////////////////////////////////////////////////////////////

type Properties map[string]string

// Merges Properties. New with higher priority
func (q Properties) OverrideWith(p Properties) Properties {
	r := p

	// Merge
	for k, v := range q {
		if _, found := p[k]; !found {
			p[k] = v
		}
	}

	return r
}

// Stringer interface
func (p Properties) String() string {
	var sb strings.Builder
	for k, v := range p {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
		sb.WriteString("\n")
	}

	return sb.String()
}

type AVPItems []radiuscodec.RadiusAVP

// Merges Radius Items. The new with higher priority
func (lp AVPItems) OverrideWith(hp AVPItems) AVPItems {
	r := hp

	// Merge Items
	var found bool
	for i := range lp {
		found = false
		for j := range hp {
			if hp[j].Name == lp[i].Name {
				found = true
				break
			}
		}
		if !found {
			r = append(r, lp[i])
		}
	}

	return r
}

// Adds the two RadiusItems
func (a AVPItems) Add(b AVPItems) AVPItems {
	return append(a, b...)
}

// Represents an entry in a UserFile
type RadiusUserFileEntry struct {
	Key                      string
	CheckItems               Properties
	ConfigItems              Properties
	ReplyItems               AVPItems
	NonOverridableReplyItems AVPItems
	OOBReplyItems            AVPItems
}

type RadiusUserFile map[string]RadiusUserFileEntry

// Parses a Radius Userfile
// Entries are of the form
// key:
//
//		checkItems: {attr: value, attr:value}
//	 configItems: {attr: value, attr:value}
//		replyItems: [<AVP>],
//		nonOverridableReplyItems: [<AVP>] -- typically for Cisco-AVPair
//		oobReplyItems: [<AVP>]			   -- Service definition queries from BNG
func NewRadiusUserFile(configObjectName string, ci *config.PolicyConfigurationManager) (RadiusUserFile, error) {
	// If we pass nil as last parameter, use the default
	var myCi *config.PolicyConfigurationManager
	if ci == nil {
		myCi = config.GetPolicyConfig()
	} else {
		myCi = ci
	}

	jBytes, err := myCi.CM.GetBytesConfigObject(configObjectName)
	if err != nil {
		return RadiusUserFile{}, err
	}

	ruf := RadiusUserFile{}
	err = json.Unmarshal(jBytes, &ruf)

	return ruf, err
}
