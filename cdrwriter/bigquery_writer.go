package cdrwriter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/francistor/igor/core"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

const (
	BIGQUERY_PACKET_BUFFER_SIZE        = 1000
	BIGQUERY_CDR_COUNT_THRESHOLD       = 500
	BIGQUERY_CDR_WRITE_TIME_MILLIS     = 500
	BIGQUERY_BACKUP_CHECK_TIME_SECONDS = 60
)

// Writes CDR to BigQuery
// If unavailability of the database last longer that the configured time
// the CDR are written in a backup file. Backup files are processed periodically
type BigQueryCDRWriter struct {

	// This channel will receive the CDR to write or a tick
	packetChan chan *core.RadiusPacket

	// To signal that we have finished processing CDR
	doneChan chan struct{}

	// Google data
	client *bigquery.Client
	table  *bigquery.Table

	// Unavailability for this time does not lead to throwing away the CDR
	glitchTime time.Duration

	// Name of the file where the CDR will be written in case of database unavailability
	backupFileName string

	// Formatter
	formatter *BigQueryFormat

	// For sending periodics singals to empty batch
	ticker *time.Ticker

	// For testing only
	_forceBigQueryError bool
}

// Builds a writer
// The attributeMap applies only for Radius
// The key is the name of the attribute to be written. The value is the name of the attribute in the CDR
func NewBigQueryCDRWriter(datasetName string, tableName string, formatter *BigQueryFormat, timeoutSeconds int, glitchSeconds int, backupFileName string) *BigQueryCDRWriter {

	ctx := context.Background()

	// Do some checks as soon as possible

	// Check backup file location
	if err := os.MkdirAll(filepath.Dir(backupFileName), 0770); err != nil {
		panic("while initializing, could not create directory " + filepath.Dir(backupFileName) + " :" + err.Error())
	}

	var projectId string
	var client *bigquery.Client
	var err error

	// If passing client credentials, use them to build the bigquery client. The projectId is one of the properties
	// of the JSON credentials file
	credentialsFile := os.Getenv("IGOR_CLOUD_CREDENTIALS")
	if credentialsFile != "" {
		var cred struct {
			Project_id string
		}

		if credBytes, err := os.ReadFile(credentialsFile); err != nil {
			panic("credentials file " + credentialsFile + " read error: " + err.Error())
		} else {
			json.Unmarshal(credBytes, &cred)
		}

		if cred.Project_id == "" {
			panic("credentials file " + credentialsFile + " could not be parsed ")
		}
		projectId = cred.Project_id

		options := option.WithCredentialsFile(credentialsFile)

		// Create the bigquery client. It will not report any errors until really used
		client, err = bigquery.NewClient(ctx, projectId, options)
		if err != nil {
			panic("could not create bigquery client: " + err.Error())
		}
	} else {

		// Use ADC to get the default credentials including the projectId
		cred, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
		if err != nil {
			panic("could not get default credentials. Are we running in a Google Cloud? " + err.Error())
		}

		// Create the bigquery client. It will not report any errors until really used
		client, err = bigquery.NewClient(ctx, cred.ProjectID)
		if err != nil {
			panic("could not create bigquery client: " + err.Error())
		}
	}

	// Try to get table metadata to verify that the provided configuration is correct
	dataset := client.Dataset(datasetName)
	table := dataset.Table(tableName)

	if _, err = table.Metadata(ctx); err != nil {
		panic("bigquery table not available: " + projectId + "." + datasetName + "." + tableName)
	}

	w := BigQueryCDRWriter{
		packetChan:     make(chan *core.RadiusPacket, ELASTIC_PACKET_BUFFER_SIZE),
		doneChan:       make(chan struct{}),
		formatter:      formatter,
		client:         client,
		table:          table,
		glitchTime:     time.Duration(glitchSeconds) * time.Second,
		backupFileName: backupFileName,
	}

	// Rename an old backup file if exists
	os.Rename(w.backupFileName, fmt.Sprintf("%s.%d.w", w.backupFileName, time.Now().UnixMilli()))

	// Start the event loop
	go w.eventLoop()

	// Start the backup processing loop
	go w.processBackupFiles()

	return &w
}

// Call when sure that no more write operations will be invoked
func (w *BigQueryCDRWriter) Close() {

	// Stop sending pings
	w.ticker.Stop()

	// Close the packet channel. The channel will receive a nil and exit
	close(w.packetChan)

	// Consume all the pending CDR in the buffer and wait here
	<-w.doneChan

	// Close the bigquery client
	if w.client != nil {
		w.client.Close()
	}
}

