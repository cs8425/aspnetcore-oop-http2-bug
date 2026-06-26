# PoC Client (h2client.go) Usage & Verification Guide

This directory contains a custom HTTP client tool written in Go (`h2client.go`). Its primary purpose is to precisely simulate and reproduce the specific request patterns that trigger the immediate **502 Bad Gateway** error from the **IIS ASP.NET Core Module (ANCM)** in **Out-of-Process mode**, even though these requests are handled correctly in In-Process, Classic ASP, and Static File modes.

This tool bypasses high-level HttpClient optimizations to enforce explicit HTTP/2 frame sequences and HTTP/1.1 headers, serving as a standalone suite to test and analyze the hidden behaviors of the Windows hosting pipeline (HTTP.sys / IIS).

---

## Core Reproduction Mechanism & Context

In a production environment, when a front-end edge proxy like **Caddy Server (HTTP/3)** receives a `GET` request with an empty body and reverse-proxies it to **IIS (HTTP/2)**, Caddy's translation layer streams the empty body by sending:
1. A **`HEADERS` frame** without the `END_STREAM` flag (indicating more data is expected).
2. An empty **`DATA` frame** with the `END_STREAM` flag (terminating the stream).

By forcing the Go request's `ContentLength` to `-1` and `TransferEncoding` to `[]string{"chunked"}`, this tool guides Go's `net/http` transport layer into replicating the exact same split-frame behavior under HTTP/2, allowing for a localized and isolated reproduction of the bug.

---

## Command Line Flags

You can control the protocol, encoding, and body length using the following command-line flags:

* `-t`: The target URL to test (Default: `https://127.0.0.1:40443/dotnetapi/req-test`).
* `-proto`: The explicit ALPN protocol negotiation mode.
    * `h2` or `http2`: Enforce HTTP/2 only.
    * `http1.1`: Enforce HTTP/1.1 only.
    * `h2,http1.1`: Allow both, prioritizing HTTP/2 (Default).
* `-method`: The HTTP request method (Default: `GET`).
* `-enc`: The transfer encoding mode for the request body.
    * `chunked`: Forces `ContentLength` to `-1` and applies `Transfer-Encoding: chunked` (Default, used to reproduce the bug).
    * `fixed`: Uses fixed-length encoding with an explicit `Content-Length` header.
* `-len`: The body length in bytes (Default: `0`).

---

## Test Cases & Wire-Level Request Patterns

The following are the core test cases utilized during validation and how they look on the wire:

### Test Case 1: HTTP/2 GET Split-Frame Test (Core Bug Reproduction)
* **Command:**
    ```bash
    go run h2client.go -proto h2 -method GET -enc chunked -len 0
    ```
* **Wire-Level Pattern:**
    The tool establishes an HTTP/2 connection and sends two separate frames within the same stream:
    ```text
    [HTTP/2 Stream]
    └── Frame 1: HEADERS Frame (Flags: NONE) -> Contains :method=GET, :path=... (No END_STREAM)
    └── Frame 2: DATA Frame    (Flags: END_STREAM) -> Empty data block (Length: 0)
    ```
* **Backend Observations & Indicators:**
    * **Out-of-Process Mode**: IIS immediately responds with **`502 Bad Gateway`**. Wireshark captures verify that ANCM **never** initiates any loopback TCP connection to the backend Kestrel process.
    * **In-Process Mode**: Successfully returns `200 OK`. A key anomaly can be observed in the application's header dump: the protocol is reported as **`HTTP/2`** and method as **`GET`**, yet the request simultaneously contains a synthetically injected **`Transfer-Encoding: chunked`** header.
    * **Classic ASP Mode**: Successfully returns `200 OK`. IIS completely normalizes the stream down to the legacy pipeline, exposing the protocol as **`HTTP/1.1`** combined with the injected **`Transfer-Encoding: chunked`** header.
    * **IIS Static File Mode**: Successfully returns `200 OK`. Although request headers are invisible in static file execution, the entire IIS pipeline processes the request cleanly without throwing any 502 errors.

* Verbose HTTP/2 logs by golang server which in `/golang` and start with `GODEBUG=http2debug=2`

