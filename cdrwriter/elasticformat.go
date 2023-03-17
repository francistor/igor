package cdrwriter

import (
	"fmt"
	"strings"
	"time"

	"github.com/francistor/igor/core"
)

const (
	ES_INDEX_TYPE_FIXED       = 0
	ES_INDEX_TYPE_FIELD       = 1
	ES_INDEX_TYPE_CURRENTDATE = 2
)

// The attribute map defines the names of the JSON attribues to be written, taking the values from the specified AVP
// Multiple AVP may be combined in a single attribute using the following operators
// : -> Take the first that is present
// + -> Add the values (valid for integers)
// ! -> Subtract the values (valid for integers)
// < -> Add with multiplication by Giga

// The _id is generated as a concatenation of the specified fields. Typically it is the
// AccountingSessionId + NAS-IP-Address, or the IP address in case of a sessions table
//
// The version is calculated as the value of the specified attribute, normaly a Time
// attribute, or as the current Time. In both cases are converted to seconds since the
// epoch 1.673.107.886, and an offset is added for taking into account that a stop takes
// higher precedence than an interim, that takes higher predecence than a start
// That offset is 200.000.000.000 for stop and 100.000.000.000 for Interim.
// It may also be calculated as the current time, and the same treatment of accounting-
// status-type is performed
//
// The index may be as specified in the indexType element
// "fixed" -> ES_INDEX_TYPE_FIXED
// "field" -> ES_INDEX_TYPE_FIELD and the name of the attribute to be used is specified in the attributeForIndex
// "currentDate" -> ES_INDEX_TYPE_CURRENTDATE
type ElasticFormatConf struct {
	AttributeMap map[string]string

	// Parameters for the insertion index
	IndexType         string
	IndexName         string
	AttributeForIndex string
	IndexDateFormat   string

	// Parameters for the _id field
	IdFields []string

	// Parameters for the version field
	VersionField string

	// Cooked
	IndexTypeValue int
}

type ElasticFormat struct {
	ElasticFormatConf
}

// Creates an instance of ElasticWriter with the specified configuration
func NewElasticFormat(conf ElasticFormatConf) *ElasticFormat {

	if strings.EqualFold(conf.IndexType, "fixed") {
		conf.IndexTypeValue = ES_INDEX_TYPE_FIXED
	} else if strings.EqualFold(conf.IndexType, "field") {
		conf.IndexTypeValue = ES_INDEX_TYPE_FIELD
		if conf.AttributeForIndex == "" {
			panic("using indexType 'field' but missing AttributeForIndex")
		}
	} else if strings.EqualFold(conf.IndexType, "currentDate") {
		conf.IndexTypeValue = ES_INDEX_TYPE_CURRENTDATE
	} else {
		panic("bad specification for index type: " + conf.IndexType)
	}

	if len(conf.IdFields) == 0 {
		panic("IdFields is required")
	}

	return &ElasticFormat{
		conf,
	}
}

// Writes the Diameter CDR in JSON format
func (ew *ElasticFormat) GetDiameterCDRString(dm *core.DiameterMessage) string {
	panic("writing diameter not supported")
}

