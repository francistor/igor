package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"
)

func TestCertificate(t *testing.T) {

	// Generate private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("could not generate private key", err)
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
		t.Fatal("could not generate the certificate", err)
	}

	// Write certificate
	certOut, err := os.Create("cert.pem")
	if err != nil {
		t.Fatal("failed to open cert.pem for writing", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		t.Fatal("failed to write data to cert.pem", err)
	}
	if err := certOut.Close(); err != nil {
		t.Fatal("error closing cert.pem", err)
	}

	// Write key
	keyOut, err := os.OpenFile("key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal("Failed to open key.pem for writing", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		t.Fatal("unable to marshal private key", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		t.Fatal("failed to write data to key.pem", err)
	}

	if err := keyOut.Close(); err != nil {
		t.Fatal("Error closing key.pem", err)
	}
}
