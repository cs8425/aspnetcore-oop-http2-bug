# IIS ASP.NET Core Module (Out-of-Process) 502 Bug Reproduction

This repository provides a test suite to reproduce and diagnose a critical issue where the **IIS ASP.NET Core Module (ANCM)** in **Out-of-Process mode** returns an immediate **502 Bad Gateway** for specific request patterns, even though these requests are handled correctly in In-Process, Classic ASP, and Static File modes.

## The Discovery Story & Context

We discovered this issue in a production environment where **Caddy Server** was configured as a front-end edge proxy supporting **HTTP/3 (QUIC)**, reverse-proxying requests to a backend IIS server hosting a legacy ASP.NET Core application over **HTTP/2**.

During operation, we observed intermittent `502 Bad Gateway` errors from IIS. Detailed troubleshooting revealed:

1. When clients connected to Caddy using **HTTP/2**, requests were processed successfully.
2. When clients connected to Caddy using **HTTP/3**, the requests always failed with `502`.
3. Caddy's QUIC-to-H2 translation layer streams the request body (even if empty) by sending a `HEADERS` frame without `END_STREAM`, followed by an empty `DATA` frame with `END_STREAM`.
4. This specific HTTP/2 frame sequence triggers a fatal translation failure inside IIS ANCM Out-of-Process mode.

---

## The Core Problem & Technical Conflict

The ANCM (Out-of-Process) fails to proxy requests to the backend process under two conditions:

1. **HTTP/2:** A `GET` request with body stream separated into `HEADERS` and `DATA` frames.
2. **HTTP/1.1:** A `GET` request containing `Transfer-Encoding: chunked`.

### The Core Architectural Bug:

* **The Synthetic Injection Behavior:** When a chunked HTTP/1.1 request or an HTTP/2 stream with split HEADERS/DATA frames arrives, the underlying infrastructure (`HTTP.sys` or IIS) accepts it. To represent this incoming stream within the pipeline, **the hosting environment synthetically injects a `Transfer-Encoding: chunked` header** into the request state. *(Note: We cannot definitively pinpoint whether `HTTP.sys` or the IIS pipeline is responsible for this injection).*
* **The Critical Pipeline Clue (In-Process vs Classic ASP):**
  * In **Classic ASP**, IIS entirely normalizes the request down to `HTTP/1.1`, exposing `HTTP/1.1 + Transfer-Encoding: chunked` to the script for both HTTP/1.1 and HTTP/2 client requests.
  * In **ASP.NET Core In-Process** mode, for an HTTP/2 client request, `HttpContext` reports the protocol natively as `HTTP/2` and method as `GET`, **yet it simultaneously contains the synthetically injected `Transfer-Encoding: chunked` header!** This specific combination (`HTTP/2` protocol + `Transfer-Encoding`) is technically illegal under strict HTTP/2 RFCs, proving that the hosting environment artificially mutates the header collection.
* **The Out-of-Process Failure:** In **Out-of-Process** mode, ANCM (a separate C++ module) receives this mutated request state. Upon seeing a `GET` method combined with the injected `Transfer-Encoding: chunked` header (whether under H1.1 or H2), **ANCM's proxy-translation layer instantly chokes and aborts**, returning a 502 before establishing any TCP connection to Kestrel.
* **Technical Conflict:** ANCM is rejecting the request with a 502 due to a header configuration (`Transfer-Encoding` on a `GET` request) that **was synthetically injected by underlying infrastructure**, and not by the actual external client!

---

## Comparison Matrix

