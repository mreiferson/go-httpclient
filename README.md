## go-httpclient

NOTE: **Requires Go 1.1+** - as of `0.4.0` the API has been completely re-written for Go 1.1 (for Go 1.0.x compatible release see the
[go1](https://github.com/mreiferson/go-httpclient/tree/go1) tag)

[![Build
Status](https://secure.travis-ci.org/mreiferson/go-httpclient.png)](http://travis-ci.org/mreiferson/go-httpclient)

Provides an HTTP Transport that implements the `RoundTripper` interface and
can be used as a built in replacement for the standard library's, providing:

 * connection timeouts
 * request timeouts

Internally, it uses a priority queue maintained in a single goroutine
(per *client* instance), leveraging the Go 1.1+ `CancelRequest()` API.


### Example

```go
transport := &httpclient.Transport{
    ConnectTimeout:        1*time.Second,
    RequestTimeout:        10*time.Second,
    ResponseHeaderTimeout: 5*time.Second,
}
defer transport.Close()

client := &http.Client{Transport: transport}
req, _ := http.NewRequest("GET", "http://127.0.0.1/test", nil)
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()
```

### Reference Docs

For API docs see [godoc](http://godoc.org/github.com/mreiferson/go-httpclient).
