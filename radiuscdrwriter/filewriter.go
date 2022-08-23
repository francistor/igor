package cdrwriter

import (
	"igor/radiuscodec"
	"os"
	"time"
)

const (
	PACKET_BUFFER_SIZE = 1000
)

// Writes files rotating by date
// The date in the name of the file follows the creation date. Dates of the CDR stored
// may span a longer time than implied in the file name.
type FileCDRWriter struct {

	// This channel will receive the CDR to write
	packetChan chan *radiuscodec.RadiusPacket

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
		packetChan:     make(chan *radiuscodec.RadiusPacket, PACKET_BUFFER_SIZE),
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

	// TODO: Check that the for loop finishes when the channel is closed
	for p := range w.packetChan {

		// Check if we must rotate
		if time.Now().Unix() >= w.currentFileTimestamp+w.rotateSeconds {
			w.rotateFile()
		}

		// Write CDR to file
		_, err := w.file.WriteString(w.formatter.GetCDRString(p))

		if err != nil {
			panic("file write error. Filename: " + w.file.Name() + "error: " + err.Error())
		}
	}

	close(w.doneChan)

}

// Writes the CDR to file
func (w *FileCDRWriter) WriteCDR(rp *radiuscodec.RadiusPacket) {
	w.packetChan <- rp
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