| Client Request Type | IIS Mode / Handler | Result | Observed Protocol (via App/Script) | Observed Headers | Technical Detail |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **HTTP/2 GET + DATA** | Out-of-Process | ❌ **502** | *No Connection* | *No Connection* | Rejected by ANCM translation layer. |
| **HTTP/2 GET + DATA** | In-Process | ✅ 200 | `HTTP/2` | `Transfer-Encoding: chunked` | App receives `HTTP/2` protocol but with an artificially injected chunked header. |
| **HTTP/2 GET + DATA** | Classic ASP | ✅ 200 | `HTTP/1.1` | `Transfer-Encoding: chunked` | IIS normalizes the H2 stream down to H1.1 for the classic pipeline. |
| **HTTP/2 GET + DATA** | Static File | ✅ 200 | N/A | N/A | Processes normally; no 502 error occurs. |
| **HTTP/1.1 GET (Chunked)** | Out-of-Process | ❌ **502** | *No Connection* | *No Connection* | Rejected by ANCM translation layer. |
| **HTTP/1.1 GET (Chunked)** | In-Process | ✅ 200 | `HTTP/1.1` | `Transfer-Encoding: chunked` | App receives standard chunked request state. |
| **HTTP/1.1 GET (Chunked)** | Classic ASP | ✅ 200 | `HTTP/1.1` | `Transfer-Encoding: chunked` | Script receives standard chunked request state. |
| **HTTP/1.1 GET (Chunked)** | Static File | ✅ 200 | N/A | N/A | Processes normally; no 502 error occurs. |

---

## Project Structure

```text
.
├── client/           # Go-based HTTP/2 & HTTP/1.1 Clients (Simulates the bug)
├── golang/           # Go-based Web Server (To verify raw frames and headers)
├── dotnet/           # ASP.NET Core 10 Web API (Target app for Out-of-Process & In-Process)
└── asp/              # Classic ASP header-dump script (To verify IIS pipeline baseline)
```

### Components

* **`/client`**: Supports sending specialized GET requests to trigger the bug condition.
* **`/golang`**: A vanilla Go HTTPS server to verify that the client is sending frames/request correctly and to dump raw headers.
* **`/dotnet`**: The test ASP.NET Core Web API, configured to dump request headers and protocol metadata.
* **`/asp`**: A Classic ASP header-dumping script proving that the native IIS pipeline handles these patterns successfully and exposes the same injected `Transfer-Encoding` header under H1.1 normalization.

---

## Reproduction Steps

### 1. Environment Setup

* Windows with IIS enabled. (tested on Server 2019 and Win11 25H2)
* ASP.NET Core 10 Hosting Bundle installed.
* Go installed (to build the client and the reference server).

### 2. Deployment

* **Site A (Out-of-Process):** Deploy `/dotnet` in Out-of-Process mode.
* **Site B (In-Process):** Deploy `/dotnet` in In-Process mode.
* **Site C (Classic ASP):** Deploy `/asp` on IIS.
* **Site D (Static File):** Set up a standard static HTML file directory on IIS.

### 3. Test Execution & Verification

1. **Test Out-of-Process (Site A):** Run the client.
   * *Observed:* IIS immediately returns `502 Bad Gateway`.
   * *Verification:* Run Wireshark. Observe that **no TCP connection is ever initiated** to the backend port.
2. **Test In-Process (Site B):** Run the client.
   * *Observed:* Returns `200 OK`. Notice that for HTTP/2 client requests, the app dumps **`Protocol: HTTP/2`** but still contains the **`Transfer-Encoding: chunked`** header.
3. **Test Classic ASP (Site C):** Run the client.
   * *Observed:* Returns `200 OK`. The response dump shows it normalizes everything to `HTTP/1.1` and displays the injected `Transfer-Encoding: chunked` header.
4. **Test Static File (Site D):** Run the client against the static file path.
   * *Observed:* Returns `200 OK`. Flawless execution without 502.
5. **Test Go Reference Server:** Run the client against the `/golang` server.
   * *Observed:* Returns `200 OK`.

---

## Conclusion & Fixes

The issue resides entirely in the **C++ translation layer of ANCM (Out-of-Process)**. The module should be fixed to allow `GET` requests with `Transfer-Encoding: chunked` when proxying internally, especially since this state is natively induced by the underlying hosting environment (either HTTP.sys or the IIS pipeline) during HTTP/2 stream transitions.
