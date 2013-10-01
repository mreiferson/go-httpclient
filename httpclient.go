/*
Provides an HTTP Transport that implements the `RoundTripper` interface and
can be used as a built in replacement for the standard library's, providing:

	* connection timeouts
	* request timeouts

Internally, it uses a priority queue maintained in a single goroutine
(per *client* instance), leveraging the Go 1.1+ `CancelRequest()` API.
*/
package httpclient

import (
	"container/heap"
	"crypto/tls"
	"github.com/mreiferson/go-httpclient/pqueue"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// returns the current version of the package
func Version() string {
	return "0.4.0"
}

// Transport implements the RoundTripper interface and can be used as a replacement
// for Go's built in http.Transport implementing end-to-end request timeouts.
//
// 	transport := &httpclient.Transport{
// 	    ConnectTimeout: 1*time.Second,
// 	    ResponseHeaderTimeout: 5*time.Second,
// 	    RequestTimeout: 10*time.Second,
// 	}
// 	defer transport.Close()
// 	
// 	client := &http.Client{Transport: transport}
// 	req, _ := http.NewRequest("GET", "http://127.0.0.1/test", nil)
// 	resp, err := client.Do(req)
// 	if err != nil {
// 	    return err
// 	}
// 	defer resp.Body.Close()
//
type Transport struct {
	sync.Mutex

	// Proxy specifies a function to return a proxy for a given
	// *http.Request. If the function returns a non-nil error, the
	// request is aborted with the provided error.
	// If Proxy is nil or returns a nil *url.URL, no proxy is used.
	Proxy func(*http.Request) (*url.URL, error)

	// TLSClientConfig specifies the TLS configuration to use with
	// tls.Client. If nil, the default configuration is used.
	TLSClientConfig *tls.Config

	// DisableKeepAlives, if true, prevents re-use of TCP connections
	// between different HTTP requests.
	DisableKeepAlives bool

	// DisableCompression, if true, prevents the Transport from
	// requesting compression with an "Accept-Encoding: gzip"
	// request header when the Request contains no existing
	// Accept-Encoding value. If the Transport requests gzip on
	// its own and gets a gzipped response, it's transparently
	// decoded in the Response.Body. However, if the user
	// explicitly requested gzip it is not automatically
	// uncompressed.
	DisableCompression bool

	// MaxIdleConnsPerHost, if non-zero, controls the maximum idle
	// (keep-alive) to keep per-host.  If zero,
	// http.DefaultMaxIdleConnsPerHost is used.
	MaxIdleConnsPerHost int

	// ConnectTimeout, if non-zero, is the maximum amount of time a dial will wait for
	// a connect to complete.
	ConnectTimeout time.Duration

	// ResponseHeaderTimeout, if non-zero, specifies the amount of
	// time to wait for a server's response headers after fully
	// writing the request (including its body, if any). This
	// time does not include the time to read the response body.
	ResponseHeaderTimeout time.Duration

	// RequestTimeout, if non-zero, specifies the amount of time for the entire
	// request to complete (including all of the above timeouts + entire response body).
	// This should never be less than the sum total of the above two timeouts.
	RequestTimeout time.Duration

	starter   sync.Once
	transport *http.Transport
	requests  pqueue.PriorityQueue
	exitChan  chan int
}

// Close cleans up the Transport, making sure its goroutine has exited
func (t *Transport) Close() error {
	if t.exitChan != nil {
		t.exitChan <- 1
		<-t.exitChan
	}
	return nil
}

func (t *Transport) lazyStart() {
	dialer := &net.Dialer{Timeout: t.ConnectTimeout}
	t.transport = &http.Transport{
		Dial:                  dialer.Dial,
		Proxy:                 t.Proxy,
		TLSClientConfig:       t.TLSClientConfig,
		DisableKeepAlives:     t.DisableKeepAlives,
		DisableCompression:    t.DisableCompression,
		MaxIdleConnsPerHost:   t.MaxIdleConnsPerHost,
		ResponseHeaderTimeout: t.ResponseHeaderTimeout,
	}
	t.requests = pqueue.New(16)
	if t.RequestTimeout > 0 {
		t.exitChan = make(chan int)
		go t.worker()
	}
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var item *pqueue.Item

	t.starter.Do(t.lazyStart)

	absTs := time.Now().Add(t.RequestTimeout).UnixNano()
	item = &pqueue.Item{Value: req, Priority: absTs}
	t.Lock()
	heap.Push(&t.requests, item)
	t.Unlock()

	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		t.Lock()
		if item.Index != -1 {
			heap.Remove(&t.requests, item.Index)
		}
		t.Unlock()
		return nil, err
	}
	resp.Body = &bodyCloseInterceptor{ReadCloser: resp.Body, item: item, t: t}

	return resp, nil
}

func (t *Transport) worker() {
	ticker := time.NewTicker(25 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
		case <-t.exitChan:
			goto exit
		}
		now := time.Now().UnixNano()
		for {
			t.Lock()
			item, _ := t.requests.PeekAndShift(now)
			t.Unlock()

			if item == nil {
				break
			}

			req := item.Value.(*http.Request)
			t.transport.CancelRequest(req)
		}
	}
exit:
	ticker.Stop()
	close(t.exitChan)
}

type bodyCloseInterceptor struct {
	io.ReadCloser
	item *pqueue.Item
	t    *Transport
}

func (bci *bodyCloseInterceptor) Close() error {
	err := bci.ReadCloser.Close()
	bci.t.Lock()
	if bci.item.Index != -1 {
		heap.Remove(&bci.t.requests, bci.item.Index)
	}
	bci.t.Unlock()
	return err
}
