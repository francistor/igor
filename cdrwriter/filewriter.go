package cdrwriter

import (
	"os"
	"time"

	"github.com/francistor/igor/diamcodec"
	"github.com/francistor/igor/radiuscodec"
)

const (
	PACKET_BUFFER_SIZE = 1000
)

// Writes files rotating by date
// The date in the name of the file follows the creation date. Dates of the CDR stored
// may span a longer time than implied in the file name.
type FileCDRWriter struct {

	// This channel will receive the CDR to write
	// TODO: re-write as either *RadiusPacket or *DiameterMessage
	packetChan chan interface{}

	// To singal that we have finished processing CDR
	doneChan chan struct{}

	// Externally created, holding the method to format the CDR
	formatter CDRFormatter

	// Timestamp in unix seconds for the currently being used file
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
func NewFileCDRWriter(filePath string, fileNameFormat string, formatter CDRFormatter, rotateSeconds int64) *FileCDRWriter {

	if err := os.MkdirAll(filePath, 0770); err != nil {
		panic("while initializing, could not create " + filePath + " :" + err.Error())
	}

	w := FileCDRWriter{
		packetChan:     make(chan interface{}, PACKET_BUFFER_SIZE),
		doneChan:       make(chan struct{}),
		formatter:      formatter,
		rotateSeconds:  rotateSeconds,
		filePath:       filePath,
		fileNameFormat: fileNameFormat,
	}

	w.rotateFile()

	go w.eventLoop()

	return &w
}

func (w *FileCDRWriter) eventLoop() {

	for p := range w.packetChan {

		// Check if we must rotate
		if time.Now().Unix() >= w.currentFileTimestamp+w.rotateSeconds {
			w.rotateFile()
		}

		switch v := p.(type) {

		case *radiuscodec.RadiusPacket:

			_, err := w.file.WriteString(w.formatter.GetRadiusCDRString(v))

			if err != nil {
				panic("file write error. Filename: " + w.file.Name() + "error: " + err.Error())
			}

		case *diamcodec.DiameterMessage:

			_, err := w.file.WriteString(w.formatter.GetDiameterCDRString(v))

			if err != nil {
				panic("file write error. Filename: " + w.file.Name() + "error: " + err.Error())
			}
		}
	}

	close(w.doneChan)

}

// Writes the Radius CDR to file
func (w *FileCDRWriter) WriteRadiusCDR(rp *radiuscodec.RadiusPacket) {
	w.packetChan <- rp
}

// Writes the Radius Diameter to file
func (w *FileCDRWriter) WriteDiameterCDR(dm *diamcodec.DiameterMessage) {
	w.packetChan <- dm
}

// Must be called in the eventLoop
func (w *FileCDRWriter) rotateFile() {

	if w.file != nil {
		w.file.Close()
	}

	fileName := w.filePath + "/" + time.Now().Format(w.fileNameFormat) + ".txt"
	// Sanity check
	if fileName == w.currentFileName {
		panic("File name not changed when rotating: " + fileName)
	}

	// Will fail if the file already exists
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0770)
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

	w.file.Close()
}
