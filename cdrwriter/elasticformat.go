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

const (
	VERSION_ALGORITHM_NONE              = 0
	VERSION_ALGORITHM_SIMPLEVALUE       = 1
	VERSION_ALGORITHM_TIMEANDTYPE       = 2
	VERSION_ALGORITHM_ADJUSTEDSTARTTIME = 3
)

// The attribute map defines the names of the JSON attribues to be written, taking the values from the specified AVP
// Multiple AVP may be combined in a single attribute using the following operators
// : -> Take the first that is present
// + -> Add the values (valid for integers)
// ! -> Subtract the values (valid for integers)
// < -> Add with multiplication by Giga

// This formatter includes some logic for intelligent updating of the CDR, insted of just
// inserting them all.
//
// The _id is generated as a concatenation of the specified fields. Typically it is the
// AccountingSessionId + NAS-IP-Address, or the IP address in case of a sessions table
//
// The version may be calculated using different algorithms, to account for different
// use cases. For instance, in pure CDR, if two records have the same timestamp, the stop
// must have overwrite the interim. or the start But for a sessions table (e.g. where the
// _id is the IP address), the opposite is true.
//
// Algorithm: 1 -> simplevalue. Uses the provided int value without further adjustments
// Algorithm: 2 -> timeandtype. Uses the provided value as timestamp, adding an offset
// for interim and another one for stop (offset is 200.000.000.000 for stop and
// 100.000.000.000 for Interim.)
// Algorithm: 3 -> adjustedstarttime. Uses two values, the timestamp and a session time
// to calculate the start time, but reduced by 1/2 for interim and for 1/3 for stop
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
	// Cooked
	IndexTypeValue int

	// Multiple filed separator
	separator string

	// Parameters for the _id field
	IdFields []string

	// Parameters for the version field
	VersionAlgorithm string
	VersionField1    string
	VersionField2    string
	// Cooked
	VersionAlgorithmValue int
}

// A JSON format is in charge of parsing CDRs and producing a string representation
// to be used in an Elastic database
type ElasticFormat struct {
	ElasticFormatConf
}

// Creates an instance of ElasticWriter with the specified configuration
func NewElasticFormat(conf ElasticFormatConf) *ElasticFormat {

	// Index type
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

	// Version type
	if strings.EqualFold(conf.VersionAlgorithm, "simplevalue") {
		conf.VersionAlgorithmValue = VERSION_ALGORITHM_SIMPLEVALUE
	} else if strings.EqualFold(conf.VersionAlgorithm, "timeandtype") {
		conf.VersionAlgorithmValue = VERSION_ALGORITHM_TIMEANDTYPE
	} else if strings.EqualFold(conf.VersionAlgorithm, "adjustedstarttime") {
		conf.VersionAlgorithmValue = VERSION_ALGORITHM_ADJUSTEDSTARTTIME
	} else if strings.EqualFold(conf.VersionAlgorithm, "none") {
		conf.VersionAlgorithmValue = VERSION_ALGORITHM_NONE
	} else {
		panic("no valid algorithm specified for version")
	}

	// By default the separator for multiple instances of an avp is a ","
	if conf.separator == "" {
		conf.separator = ","
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

	var version int64
	switch ew.VersionAlgorithmValue {
	case VERSION_ALGORITHM_NONE:
		// Do nothing. Version will be a fixed 0
	case VERSION_ALGORITHM_SIMPLEVALUE:
		// Will use 0 as version if attribute not found
		version = rp.GetIntAVP(ew.VersionField1)
	case VERSION_ALGORITHM_TIMEANDTYPE:
		// Calculate offset for version
		var versionOffset int64
		if rp.GetIntAVP("Acct-Status-Type") == 2 {
			versionOffset = 200000000000 // Stop
		} else if rp.GetIntAVP("Acct-Status-Type") == 3 {
			versionOffset = 100000000000 // Interim
		}
		// Make the version
		if ew.VersionField1 == "" {
			version = int64(time.Since(core.ZeroRadiusTime).Seconds()) + versionOffset
		} else {
			if versionAVP, err := rp.GetAVP(ew.VersionField1); err != nil {
				core.GetLogger().Errorf("version attribute %s not found", ew.VersionField1)
				return ""
			} else {
				version = versionAVP.GetInt()
				if version == 0 {
					core.GetLogger().Errorf("version attribute cannot be expressed as integer %v", ew.VersionField1)
					return ""
				}
				version += versionOffset
			}
		}
	case VERSION_ALGORITHM_ADJUSTEDSTARTTIME:
		// Get the event time
		eventTime := rp.GetIntAVP(ew.VersionField1)
		if eventTime == 0 {
			core.GetLogger().Errorf("event time version attribute cannot be expressed as integer %v", ew.VersionField1)
			return ""
		}
		// Get the session time
		sessionTime := rp.GetIntAVP(ew.VersionField2)
		// Calculate the version
		if rp.GetIntAVP("Acct-Status-Type") == 2 { // Stop
			version = eventTime - sessionTime/3
		} else if rp.GetIntAVP("Acct-Status-Type") == 3 { // Interim
			version = eventTime - sessionTime/2
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
				ew.writeStringAVP(&sb, rp.GetAllAVP(attrName), k, &first)
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
			ew.writeStringAVP(&sb, rp.GetAllAVP(v), k, &first)
		}
	}
	sb.WriteString("}")

	sb.WriteString("\n")
	return sb.String()
}

// Helper to write "esAttributeName": <attributeValue> from an AVP
func (ew *ElasticFormat) writeStringAVP(sb *strings.Builder, avps []core.RadiusAVP, attributeName string, first *bool) {

	// Ignore empty set
	if len(avps) == 0 {
		return
	}

	// If not the first attribute, use a comma
	if !*first {
		sb.WriteString(", ")
	} else {
		*first = false
	}

	// Write the name of the attribute
	sb.WriteString("\"")
	sb.WriteString(attributeName)
	sb.WriteString("\": ")

	// Write the value of the attribute, taking into account that it might be multi-valued
	var values []string
	var writeAsString = false
	switch avps[0].DictItem.RadiusType {
	case core.RadiusTypeInteger, core.RadiusTypeInteger64:
		for i, avp := range avps {
			if val, found := avp.DictItem.EnumCodes[int(avp.GetInt())]; found {
				// Write as string
				writeAsString = true
				values = append(values, val)
			} else {
				// Write as integer (yes, using GetString)
				// If an array, surely we must use separators and represent as string
				if i > 0 {
					writeAsString = true
				}
				values = append(values, avp.GetString())
			}
		}
	default:
		for _, avp := range avps {
			// Write as string
			writeAsString = true
			values = append(values, avp.GetString())
		}
	}

	if writeAsString {
		sb.WriteString("\"")
		sb.WriteString(strings.Join(values, ew.separator))
		sb.WriteString("\"")
	} else {
		sb.WriteString(values[0])
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