```
2026/06/26 10:50:35 http2: server connection from 127.0.0.1:49166 on 0x3b0c141cc200
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: wrote SETTINGS len=30, settings: MAX_FRAME_SIZE=1048576, MAX_CONCURRENT_STREAMS=250, MAX_HEADER_LIST_SIZE=1048896, HEADER_TABLE_SIZE=4096, INITIAL_WINDOW_SIZE=1048576
2026/06/26 10:50:35 http2: server: client 127.0.0.1:49166 said hello
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: wrote WINDOW_UPDATE len=4 (conn) incr=983041
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: read SETTINGS len=24, settings: ENABLE_PUSH=0, INITIAL_WINDOW_SIZE=4194304, MAX_FRAME_SIZE=16384, MAX_HEADER_LIST_SIZE=10485760
2026/06/26 10:50:35 http2: server read frame SETTINGS len=24, settings: ENABLE_PUSH=0, INITIAL_WINDOW_SIZE=4194304, MAX_FRAME_SIZE=16384, MAX_HEADER_LIST_SIZE=10485760
2026/06/26 10:50:35 http2: server processing setting [ENABLE_PUSH = 0]
2026/06/26 10:50:35 http2: server processing setting [INITIAL_WINDOW_SIZE = 4194304]
2026/06/26 10:50:35 http2: server processing setting [MAX_FRAME_SIZE = 16384]
2026/06/26 10:50:35 http2: server processing setting [MAX_HEADER_LIST_SIZE = 10485760]
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: wrote SETTINGS flags=ACK len=0
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: read WINDOW_UPDATE len=4 (conn) incr=1073741824
2026/06/26 10:50:35 http2: server read frame WINDOW_UPDATE len=4 (conn) incr=1073741824
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: read SETTINGS flags=ACK len=0
2026/06/26 10:50:35 http2: server read frame SETTINGS flags=ACK len=0
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: read HEADERS flags=END_HEADERS stream=1 len=40
2026/06/26 10:50:35 http2: decoded hpack field header field ":authority" = "127.0.0.1:8443"
2026/06/26 10:50:35 http2: decoded hpack field header field ":method" = "GET"
2026/06/26 10:50:35 http2: decoded hpack field header field ":path" = "/ref/req"
2026/06/26 10:50:35 http2: decoded hpack field header field ":scheme" = "https"
2026/06/26 10:50:35 http2: decoded hpack field header field "user-agent" = "Go-http-client/2.0"
2026/06/26 10:50:35 http2: server read frame HEADERS flags=END_HEADERS stream=1 len=40
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: read DATA flags=END_STREAM stream=1 len=0 data=""
2026/06/26 10:50:35 http2: server read frame DATA flags=END_STREAM stream=1 len=0 data=""
2026/06/26 10:50:35 [req]Method: GET HTTP/2.0
RequestURI: /ref/req
RemoteAddr: 127.0.0.1:49166
User-Agent: [Go-http-client/2.0]


2026/06/26 10:50:35 http2: server encoding header ":status" = "200"
2026/06/26 10:50:35 http2: server encoding header "content-type" = "text/plain"
2026/06/26 10:50:35 http2: server encoding header "content-length" = "103"
2026/06/26 10:50:35 http2: server encoding header "date" = "Fri, 26 Jun 2026 02:50:35 GMT"
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: wrote HEADERS flags=END_HEADERS stream=1 len=38
2026/06/26 10:50:35 http2: Framer 0x3b0c14332540: wrote DATA flags=END_STREAM stream=1 len=103 data="Method: GET HTTP/2.0\nRequestURI: /ref/req\nRemoteAddr: 127.0.0.1:49166\nUser-Agent: [Go-http-client/2.0]\n"
```


### Test Case 2: HTTP/1.1 GET Chunked Test
* **Command:**
    ```bash
    go run h2client.go -proto http1.1 -method GET -enc chunked -len 0
    ```
* **Wire-Level Pattern:**
    Sends a traditional HTTP/1.1 GET request containing a literal chunked encoding header:
    ```http
    GET /dotnetapi/req-test HTTP/1.1
    Host: localhost:40443
    Transfer-Encoding: chunked

    0
    ```
* **Backend Observations & Indicators:**
    * **Out-of-Process Mode**: Triggers the same ANCM proxy translation flaw, instantly yielding a **`502 Bad Gateway`** with no backend connection established.
    * **In-Process / Classic ASP Mode**: Both successfully return `200 OK`, and both observe the protocol as **`HTTP/1.1`** with the **`Transfer-Encoding: chunked`** header in their respective dumps.
    * **IIS Static File Mode**: Successfully returns `200 OK`.

### Test Case 3: Control Group - Standard HTTP/2 GET
* **Command:**
    ```bash
    go run h2client.go -proto h2 -method GET -enc fixed -len 0
    ```
* **Wire-Level Pattern:**
    A standard and valid HTTP/2 GET request where the stream is terminated immediately on the headers:
    ```text
    [HTTP/2 Stream]
    └── Frame 1: HEADERS Frame (Flags: END_STREAM|END_HEADERS) -> Contains all headers and terminates the stream
    ```
* **Backend Observations & Indicators:**
    * **All Modes (including Out-of-Process)**: All paths return **`200 OK`** normally, and no unexpected `Transfer-Encoding` header is injected. This proves the bug is exclusively isolated to scenarios where the internal chunked state is triggered.