// Event processing loop
func (w *BigQueryCDRWriter) eventLoop() {

	var batch []*WritableCDR
	var cdrCounter = 0
	var lastWritten = time.Now()
	var lastError time.Time
	var hasBackup bool

	// Sends Ticks to signal that a write must be
	// done even if the number of packets has not reached the triggering value.
	w.ticker = time.NewTicker(BIGQUERY_CDR_WRITE_TIME_MILLIS * time.Millisecond)

loop:
	for {
		var m *core.RadiusPacket

		select {
		case <-w.ticker.C:
			// Nothing to do

		case v := <-w.packetChan:
			if v == nil {
				break loop
			} else {
				m = v
				cdrCounter++
				batch = append(batch, w.formatter.GetWritableCDR(m))
			}
		}

		if cdrCounter > BIGQUERY_CDR_COUNT_THRESHOLD || time.Since(lastWritten).Milliseconds() > BIGQUERY_CDR_WRITE_TIME_MILLIS {

			err := w.sendToBigQuery(batch)
			if err != nil {
				// Not written to bq and batch not reset.
				core.GetLogger().Errorf("bq writer error: %s", err)

				// Only if we are outside the glitch interval, backup the CDR
				if time.Since(lastError) > w.glitchTime && len(batch) > 0 {
					core.GetLogger().Errorf("backing up CDR!")

					// Open backup file
					file, err := os.OpenFile(w.backupFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0770)
					if err != nil {
						panic("could not open " + w.backupFileName + " due to " + err.Error())
					}
					hasBackup = true

					// Write to backup
					for _, wcdr := range batch {
						_, err = file.WriteString(wcdr.String())
						if err != nil {
							panic("file write error. Filename: " + w.backupFileName + "error: " + err.Error())
						}

					}
					batch = nil
					file.Close()
				}

				// Set to 0 so that we don't try again immediately later
				cdrCounter = 0
				lastError = time.Now()

			} else {
				// Success. Emtpy the batch
				batch = nil

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
	err := w.sendToBigQuery(batch)
	if err != nil {
		// Not written to elasic and sb not reset.
		core.GetLogger().Errorf("big query writer error: %s. Some CDR may be lost", err)
	}

	close(w.doneChan)
}

// Sends the contents of the current batch to bigquery
// If ok, the builder is reset. Otherwise, the contents are kept
func (w *BigQueryCDRWriter) sendToBigQuery(batch []*WritableCDR) error {
	// For testing only
	if w._forceBigQueryError {
		return errors.New("fake error")
	} else {
		return w.table.Inserter().Put(context.Background(), batch)
	}
}

// Writes the Radius CDR
func (w *BigQueryCDRWriter) WriteRadiusCDR(rp *core.RadiusPacket) {
	if rp == nil {
		return
	}
	w.packetChan <- rp
}

// Writes the Diameter CDR
func (w *BigQueryCDRWriter) WriteDiameterCDR(dm *core.DiameterMessage) {
	panic("Writing diameter to bigquery is not supported yet")
}

// Processes the backup files (the ones with names terminating in ".w")
func (w *BigQueryCDRWriter) processBackupFiles() {

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

		time.Sleep(BIGQUERY_BACKUP_CHECK_TIME_SECONDS * time.Second)
	}
}

// Inserts the contents of the backup file into Bigquery, and deletes
// the file if successful
func (w *BigQueryCDRWriter) processBackupFile(fileName string) error {

	var batch []*WritableCDR

	fullFileName := filepath.Dir(w.backupFileName) + "/" + fileName
	file, err := os.Open(fullFileName)

	core.GetLogger().Debugf("processing backup file %s", fullFileName)

	if err != nil {
		core.GetLogger().Errorf("could not open %s", fullFileName)
		return err
	}
	defer file.Close()

	fileScanner := bufio.NewScanner(file)
	// CDRs are separated by empty lines
	fileScanner.Split(splitAt("\n\n"))

	for fileScanner.Scan() {
		cdr := NewWritableCDRFromStrings(strings.Split(fileScanner.Text(), "\n"))
		batch = append(batch, cdr)
	}

	// Write the batch
	if err := w.sendToBigQuery(batch); err == nil {
		os.Remove(fullFileName)
	} else {
		core.GetLogger().Errorf("error processing backup file %s", fullFileName)
	}

	return err
}

// https://gist.github.com/guleriagishere/8185da56df6d64c2ab652a59808c1011
func splitAt(substring string) func(data []byte, atEOF bool) (advance int, token []byte, err error) {
	searchBytes := []byte(substring)
	searchLength := len(substring)
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		dataLen := len(data)

		// Return Nothing if at the end of file or no data passed.
		if atEOF && dataLen == 0 {
			return 0, nil, nil
		}

		// Find next separator and return token.
		if i := bytes.Index(data, searchBytes); i >= 0 {
			return i + searchLength, data[0:i], nil
		}

		// If we're at EOF, we have a final, non-terminated line. Return it.
		if atEOF {
			return dataLen, data, nil
		}

		// Request more data.
		return 0, nil, nil
	}
}
