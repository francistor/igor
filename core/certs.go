package core

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path"
	"sync"
	"time"
)

var certMutex sync.Mutex

// Helper function go generate certificates on each execution
// Panics in case of any error
// Files are "cert.pem" and "key.pem" located in the parent of the bootstrap directory
// or in the current directory if this variable is not set (because the bootstrap file is remote)
// It returns the names of the files
// If environment variables IGOR_HTTPS_KEY_FILE and IGOR_HTTPS_CERT_FILE exist, use those instead
// and does not create certificates
func EnsureCertificates() (string, string) {

	certMutex.Lock()
	defer certMutex.Unlock()

	var certFile string = os.Getenv("IGOR_HTTPS_CERT_FILE")
	var keyFile string = os.Getenv("IGOR_HTTPS_KEY_FILE")

	// Check whether environment variables exist
	if certFile != "" && keyFile != "" {
		if path.IsAbs(certFile) {
			return certFile, keyFile
		} else {
			return IgorConfigBase + certFile, IgorConfigBase + keyFile
		}
	}

	// Locations of the files to be created are relative to the base config directory (bootstrap file)
	if IgorConfigBase != "" {
		certFile = IgorConfigBase + "../cert.pem"
		keyFile = IgorConfigBase + "../key.pem"
	} else {
		certFile = "cert.pem"
		keyFile = "key.pem"
	}

	// Return if the file already exists
	if _, err := os.Stat(certFile); err == nil {
		return certFile, keyFile
	}

	// Generation of the certificate and key

	// Generate private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic("could not generate private key " + err.Error())
	}

	// Generate public key
	publicKey := &privKey.PublicKey

	// Generate the certificate
	certTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Indra"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, // This certificate is for a CA
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add the endpoints to the certificate. It will be valid for the hostname and for "localhost"
	// Most likely the client will ingore verification of the certifiate anyway, since this is not
	// easy to get right in Kubernetes
	myHostname, _ := os.Hostname()
	certTemplate.DNSNames = append(certTemplate.DNSNames, myHostname, "localhost")

	// Serialize the certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, publicKey, privKey)
	if err != nil {
		panic("could not generate the certificate " + err.Error())
	}

	// Write certificate
	certOut, err := os.Create(certFile)
	if err != nil {
		panic("failed to open cert.pem for writing " + err.Error())
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		panic("failed to write data to cert.pem " + err.Error())
	}
	if err := certOut.Close(); err != nil {
		panic("error closing cert.pem " + err.Error())
	}

	// Write key
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		panic("Failed to open key.pem for writing " + err.Error())
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		panic("unable to marshal private key " + err.Error())
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		panic("failed to write data to key.pem " + err.Error())
	}

	if err := keyOut.Close(); err != nil {
		panic("Error closing key.pem " + err.Error())
	}

	return certFile, keyFile
}
