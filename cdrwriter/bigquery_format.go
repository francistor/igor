package cdrwriter

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/francistor/igor/core"
)

// The attribute map defines the names of the attribues to be written, taking the values from the specified AVP
// Multiple AVP may be combined in a single attribute using the following operators
// : -> Take the first that is present
// + -> Add the values (valid for integers)
// ! -> Subtract the values (valid for integers)
// < -> Add with multiplication by Giga
//
// The types of the attributes are inferred from the types of the AVP
// Int and Int64 are mapped to BigQueryInteger.
// Time is mapped to BigQueryTimestamp.
// All others are mapped to BigQueryString.
//
// Note: Only the first attribute of each type is used.
//
// Following the philosophy of Bigquery, updates are never done. All rows are inserted
// in big query, each with its own timestamp. How to extract a single session information
// is left as a problem for the reader.
//
// The dataset and schema need to be created beforehand. Notice that they cannot be created
// on the fly, because it takes some time for them to become available after that operation.
//
// The following needs to be done in the Google Account
// * Define the project to use
// * Have a Service Account with permissions for BigQuery (Data Editor?)
// * Get a JSON key for that Service Account. This is an input for the writer
//

// Definition of the data to be inserted, implementing the ValueSaver interface
type WritableCDR struct {
	fields map[string]bigquery.Value
}

// For the ValueSaver interface
// Returns a map of the field names to an interface, plus the id of the insertion (use NoDeupeId here)
// Format is attrname\n type\n value\n for each attribute, ending with a newline
func (cdr *WritableCDR) Save() (map[string]bigquery.Value, string, error) {
	return cdr.fields, bigquery.NoDedupeID, nil
}

// Serialize to a string to send to a file. For the processing of backups
func (cdr *WritableCDR) String() string {
	var sb strings.Builder

	// Iterate over the CDR fields
	for k, v := range cdr.fields {
		sb.WriteString(k)
		sb.WriteString("\n")
		// Try as time
		if timeVal, ok := v.(time.Time); ok {
			sb.WriteString("T")
			sb.WriteString("\n")
			sb.WriteString(strconv.FormatInt(timeVal.Unix(), 10))
		} else if intVal, ok := v.(int64); ok {
			sb.WriteString("I")
			sb.WriteString("\n")
			sb.WriteString(strconv.FormatInt(intVal, 10))
		} else if octetsVal, ok := v.([]byte); ok {
			sb.WriteString("O")
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf("%x", octetsVal))
		} else if stringVal, ok := v.(string); ok {
			sb.WriteString("S")
			sb.WriteString("\n")
			sb.WriteString(stringVal)
		}
		sb.WriteString("\n")
	}
	// End of the CDR
	sb.WriteString("\n")
	return sb.String()
}

// Need to pass a multiple of 3 lines
func NewWritableCDRFromStrings(lines []string) *WritableCDR {

	cdr := WritableCDR{
		fields: make(map[string]bigquery.Value),
	}

	var i = 0
	for {
		// Sanity Check to avoid index out of bounds
		if len(lines) < i+3 {
			panic("bad framing. Less than 3 lines to read in after line " + strconv.FormatInt(int64(i), 10))
		}

		// Read line with attribute name (i+0)
		attrName := lines[i]

		// Read line with type (i+1)
		i++
		switch lines[i] {

		// Read value (i+2)
		case "T":
			i++
			v, _ := strconv.Atoi(lines[i])
			cdr.fields[attrName] = time.Unix(int64(v), 0)
		case "I":
			i++
			v, _ := strconv.Atoi(lines[i])
			cdr.fields[attrName] = v
		case "O":
			i++
			v, _ := hex.DecodeString(lines[i])
			cdr.fields[attrName] = v
		case "S":
			i++
			cdr.fields[attrName] = lines[i]
		default:
			panic("Got " + lines[i] + " as type -> bad framing")
		}

		i++
		if i >= len(lines) {
			break
		}
	}

	return &cdr
}

type BigQueryFormatConf struct {
	AttributeMap map[string]string
}

// BigQueryFormat generates the WritableCDR from a radius packet
type BigQueryFormat struct {
	BigQueryFormatConf
}

// Creates an instance of BigQueryWriter with the specified configuration
func NewBigQueryFormat(conf BigQueryFormatConf) *BigQueryFormat {
	return &BigQueryFormat{
		conf,
	}
}

// From the Radius packet, generates an object that is insertable in BigQuery
func (bq *BigQueryFormat) GetWritableCDR(rp *core.RadiusPacket) *WritableCDR {

	cdr := WritableCDR{
		fields: make(map[string]bigquery.Value),
	}

	// Generate CDR
	for k, v := range bq.AttributeMap {
		if strings.Contains(v, ":") {
			// Write the first not empty
			for _, attrName := range strings.Split(v, ":") {
				if avp, err := rp.GetAVP(attrName); err != nil {
					break
				} else {
					cdr.fields[k] = avp.Value
				}
			}
		} else if strings.Contains(v, "+") {
			// Add the values
			var val int64 = 0
			for _, attrName := range strings.Split(v, "+") {
				val += rp.GetIntAVP(attrName)
			}
			cdr.fields[k] = val
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
			cdr.fields[k] = val
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
			cdr.fields[k] = val
		} else {
			if avp, err := rp.GetAVP(v); err == nil {
				cdr.fields[k] = avp.Value
			}
		}
	}

	return &cdr
}
