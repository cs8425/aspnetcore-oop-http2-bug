package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"
)

var (
	targetUrl   = flag.String("t", "https://127.0.0.1:40443/dotnetapi/req-test", "url to test")
	targetProto = flag.String("proto", "h2,http1.1", "http protocol to test (h2, http1.1, or h2,http1.1)")
	bodyLen     = flag.Int("len", 0, "body length (0 to any, default 0)")
	encoding    = flag.String("enc", "chunked", `chunked for "Transfer-Encoding: chunked"; fixed for "Content-Length" fixed length`)
	method      = flag.String("method", "GET", "request method")
)

func main() {
	flag.Parse()

	url := *targetUrl

	// create a custom TLS configuration with explicit ALPN protocols
	var nextProtos []string
	pMode := strings.ToLower(*targetProto)

	switch pMode {
	case "h2", "http2":
		// h2 only
		nextProtos = []string{"h2"}
	case "http1.1":
		// http 1.1 only
		nextProtos = []string{"http/1.1"}
	default:
		// Default: both h2 and http1.1
		nextProtos = []string{"h2", "http/1.1"}
	}
	tlsConfig := &tls.Config{
		NextProtos:         nextProtos,
		InsecureSkipVerify: true,
	}

	// for http client using http2 http1.1
	proto := &http.Protocols{}
	if slices.Contains(nextProtos, "h2") {
		proto.SetHTTP2(true)
	}
	if slices.Contains(nextProtos, "http1.1") {
		proto.SetHTTP1(true)
	}

	// configure the Transport to use TLS settings
	transport := &http.Transport{
		TLSClientConfig:    tlsConfig,
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
		Protocols:          proto,
	}

	// create the HTTP Client
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	// build request
	// req, err := http.NewRequest(http.MethodGet, url, nil)
	req, err := http.NewRequest(strings.ToUpper(*method), url, nil)
	if err != nil {
		log.Fatal(err)
	}

	// build body
	dummyData := make([]byte, *bodyLen)
	for idx := range dummyData {
		dummyData[idx] = '#'
	}
	dummyBody := io.NopCloser(bytes.NewReader(dummyData))
	req.Body = dummyBody

	switch *encoding {
	case "fixed":
		req.ContentLength = int64(len(dummyData))
		req.TransferEncoding = nil
		if *bodyLen == 0 {
			req.Body = http.NoBody // same as http.NewRequest() do with nil body
		}
	default:
		fallthrough
	case "chunked":
		// force body length unknown and chunked
		req.ContentLength = -1
		req.TransferEncoding = []string{"chunked"}
	}

	fmt.Println("--------- Req ---------")
	fmt.Printf("Url         : %v\n", url)
	fmt.Printf("Host        : %v\n", req.Host)
	fmt.Printf("Scheme      : %v\n", req.URL.Scheme)
	fmt.Printf("UrlHost     : %v\n", req.URL.Host)
	fmt.Printf("UrlPath     : %v\n", req.URL.Path)
	fmt.Printf("Method      : %v\n", req.Method)
	fmt.Printf("ContentLen  : %v\n", req.ContentLength)
	fmt.Printf("TransferEnc : %v\n", req.TransferEncoding)
	fmt.Printf("ProtoMode   : %v\n", nextProtos)

	// Send a GET request
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	elapsed := time.Since(start)

	// Output results
	fmt.Println("--------- Resp ---------")
	fmt.Printf("Status   : %v\n", resp.Status)
	fmt.Printf("Protocol : %v\n", resp.Proto)
	fmt.Printf("Elapsed  : %v\n", elapsed)
	fmt.Println("--------- Resp Header ---------")
	for k, v := range resp.Header {
		fmt.Printf(" - %v: %v\n", k, v)
	}
	fmt.Println("--------- Body ---------")
	fmt.Print(string(body))
}

type nopStream struct{}

func (s *nopStream) Close() error { return nil }

func (s *nopStream) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}
