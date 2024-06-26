package cdrwriter

import (
	"os"
	"time"

	"github.com/francistor/igor/core"
)

const (
	FILE_PACKET_BUFFER_SIZE = 1000
)

// Writes files rotating by date
// The date in the name of the file follows the creation date. Dates of the CDR stored
// may span a longer time than implied in the file name.
type FileCDRWriter struct {

	// This channel will receive the CDR to write
	// TODO: re-write as either *RadiusPacket or *DiameterMessage
	packetChan chan interface{}

	// To signal that we have finished processing CDR
	doneChan chan struct{}

	// Externally created, holding the method to format the CDR
	formatter CDRFormatter

	// Timestamp in unix seconds for the date of creation of the file currently being used
	currentFileTimestamp int64

	// For sanity check
	currentFileName string

	// The file in use now
	file *os.File

	// Writer configuration
	rotateSeconds  int64
	filePath       string
	fileNameFormat string
}

// Builds a writer
// The fileNameFormat is a (golang) date format string
func NewFileCDRWriter(filePath string, fileNameFormat string, formatter CDRFormatter, rotateSeconds int64) *FileCDRWriter {

	if err := os.MkdirAll(filePath, 0770); err != nil {
		panic("while initializing, could not create " + filePath + " :" + err.Error())
	}

	w := FileCDRWriter{
		packetChan:     make(chan interface{}, FILE_PACKET_BUFFER_SIZE),
		doneChan:       make(chan struct{}),
		formatter:      formatter,
		rotateSeconds:  rotateSeconds,
		filePath:       filePath,
		fileNameFormat: fileNameFormat,
	}

	// The first file will be created with the first CDR

	go w.eventLoop()

	return &w
}

func (w *FileCDRWriter) eventLoop() {

	// The loop will finish when the packet channel is closed, which occurs
	// when invoking Close()
	for p := range w.packetChan {

		// Check if we must rotate
		if time.Now().Unix() >= w.currentFileTimestamp+w.rotateSeconds {
			w.rotateFile()
		}

		switch v := p.(type) {

		case *core.RadiusPacket:

			_, err := w.file.WriteString(w.formatter.GetRadiusCDRString(v))

			if err != nil {
				panic("file write error. Filename: " + w.file.Name() + "error: " + err.Error())
			}

		case *core.DiameterMessage:

			_, err := w.file.WriteString(w.formatter.GetDiameterCDRString(v))

			if err != nil {
				panic("file write error. Filename: " + w.file.Name() + "error: " + err.Error())
			}
		}
	}

	close(w.doneChan)
}

// Writes the Radius CDR to file
func (w *FileCDRWriter) WriteRadiusCDR(rp *core.RadiusPacket) {
	w.packetChan <- rp
}

// Writes the Diameter CDR
func (w *FileCDRWriter) WriteDiameterCDR(dm *core.DiameterMessage) {
	w.packetChan <- dm
}

// Must be called in the eventLoop
func (w *FileCDRWriter) rotateFile() {

	if w.file != nil {
		w.file.Close()
	}

	fileName := w.filePath + "/" + time.Now().Format(w.fileNameFormat)
	// Sanity check
	if fileName == w.currentFileName {
		panic("File name not changed when rotating: " + fileName)
	}

	// Adding O_APPEND to avoid failure if the file already exists (e.g. program
	// terminated and started again quickly)
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0770)
	if err != nil {
		panic("while rotating, could not create " + fileName + " due to " + err.Error())
	}
	w.file = file
	w.currentFileTimestamp = time.Now().Unix()

}

// Call when sure that no more write operations will be invoked
func (w *FileCDRWriter) Close() {
	close(w.packetChan)

	// Consume all the pending CDR in the buffer
	<-w.doneChan

	if w.file != nil {
		w.file.Close()
	}
}
