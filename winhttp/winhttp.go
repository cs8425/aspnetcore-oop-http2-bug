//go:build windows
// +build windows

package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WinHTTP Access Types
const (
	// WinHTTP!WinHttpOpen dwAccessType
	// https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpopen
	WINHTTP_ACCESS_TYPE_DEFAULT_PROXY   = 0
	WINHTTP_ACCESS_TYPE_NO_PROXY        = 1
	WINHTTP_ACCESS_TYPE_NAMED_PROXY     = 3
	WINHTTP_ACCESS_TYPE_AUTOMATIC_PROXY = 4

	// WinHTTP dwFlags
	WINHTTP_FLAG_NONE                 = 0x00000000
	WINHTTP_FLAG_ASYNC                = 0x10000000
	WINHTTP_FLAG_SECURE_DEFAULTS      = 0x30000000
	WINHTTP_FLAG_SECURE               = 0x00800000
	WINHTTP_FLAG_ESCAPE_PERCENT       = 0x00000004
	WINHTTP_FLAG_NULL_CODEPAGE        = 0x00000008
	WINHTTP_FLAG_ESCAPE_DISABLE       = 0x00000040
	WINHTTP_FLAG_ESCAPE_DISABLE_QUERY = 0x00000080
	WINHTTP_FLAG_BYPASS_PROXY_CACHE   = 0x00000100
	WINHTTP_FLAG_REFRESH              = WINHTTP_FLAG_BYPASS_PROXY_CACHE
	WINHTTP_FLAG_AUTOMATIC_CHUNKING   = 0x00000200

	WINHTTP_OPTION_SECURITY_FLAGS          = 31
	SECURITY_FLAG_IGNORE_UNKNOWN_CA        = 0x00000100
	SECURITY_FLAG_IGNORE_CERT_DATE_INVALID = 0x00002000
	SECURITY_FLAG_IGNORE_CERT_CN_INVALID   = 0x00001000
	SECURITY_FLAG_IGNORE_CERT_WRONG_USAGE  = 0x00000200
	SECURITY_FLAG_SECURE                   = 0x00000001
	SECURITY_FLAG_STRENGTH_WEAK            = 0x10000000
	SECURITY_FLAG_STRENGTH_MEDIUM          = 0x40000000
	SECURITY_FLAG_STRENGTH_STRONG          = 0x20000000

	WINHTTP_ADDREQ_FLAG_ADD_IF_NEW              = 0x10000000
	WINHTTP_ADDREQ_FLAG_ADD                     = 0x20000000
	WINHTTP_ADDREQ_FLAG_COALESCE_WITH_COMMA     = 0x40000000
	WINHTTP_ADDREQ_FLAG_COALESCE_WITH_SEMICOLON = 0x01000000
	WINHTTP_ADDREQ_FLAG_COALESCE                = WINHTTP_ADDREQ_FLAG_COALESCE_WITH_COMMA
	WINHTTP_ADDREQ_FLAG_REPLACE                 = 0x80000000
)

// Load winhttp.dll and resolve the necessary procedures
var (
	modWinHttp = windows.NewLazySystemDLL("winhttp.dll")

	procWinHttpOpen               = modWinHttp.NewProc("WinHttpOpen")
	procWinHttpConnect            = modWinHttp.NewProc("WinHttpConnect")
	procWinHttpOpenRequest        = modWinHttp.NewProc("WinHttpOpenRequest")
	procWinHttpSendRequest        = modWinHttp.NewProc("WinHttpSendRequest")
	procWinHttpReceiveResponse    = modWinHttp.NewProc("WinHttpReceiveResponse")
	procWinHttpQueryDataAvailable = modWinHttp.NewProc("WinHttpQueryDataAvailable")
	procWinHttpReadData           = modWinHttp.NewProc("WinHttpReadData")
	procWinHttpCloseHandle        = modWinHttp.NewProc("WinHttpCloseHandle")

	procWinHttpSetOption         = modWinHttp.NewProc("WinHttpSetOption")
	procWinHttpAddRequestHeaders = modWinHttp.NewProc("WinHttpAddRequestHeaders")
	procWinHttpWriteData         = modWinHttp.NewProc("WinHttpWriteData")
)

