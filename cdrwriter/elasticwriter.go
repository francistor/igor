package cdrwriter

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/francistor/igor/core"
)

const (
	ELASTIC_PACKET_BUFFER_SIZE = 1000
	CDR_COUNT_THRESHOLD        = 1000
	CDR_WRITE_TIME_MILLIS      = 250
)

type Tick struct{}

// Writes CDR to Elastic using bulk injectionimport
type ElasticCDRWriter struct {

	// This channel will receive the CDR to write
	packetChan chan interface{}

	// To signal that we have finished processing CDR
	doneChan chan struct{}

	// Location of elastic
	url string

	// Http client
	httpClient http.Client

	// Parameters for authentication
	username string
	password string

	// Unavailability for this time does not lead to throwing away the CDR
	glitchTime time.Duration

	// Formatter
	formatter *ElasticWriter
}

// Builds a writer
// The attributeMap applies only for Radius
// The key is the name of the attribute to be writen. The value is the name of the attribute in the CDR
func NewElasticCDRWriter(url string, username string, password string, formatter *ElasticWriter, timeoutSeconds int, glitchSeconds int) *ElasticCDRWriter {

	w := ElasticCDRWriter{
		packetChan: make(chan interface{}, ELASTIC_PACKET_BUFFER_SIZE),
		doneChan:   make(chan struct{}),
		url:        url,
		username:   username,
		password:   password,
		glitchTime: time.Duration(glitchSeconds) * time.Second,
		formatter:  formatter,
	}

	// Create the http client
	w.httpClient = http.Client{
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	}

	if strings.HasPrefix(url, "https:") {
		w.httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ignore expired SSL certificates
		}
	}

	go w.eventLoop()

	return &w
}

func (w *ElasticCDRWriter) eventLoop() {

	var sb strings.Builder
	var cdrCounter = 0
	var lastWritten = time.Now()
	var lastError time.Time

	// Sends Ticks through the packet channel
	var ticker = time.NewTicker(CDR_WRITE_TIME_MILLIS * time.Millisecond)
	go func() {
		for {
			<-ticker.C
			w.packetChan <- Tick{}
		}
	}()

	for m := range w.packetChan {

		packet, isPacket := m.(*core.RadiusPacket)
		if isPacket {
			sb.WriteString(w.formatter.GetRadiusCDRString(packet))
			cdrCounter++
		}

		if cdrCounter > CDR_COUNT_THRESHOLD || time.Since(lastWritten).Milliseconds() > CDR_WRITE_TIME_MILLIS {
			err := w.sendToElastic(&sb)
			if err != nil {
				// Not written to elasic and sb not reset.
				core.GetLogger().Errorf("elastic writer error: %s", err)

				// Only if we are outside the glitch interval, throw away the CDR
				if time.Since(lastError) > w.glitchTime {
					sb.Reset()
				}
				// Set to 0 so that we don't try again immediately later
				cdrCounter = 0
				lastError = time.Now()

			} else {
				// For clarity, repeated here
				cdrCounter = 0
			}
			lastWritten = time.Now()
		}
	}

	// Write the remaining CDR
	err := w.sendToElastic(&sb)
	if err != nil {
		// Not written to elasic and sb not reset.
		core.GetLogger().Errorf("elastic writer error: %s", err)
	}

	ticker.Stop()
	close(w.doneChan)
}

// Sends the contents of the current stringbuilder to Elastic
// If ok, the builder is reset. Otherwise, the contents are kept
func (w *ElasticCDRWriter) sendToElastic(sb *strings.Builder) error {

	if sb.Len() == 0 {
		// Nothing to do
		return nil
	}

	httpReq, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader([]byte(sb.String())))
	if err != nil {
		return fmt.Errorf("could not generate request: %w", err)
	}
	if w.username != "" {
		httpReq.SetBasicAuth(w.username, w.password)
	}
	httpResp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("could not send data to elastic: %w", err)
	}
	if httpResp.StatusCode > 400 {
		httpResp.Body.Close()
		return fmt.Errorf("insertion to elastic returned %d: ", httpResp.StatusCode)
	}
	// Read response as JSON
	jsonResponse, err := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	if err != nil {
		return fmt.Errorf("could not read elastic response: %w", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal(jsonResponse, &response)
	if err != nil {
		return fmt.Errorf("could not unmarshal elastic response: %w", err)
	}

	errors, found := response["errors"]
	if !found {
		return fmt.Errorf("could not interpret elastic response: %w", err)
	}
	errorsBool, ok := errors.(bool)
	if !ok {
		return fmt.Errorf("could not interpret elastic response. Errors was not boolean: %w", err)
	}

	// If here, insertion was done
	(*sb).Reset()

	if errorsBool {
		core.GetLogger().Warn("insertion returned errors")
	}

	return nil
}

// Writes the Radius CDR
func (w *ElasticCDRWriter) WriteRadiusCDR(rp *core.RadiusPacket) {
	w.packetChan <- rp
}

// Writes the Diameter CDR
func (w *ElasticCDRWriter) WriteDiameterCDR(dm *core.DiameterMessage) {
	panic("Writing diameter to elastic is not supported yet")
}

// Call when sure that no more write operations will be invoked
func (w *ElasticCDRWriter) Close() {
	close(w.packetChan)

	// Consume all the pending CDR in the buffer
	<-w.doneChan
}
