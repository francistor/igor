package cdrwriter

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/francistor/igor/core"
)

const (
	ELASTIC_PACKET_BUFFER_SIZE        = 1000
	ELASTIC_CDR_COUNT_THRESHOLD       = 500
	ELASTIC_CDR_WRITE_TIME_MILLIS     = 500
	ELASTIC_BACKUP_CHECK_TIME_SECONDS = 60
)

// Writes CDR to Elastic using bulk injection
// If unavailability of the database last longer that the configured time
// the CDR are written in a backup file. Backup files are processed periodically
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
	formatter *ElasticFormat

	// Name of the file where the CDR will be written in case of database unavailability
	backupFileName string
}

// Builds a writer
// The attributeMap applies only for Radius
// The key is the name of the attribute to be written. The value is the name of the attribute in the CDR
func NewElasticCDRWriter(url string, username string, password string, formatter *ElasticFormat,
	timeoutSeconds int, glitchSeconds int, backupFileName string) *ElasticCDRWriter {

	if err := os.MkdirAll(filepath.Dir(backupFileName), 0770); err != nil {
		panic("while initializing, could not create directory " + filepath.Dir(backupFileName) + " :" + err.Error())
	}

	w := ElasticCDRWriter{
		packetChan:     make(chan interface{}, ELASTIC_PACKET_BUFFER_SIZE),
		doneChan:       make(chan struct{}),
		url:            url,
		username:       username,
		password:       password,
		glitchTime:     time.Duration(glitchSeconds) * time.Second,
		formatter:      formatter,
		backupFileName: backupFileName,
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

	// Rename an old backup file if exists
	os.Rename(w.backupFileName, fmt.Sprintf("%s.%d.w", w.backupFileName, time.Now().UnixMilli()))

	// Start the event loop
	go w.eventLoop()

	// Start the backup processing loop
	go w.processBackupFiles()

	return &w
}

// Event processing loop
func (w *ElasticCDRWriter) eventLoop() {

	var sb strings.Builder
	var cdrCounter = 0
	var lastWritten = time.Now()
	var lastError time.Time
	var hasBackup bool

	// Sends Ticks through the packet channel, to signal that a write must be
	// done even if the number of packets has not reached the triggering value.
	var ticker = time.NewTicker(ELASTIC_CDR_WRITE_TIME_MILLIS * time.Millisecond)
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

		if cdrCounter > ELASTIC_CDR_COUNT_THRESHOLD || time.Since(lastWritten).Milliseconds() > ELASTIC_CDR_WRITE_TIME_MILLIS {
			if sb.Len() == 0 {
				continue
			}
			err := w.sendToElastic(&sb)
			if err != nil {
				// Not written to elasic and sb not reset.
				core.GetLogger().Errorf("elastic writer error: %s", err)

				// Only if we are outside the glitch interval, throw away the CDR
				if time.Since(lastError) > w.glitchTime {
					core.GetLogger().Errorf("backing up CDR!")

					// Open backup file
					file, err := os.OpenFile(w.backupFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0770)
					if err != nil {
						panic("could not open " + w.backupFileName + " due to " + err.Error())
					}
					hasBackup = true

					// Write to backup
					_, err = file.WriteString(sb.String())
					if err != nil {
						panic("file write error. Filename: " + w.backupFileName + "error: " + err.Error())
					} else {
						file.Close()
					}

					sb.Reset()
				}
				// Set to 0 so that we don't try again immediately later
				cdrCounter = 0
				lastError = time.Now()

			} else {
				// Move backup file and start processing, if just recovered from an error
				if hasBackup {
					os.Rename(w.backupFileName, fmt.Sprintf("%s.%d.w", w.backupFileName, time.Now().UnixMilli()))
				}
				hasBackup = false

				// For clarity, repeated here
				cdrCounter = 0
				lastError = time.Time{}
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

	httpReq, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader([]byte(sb.String())))
	if err != nil {
		return fmt.Errorf("could not generate request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "Application/json")
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

// Processes the backup files (the ones with names terminating in ".w")
func (w *ElasticCDRWriter) processBackupFiles() {

	// Will run forever
	for {
		// List backup files
		files, err := os.ReadDir(filepath.Dir(w.backupFileName))
		if err != nil {
			core.GetLogger().Errorf("could not list files in %s", filepath.Dir(w.backupFileName))
		}

		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".w") {
				w.processBackupFile(file.Name())
			}
		}

		time.Sleep(ELASTIC_BACKUP_CHECK_TIME_SECONDS * time.Second)
	}
}

// Processes a single backup file. Deletes it if successful
func (w *ElasticCDRWriter) processBackupFile(fileName string) error {

	fullFileName := filepath.Dir(w.backupFileName) + "/" + fileName
	file, err := os.Open(fullFileName)

	core.GetLogger().Debugf("processing backup file %s", fullFileName)

	if err != nil {
		core.GetLogger().Errorf("could not open %s", fullFileName)
		return err
	}
	defer file.Close()

	fileScanner := bufio.NewScanner(file)
	fileScanner.Split(bufio.ScanLines)

	var i int
	var sb strings.Builder
	for fileScanner.Scan() {

		sb.WriteString(fileScanner.Text())
		sb.WriteString("\n")
		i++
		if i == 2*ELASTIC_PACKET_BUFFER_SIZE {
			err = w.sendToElastic(&sb)
			if err != nil {
				core.GetLogger().Errorf("error writing to elastic: %s", err)
				return err
			}
			i = 0
		}
	}

	// Write the remaining lines
	if sb.Len() > 0 {
		err = w.sendToElastic(&sb)
	}
	if err != nil {
		core.GetLogger().Errorf("error writing to elastic: %s", err)
		return err
	}

	os.Remove(fullFileName)

	return nil
}