var (
	targetUrl = flag.String("t", "https://127.0.0.1:8443/dotnetapi/req-test", "url to test")
	method    = flag.String("method", "GET", "request method")
	bodyLen   = flag.Int("len", 0, "body length (0 to any, default 0)")
	bodySplit = flag.Int("sp", -1, "body length per write (-1 write all in one time, default -1)")
	bodySleep = flag.Int("sleep", 200, "sleep between write chunk (in ms, default: 200)")
)

func main() {
	flag.Parse()

	srvUrl, err := url.Parse(*targetUrl)
	if err != nil {
		fmt.Printf("url.Parse() failed: %v\n", err)
		return
	}
	port, err := strconv.ParseUint(srvUrl.Port(), 10, 16)
	if err != nil {
		fmt.Printf("Parse server port failed: %v\n", err)
		return
	}

	// Open a WinHTTP Session
	userAgentPtr, _ := windows.UTF16PtrFromString("GoWinHTTPClient/1.0")
	hSession, _, err := procWinHttpOpen.Call(
		uintptr(unsafe.Pointer(userAgentPtr)),
		WINHTTP_ACCESS_TYPE_DEFAULT_PROXY,
		0, // WINHTTP_NO_PROXY_NAME
		0, // WINHTTP_NO_PROXY_BYPASS
		0,
	)
	if hSession == 0 {
		fmt.Printf("WinHttpOpen failed: %v\n", err)
		return
	}
	defer procWinHttpCloseHandle.Call(hSession)

	// Connect to the Server (Host only, do not include https://)
	serverPtr, _ := windows.UTF16PtrFromString(srvUrl.Hostname())
	hConnect, _, err := procWinHttpConnect.Call(
		hSession,
		uintptr(unsafe.Pointer(serverPtr)),
		uintptr(port),
		0,
	)
	if hConnect == 0 {
		fmt.Printf("WinHttpConnect failed: %v\n", err)
		return
	}
	defer procWinHttpCloseHandle.Call(hConnect)

	// Open the Request
	methodPtr, _ := windows.UTF16PtrFromString(*method)
	pathPtr, _ := windows.UTF16PtrFromString(srvUrl.Path)
	hRequest, _, err := procWinHttpOpenRequest.Call(
		hConnect,
		uintptr(unsafe.Pointer(methodPtr)),
		uintptr(unsafe.Pointer(pathPtr)),
		0,                   // HTTP version (NULL means default)
		0,                   // WINHTTP_NO_REFERER
		0,                   // WINHTTP_DEFAULT_ACCEPT_TYPES
		WINHTTP_FLAG_SECURE, // Use TLS/SSL
	)
	if hRequest == 0 {
		fmt.Printf("WinHttpOpenRequest failed: %v\n", err)
		return
	}
	defer procWinHttpCloseHandle.Call(hRequest)

	// InsecureSkipVerify
	{
		flags := SECURITY_FLAG_IGNORE_UNKNOWN_CA | SECURITY_FLAG_IGNORE_CERT_CN_INVALID | SECURITY_FLAG_IGNORE_CERT_DATE_INVALID | SECURITY_FLAG_IGNORE_CERT_WRONG_USAGE
		buffer := make([]byte, 4)
		binary.LittleEndian.PutUint32(buffer, uint32(flags))
		r, _, err := procWinHttpSetOption.Call(
			hRequest,                               //	[in] HINTERNET hInternet
			uintptr(WINHTTP_OPTION_SECURITY_FLAGS), //	[in] DWORD     dwOption,
			uintptr(unsafe.Pointer(&buffer[0])),    // [in] LPVOID    lpBuffer,
			4,                                      // [in] DWORD     dwBufferLength
		)
		if r == 0 {
			fmt.Printf("WinHttpSetOption failed: %v\n", err)
			return
		}
	}

	// header
	{
		hdr := http.Header{
			"User-Agent":        []string{"WinHttp-Client"},
			"Transfer-Encoding": []string{"chunked"},
			"X-Test":            []string{"test"},
		}

		var headers string
		for k, v := range hdr {
			// Each header except the last must be terminated by a carriage return/line feed (CR/LF)
			headers += fmt.Sprintf("%s: %s", k, strings.Join(v, ", "))
			headers = strings.TrimSuffix(headers, ", ")
			headers += "\r\n"
		}
		headers = strings.TrimSuffix(headers, "\r\n")
		lpszHeaders, _ := windows.UTF16PtrFromString(headers)
		dwHeadersLength := uint32(len(headers))
		dwModifiers := WINHTTP_ADDREQ_FLAG_ADD
		r, _, err := procWinHttpAddRequestHeaders.Call(
			uintptr(hRequest),
			uintptr(unsafe.Pointer(lpszHeaders)),
			uintptr(dwHeadersLength),
			uintptr(dwModifiers),
		)
		if r == 0 {
			fmt.Printf("WinHttpAddRequestHeaders failed: %v\n", err)
		}
	}

	// 4. Send the Request using WinHttpSendRequest
	additionalHeadersPtr, _ := windows.UTF16PtrFromString("")
	dwHeadersLength := uint32(0)
	optionalDataLength := uint32(0)
	dwTotalLength := uint32(0) // WINHTTP_IGNORE_REQUEST_TOTAL_LENGTH
	_ = dwTotalLength
	success, _, err := procWinHttpSendRequest.Call(
		hRequest, // [in]           HINTERNET hRequest,
		uintptr(unsafe.Pointer(additionalHeadersPtr)), // [in, optional] LPCWSTR   lpszHeaders,
		uintptr(dwHeadersLength),                      // [in]           DWORD     dwHeadersLength,
		uintptr(0),                                    // [in, optional] LPVOID    lpOptional,
		uintptr(optionalDataLength),                   // [in]           DWORD     dwOptionalLength,
		uintptr(dwTotalLength),                        // [in]           DWORD     dwTotalLength,
		uintptr(0),                                    // [in]           DWORD_PTR dwContext
	)
	if success == 0 {
		fmt.Printf("WinHttpSendRequest failed: %v\n", err)
		return
	}

	// build and write body
	if *bodyLen > 0 {
		dummyData := make([]byte, *bodyLen)
		for idx := range dummyData {
			dummyData[idx] = '#'
		}
		wn := *bodySplit
		if wn < 0 || wn > len(dummyData) {
			wn = len(dummyData)
		}
		wBuf := dummyData
		i := 0
		written := 0
		chunkBuf := &bytes.Buffer{}
		for len(wBuf) > 0 {
			if wn > len(wBuf) {
				wn = len(wBuf)
			}

			chunkBuf.Reset()
			fmt.Fprintf(chunkBuf, "%x\r\n", wn)
			chunkBuf.Write(wBuf[:wn])
			fmt.Fprintf(chunkBuf, "\r\n")

			printChunkSz := chunkBuf.Len()
			if chunkBuf.Len() > 20 {
				printChunkSz = 30
			}
			fmt.Println("WinHttpWriteData:", i, written, wn, len(wBuf), chunkBuf.String()[:printChunkSz])
			n, err := WinHttpWriteData(hRequest, chunkBuf.Bytes())
			if err != nil {
				fmt.Println("WinHttpWriteData failed:", i, written, len(wBuf), n, err)
				return
			}

			wBuf = wBuf[wn:]
			written += int(n)
			i += 1

			time.Sleep(time.Duration(*bodySleep) * time.Millisecond)
		}
	}
	n, err := WinHttpWriteData(hRequest, []byte("0\r\n\r\n"))
	if err != nil {
		fmt.Println("WinHttpWriteData failed:", n, err)
	}

	// Receive the Response
	success, _, err = procWinHttpReceiveResponse.Call(hRequest, 0)
	if success == 0 {
		fmt.Printf("WinHttpReceiveResponse failed: %v\n", err)
		return
	}

	// Read Data from the Response Stream
	var completeResponseBody []byte
	for {
		var bytesAvailable uint32
		_, _, _ = procWinHttpQueryDataAvailable.Call(
			hRequest,
			uintptr(unsafe.Pointer(&bytesAvailable)),
		)

		if bytesAvailable == 0 {
			break // No more data to read
		}

		buffer := make([]byte, bytesAvailable)
		var bytesRead uint32

		_, _, _ = procWinHttpReadData.Call(
			hRequest,
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(bytesAvailable),
			uintptr(unsafe.Pointer(&bytesRead)),
		)

		completeResponseBody = append(completeResponseBody, buffer[:bytesRead]...)
	}

	// Output results
	fmt.Println("Response Received Successfully:")
	fmt.Println(string(completeResponseBody))
}

func WinHttpWriteData(hRequest uintptr, buffer []byte) (uint32, error) {
	var bytesWritten uint32
	r0, _, err := procWinHttpWriteData.Call(
		uintptr(hRequest),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		uintptr(unsafe.Pointer(&bytesWritten)),
	)

	if r0 == 0 {
		return bytesWritten, err
	}
	return bytesWritten, nil
}