* Verbose HTTP/2 logs by golang server which in `/golang` and start with `GODEBUG=http2debug=2`

```
2026/06/26 11:02:51 http2: server connection from 127.0.0.1:53391 on 0x3b0c141cc200
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: wrote SETTINGS len=30, settings: MAX_FRAME_SIZE=1048576, MAX_CONCURRENT_STREAMS=250, MAX_HEADER_LIST_SIZE=1048896, HEADER_TABLE_SIZE=4096, INITIAL_WINDOW_SIZE=1048576
2026/06/26 11:02:51 http2: server: client 127.0.0.1:53391 said hello
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: wrote WINDOW_UPDATE len=4 (conn) incr=983041
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: read SETTINGS len=24, settings: ENABLE_PUSH=0, INITIAL_WINDOW_SIZE=4194304, MAX_FRAME_SIZE=16384, MAX_HEADER_LIST_SIZE=10485760
2026/06/26 11:02:51 http2: server read frame SETTINGS len=24, settings: ENABLE_PUSH=0, INITIAL_WINDOW_SIZE=4194304, MAX_FRAME_SIZE=16384, MAX_HEADER_LIST_SIZE=10485760
2026/06/26 11:02:51 http2: server processing setting [ENABLE_PUSH = 0]
2026/06/26 11:02:51 http2: server processing setting [INITIAL_WINDOW_SIZE = 4194304]
2026/06/26 11:02:51 http2: server processing setting [MAX_FRAME_SIZE = 16384]
2026/06/26 11:02:51 http2: server processing setting [MAX_HEADER_LIST_SIZE = 10485760]
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: wrote SETTINGS flags=ACK len=0
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: read WINDOW_UPDATE len=4 (conn) incr=1073741824
2026/06/26 11:02:51 http2: server read frame WINDOW_UPDATE len=4 (conn) incr=1073741824
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: read HEADERS flags=END_STREAM|END_HEADERS stream=1 len=37
2026/06/26 11:02:51 http2: decoded hpack field header field ":authority" = "127.0.0.1:8443"
2026/06/26 11:02:51 http2: decoded hpack field header field ":method" = "GET"
2026/06/26 11:02:51 http2: decoded hpack field header field ":path" = "/ref/req"
2026/06/26 11:02:51 http2: decoded hpack field header field ":scheme" = "https"
2026/06/26 11:02:51 http2: decoded hpack field header field "user-agent" = "Go-http-client/2.0"
2026/06/26 11:02:51 http2: server read frame HEADERS flags=END_STREAM|END_HEADERS stream=1 len=37
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: read SETTINGS flags=ACK len=0
2026/06/26 11:02:51 [req]Method: GET HTTP/2.0
RequestURI: /ref/req
RemoteAddr: 127.0.0.1:53391
User-Agent: [Go-http-client/2.0]


2026/06/26 11:02:51 http2: server read frame SETTINGS flags=ACK len=0
2026/06/26 11:02:51 http2: server encoding header ":status" = "200"
2026/06/26 11:02:51 http2: server encoding header "content-type" = "text/plain"
2026/06/26 11:02:51 http2: server encoding header "content-length" = "103"
2026/06/26 11:02:51 http2: server encoding header "date" = "Fri, 26 Jun 2026 03:02:51 GMT"
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: wrote HEADERS flags=END_HEADERS stream=1 len=38
2026/06/26 11:02:51 http2: Framer 0x3b0c142ea1c0: wrote DATA flags=END_STREAM stream=1 len=103 data="Method: GET HTTP/2.0\nRequestURI: /ref/req\nRemoteAddr: 127.0.0.1:53391\nUser-Agent: [Go-http-client/2.0]\n"
```

---

## Summary of Findings & Facts

1. **Synthetic Header Injection:**
   When encountering a split-frame HTTP/2 stream (Test Case 1) or an HTTP/1.1 chunked request (Test Case 2), the underlying hosting environment automatically injects a **`Transfer-Encoding: chunked`** header into the pipeline state. *(Note: Without public source code, it is impossible to definitively distinguish whether HTTP.sys or an internal IIS pipeline component handles this injection, but it is guaranteed to be synthetic).*
2. **ANCM Translation Layer Defect:**
   In Out-of-Process mode, the ANCM module (C++) intercepts this mutated request state from upstream. Upon seeing a `GET` request tied with the internal `Transfer-Encoding: chunked` state (under either H1.1 or H2), **ANCM's proxy translation layer chokes and aborts**, immediately returning a 502 before invoking `connect()` to establish a backend loopback socket.
3. **The Architectural Contradiction:**
   The failure is completely self-inflicted: ANCM Out-of-Process fails because it rejects a `GET` request for having a `Transfer-Encoding` configuration that **its own underlying infrastructure injected** to represent the incoming stream.
