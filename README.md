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

#### TODO

 * HTTPS support
 * more helper methods
