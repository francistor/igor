package cdrwriter

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/francistor/igor/core"
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

	// This channel will receive the CDR to write
	packetChan chan interface{}

	// To signal that we have finished processing CDR
	doneChan chan struct{}

	// Location of BigQuery
	projectName     string
	datasetName     string
	tableName       string
	credentialsFile string

	// Unavailability for this time does not lead to throwing away the CDR
	glitchTime time.Duration

	// Name of the file where the CDR will be written in case of database unavailability
	backupFileName string
}

// Builds a writer
// The attributeMap applies only for Radius
// The key is the name of the attribute to be written. The value is the name of the attribute in the CDR
func NewBigQueryCDRWriter(projectName string, datasetName string, tableName string, credentialsFile string,
	timeoutSeconds int, glitchSeconds int, backupFileName string) *BigQueryCDRWriter {

	if err := os.MkdirAll(filepath.Dir(backupFileName), 0770); err != nil {
		panic("while initializing, could not create directory " + filepath.Dir(backupFileName) + " :" + err.Error())
	}

	w := BigQueryCDRWriter{
		packetChan:      make(chan interface{}, ELASTIC_PACKET_BUFFER_SIZE),
		doneChan:        make(chan struct{}),
		projectName:     projectName,
		datasetName:     datasetName,
		tableName:       tableName,
		credentialsFile: credentialsFile,
		glitchTime:      time.Duration(glitchSeconds) * time.Second,
		backupFileName:  backupFileName,
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
func (w *BigQueryCDRWriter) eventLoop() {

	var sb strings.Builder
	var cdrCounter = 0
	var lastWritten = time.Now()
	var lastError time.Time
	var hasBackup bool

	// Sends Ticks through the packet channel, to signal that a write must be
	// done even if the number of packets has not reached the triggering value.
	var ticker = time.NewTicker(BIGQUERY_CDR_WRITE_TIME_MILLIS * time.Millisecond)
	go func() {
		for {
			<-ticker.C
			w.packetChan <- Tick{}
		}
	}()

	for m := range w.packetChan {

		packet, isPacket := m.(*core.RadiusPacket)
		if isPacket {
			fmt.Println(packet)
			cdrCounter++
		}

		if cdrCounter > BIGQUERY_CDR_COUNT_THRESHOLD || time.Since(lastWritten).Milliseconds() > BIGQUERY_CDR_WRITE_TIME_MILLIS {
			if sb.Len() == 0 {
				continue
			}
			err := w.sendToBigQuery(&sb)
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
	err := w.sendToBigQuery(&sb)
	if err != nil {
		// Not written to elasic and sb not reset.
		core.GetLogger().Errorf("elastic writer error: %s", err)
	}

	ticker.Stop()
	close(w.doneChan)
}

// Sends the contents of the current stringbuilder to Elastic
// If ok, the builder is reset. Otherwise, the contents are kept
func (w *BigQueryCDRWriter) sendToBigQuery(sb *strings.Builder) error {

	return nil
}

// Writes the Radius CDR
func (w *BigQueryCDRWriter) WriteRadiusCDR(rp *core.RadiusPacket) {
	w.packetChan <- rp
}

// Writes the Diameter CDR
func (w *BigQueryCDRWriter) WriteDiameterCDR(dm *core.DiameterMessage) {
	panic("Writing diameter to bigquery is not supported yet")
}

// Call when sure that no more write operations will be invoked
func (w *BigQueryCDRWriter) Close() {
	close(w.packetChan)

	// Consume all the pending CDR in the buffer
	<-w.doneChan
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

// Processes a single backup file. Deletes it if successful
func (w *BigQueryCDRWriter) processBackupFile(fileName string) error {

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
			err = w.sendToBigQuery(&sb)
			if err != nil {
				core.GetLogger().Errorf("error writing to elastic: %s", err)
				return err
			}
			i = 0
		}
	}

	// Write the remaining lines
	if sb.Len() > 0 {
		err = w.sendToBigQuery(&sb)
	}
	if err != nil {
		core.GetLogger().Errorf("error writing to elastic: %s", err)
		return err
	}

	os.Remove(fullFileName)

	return nil
}
