## HttpClient

HttpClient wraps Go's built in HTTP client providing an API to:

 * set connect timeout
 * set read/write timeout
 * easy access to the connection object for a given request

```go
package httpclient

type HttpClient struct {
    ConnectTimeout   time.Duration
    ReadWriteTimeout time.Duration
    MaxRedirects     int
    MaxConnsPerHost  int
}

func New() *HttpClient

func (h *HttpClient) Do(req *http.Request) (*http.Response, error)

func (h *HttpClient) FinishRequest(req *http.Request) error

func (h *HttpClient) GetConn(req *http.Request) (net.Conn, error)

func (h *HttpClient) RoundTrip(req *http.Request) (*http.Response, error)
```

#### Example

```go
package main

import (
    "httpclient"
    "io/ioutil"
    "log"
    "net/http"
    "time"
)

func main() {
    httpClient := New()
    httpClient.ConnectTimeout = time.Second
    httpClient.ReadWriteTimeout = time.Second

    req, _ := http.NewRequest("GET", "http://127.0.0.1/test", nil)

    resp, err := httpClient.Do(req)
    if err != nil {
        log.Fatalf("request failed - %s", err.Error())
    }
    defer resp.Body.Close()

    conn, err := httpClient.GetConn(req)
    if err != nil {
        log.Fatalf("failed to get conn for req")
    }
    // do something with conn

    body, err := ioutil.ReadAll(resp.Body)
    log.Printf("%s", body)

    httpClient.FinishRequest(req)
}
```

#### TODO

 * HTTPS support
