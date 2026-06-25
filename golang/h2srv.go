package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	bind = flag.String("l", ":8443", "the address and port to bind")

	keyFile  = flag.String("key", "server.key", "path to the TLS private key file")
	certFile = flag.String("crt", "server.crt", "path to the TLS certificate file")
)

func main() {
	// parse CLI options
	flag.Parse()

	// check start with "GODEBUG=http2debug=2" for http2 debug info
	h2debugFound := false
	goEnv := os.Getenv("GODEBUG")
	for v := range strings.SplitSeq(goEnv, ",") {
		kv := strings.SplitN(v, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if kv[0] != "http2debug" {
			continue
		}
		if kv[1] != "2" {
			continue
		}
		h2debugFound = true
		break
	}
	if !h2debugFound {
		log.Println("Warning: verbose HTTP/2 logs are disabled! Set GODEBUG=http2debug=2 to enable!")
	}

	// ensure certificates exist
	if err := ensureCertificates(*certFile, *keyFile); err != nil {
		log.Fatalf("Failed to prepare certificates: %v", err)
	}

	// define the handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		buf := &bytes.Buffer{}
		fmt.Fprintf(buf, "Method: %v %v\n", r.Method, r.Proto)
		fmt.Fprintf(buf, "RequestURI: %v\n", r.RequestURI)
		fmt.Fprintf(buf, "RemoteAddr: %v\n", r.RemoteAddr)
		for k, v := range r.Header {
			fmt.Fprintf(buf, "%v: %v\n", k, v)
		}
		out := buf.Bytes()
		log.Printf("[req]%v\n\n", string(out))
		buf.WriteTo(w)
	})

	// setup server
	server := &http.Server{
		Addr:         *bind,
		Handler:      handler,
		TLSConfig:    nil, // use nil to use the standard TLS setup
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Starting server on %v...\n", *bind)

	// start the HTTP/2 Server
	// automatically enables HTTP/2 if supported by the client
	err := server.ListenAndServeTLS(*certFile, *keyFile)
	if err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// ensureCertificates checks for files, generates them if missing
func ensureCertificates(certFile string, keyFile string) error {
	if _, err := os.Stat(certFile); err == nil {
		log.Println("Found existing certificates.")
		return nil
	}

	log.Println("Generating new self-signed certificate...")

	// Generate RSA Key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Create Certificate Template
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // Valid for 1 year

	serialNumber, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"Go HTTP/2 Server"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create the certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	// Write Certificate to file
	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	// Write Private Key to file
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	log.Println("Certificates generated successfully.")
	return nil
}