// Writes the CDR in JSON format applying the format
// In case of error, returns zero string
func (ew *ElasticFormat) GetRadiusCDRString(rp *core.RadiusPacket) string {

	// Calculate offset for version
	var versionOffset int64
	if rp.GetIntAVP("Acct-Status-Type") == 2 {
		versionOffset = 200000000000 // Stop
	} else if rp.GetIntAVP("Acct-Status-Type") == 3 {
		versionOffset = 100000000000 // Interim
	}

	// Make the index
	var indexName string
	switch ew.IndexTypeValue {
	case ES_INDEX_TYPE_FIXED:
		indexName = ew.IndexName
	case ES_INDEX_TYPE_FIELD:
		if indexAVP, err := rp.GetAVP(ew.AttributeForIndex); err != nil {
			core.GetLogger().Errorf("index attribute %s not found", ew.AttributeForIndex)
			return ""
		} else {
			if indexAVP.DictItem.RadiusType == core.RadiusTypeTime {
				indexName = ew.IndexName + indexAVP.GetDate().Format(ew.IndexDateFormat)
			} else {
				indexName = ew.IndexName + indexAVP.GetString()
			}
		}
	case ES_INDEX_TYPE_CURRENTDATE:
		indexName = ew.IndexName + time.Now().Format(ew.IndexDateFormat)
	}

	// Make the _id
	var _id string
	for _, s := range ew.IdFields {
		_id += rp.GetStringAVP(s) + "|"
	}

	// Make the version
	var version int64
	if ew.VersionField == "" {
		version = int64(time.Since(core.ZeroRadiusTime).Seconds()) + versionOffset
	} else {
		if versionAVP, err := rp.GetAVP(ew.VersionField); err != nil {
			core.GetLogger().Errorf("version attribute %s not found", ew.VersionField)
			return ""
		} else {
			version = versionAVP.GetInt()
			if version == 0 {
				core.GetLogger().Errorf("version attribute cannot be exrpessed as integer", ew.VersionField)
				return ""
			}
			version += versionOffset
		}
	}

	/*
		Example
		{"index": {"_id": "session-2", "_index": "cdr", "version": 1, "version_type": "extrnal"}}
		{"Event-Date": "2018-10-30 01:00:00", "Acct-Session-Id":"session-2", "Acct-Session-Time": 3600, "NAS-IP-Address": "1.1.1.1", "NAS-Port": 31416, "User-Name": "frg1@tid.es", "Class": "#S:Asd1M","Acct-Input-Octets": 0, "Acct-Output-Octets": 0, "Acct-Input-Gigawords": 0, "Acct-Output-Gigawords": 0, "Framed-IP-Address": "192.168.1.34", "Acct-Terminate-Cause": "User-Request", "Acct-Status-Type": "Start", "NAS-Port-Id": "1.1.1.1:45-6", "Acct-Multi-Session-Id": "1.1.1.1:45-6", "PSA-SERVICE-NAME": "sd1M"}
	*/

	var sb strings.Builder

	// Write header
	sb.WriteString("{\"index\": {\"_id\": \"")
	sb.WriteString(_id)
	sb.WriteString("\", \"_index\": \"")
	sb.WriteString(indexName)
	sb.WriteString("\", ")
	sb.WriteString("\"version\": ")
	sb.WriteString(fmt.Sprintf("%d", version))
	sb.WriteString(", \"version_type\": \"external\"")
	sb.WriteString("}}")
	sb.WriteString("\n")

	// Write content
	sb.WriteString("{")
	var first = true
	for k, v := range ew.AttributeMap {
		if strings.Contains(v, ":") {
			// Write the first not null
			for _, attrName := range strings.Split(v, ":") {
				if avp, err := rp.GetAVP(attrName); err != nil {
					ew.writeStringAVP(&sb, avp, k, &first)
					break
				}
			}
		} else if strings.Contains(v, "+") {
			// Add the values
			var val int64 = 0
			for _, attrName := range strings.Split(v, "+") {
				val += rp.GetIntAVP(attrName)
			}
			ew.writeIntAVP(&sb, val, k, &first)
		} else if strings.Contains(v, "!") {
			// Substract the values
			var val int64 = 0
			for i, attrName := range strings.Split(v, "!") {
				if i == 0 {
					val = rp.GetIntAVP(attrName)
				} else {
					val -= rp.GetIntAVP(attrName)
				}
			}
			ew.writeIntAVP(&sb, val, k, &first)
		} else if strings.Contains(v, "<") {
			// Add the second multiplied by 2^32 (for Gigawords)
			var val int64 = 0
			for i, attrName := range strings.Split(v, "<") {
				if i == 0 {
					val = rp.GetIntAVP(attrName)
				} else {
					val += rp.GetIntAVP(attrName) * int64(4294967296)
				}
			}
			ew.writeIntAVP(&sb, val, k, &first)
		} else {
			if avp, err := rp.GetAVP(v); err == nil {
				ew.writeStringAVP(&sb, avp, k, &first)
			}
		}
	}
	sb.WriteString("}")

	sb.WriteString("\n")
	return sb.String()
}

// Helper to write "esAttributeName": <attributeValue> from an AVP
func (ew *ElasticFormat) writeStringAVP(sb *strings.Builder, avp core.RadiusAVP, attributeName string, first *bool) {

	if !*first {
		sb.WriteString(", ")
	} else {
		*first = false
	}

	sb.WriteString("\"")
	sb.WriteString(attributeName)
	sb.WriteString("\": ")

	switch avp.DictItem.RadiusType {
	case core.RadiusTypeInteger, core.RadiusTypeInteger64:
		if val, found := avp.DictItem.EnumCodes[int(avp.GetInt())]; found {
			// Write as string
			sb.WriteString("\"")
			sb.WriteString(val)
			sb.WriteString("\"")
		} else {
			// Write as integer (yes, using GetString)
			sb.WriteString(avp.GetString())
		}
	default:
		// Write as string
		sb.WriteString("\"")
		sb.WriteString(avp.GetString())
		sb.WriteString("\"")
	}
}

// Helper to write "esAttributeName": integerAttributeValue
func (ew *ElasticFormat) writeIntAVP(sb *strings.Builder, value int64, attributeName string, first *bool) {

	if !*first {
		sb.WriteString(", ")
	} else {
		*first = false
	}

	sb.WriteString("\"")
	sb.WriteString(attributeName)
	sb.WriteString("\": ")

	sb.WriteString(fmt.Sprintf("%d", value))
}
