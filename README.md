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
    MaxConnsPerHost  int
    RedirectPolicy   func(*http.Request, []*http.Request) error
    TLSClientConfig  *tls.Config
}

func New() *HttpClient
    create a new HttpClient all options should be set on the instance
    returned

func (h *HttpClient) Do(req *http.Request) (*http.Response, error)
    perform the specified request

func (h *HttpClient) FinishRequest(req *http.Request) error
    perform final cleanup for the specified request *must* be called for
    every request performed after processing is finished and after which
    GetConn will no longer return successfully

func (h *HttpClient) Get(url string) (*http.Response, error)
    convenience method to perform a HTTP GET request

func (h *HttpClient) GetConn(req *http.Request) (net.Conn, error)
    returns the connection associated with the specified request cannot be
    called after FinishRequest

func (h *HttpClient) Post(url string, contentType string, body io.Reader) (*http.Response, error)
    convenience method to perform a HTTP POST request

func (h *HttpClient) RoundTrip(req *http.Request) (*http.Response, error)
    satisfies the RoundTripper interface and handles checking the connection
    cache or dialing (with ConnectTimeout)

func DefaultRedirectPolicy(req *http.Request, via []*http.Request) error
    default redirect policy which fails after 3 redirects.

func Version() string
    returns the current version
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
    httpClient := httpclient.New()
    httpClient.ConnectTimeout = time.Second
    httpClient.ReadWriteTimeout = time.Second

    // Allow insecure HTTPS connections.  Note: the TLSClientConfig pointer can't change
    // places, so you can only modify the existing tls.Config object
	httpClient.TLSClientConfig.InsecureSkipVerify = true

    // Make a custom redirect policy to keep track of the number of redirects we've followed
    var numRedirects int
    httpClient.RedirectPolicy = func(r *http.Request, v []*http.Request) error {
        numRedirects += 1
        return DefaultRedirectPolicy(r, v)
    }

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
