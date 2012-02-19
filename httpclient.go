package httpclient

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"
)

// HttpClient wraps Go's built in HTTP client providing an API to:
//    * set connect timeout
//    * set read/write timeout
//    * skip invalid SSL certificates
//    * return the connection object from Do()
//
// NOTE: this client is not goroutine safe (this is a result of the 
// inability of the built in API to provide access to the connection 
// and the resulting hack to work around that)
type HttpClient struct {
	client           *http.Client
	conn             net.Conn
	ConnectTimeout   time.Duration
	ReadWriteTimeout time.Duration
	MaxRedirects     int
}

func New(skipInvalidSSL bool) *HttpClient {
	client := &http.Client{}
	h := &HttpClient{client: client}

	dialFunc := func(netw, addr string) (net.Conn, error) {
		return h.dial(netw, addr)
	}
	redirFunc := func(r *http.Request, v []*http.Request) error {
		return h.redirectPolicy(r, v)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipInvalidSSL},
		Dial:            dialFunc,
	}

	client.CheckRedirect = redirFunc
	client.Transport = transport

	return h
}

func (h *HttpClient) redirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= h.MaxRedirects {
		return errors.New("stopped after 3 redirects")
	}
	return nil
}

func (h *HttpClient) dial(netw, addr string) (net.Conn, error) {
	deadline := time.Now().Add(h.ReadWriteTimeout + h.ConnectTimeout)
	c, err := net.DialTimeout(netw, addr, h.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	c.SetDeadline(deadline)
	h.conn = c
	return c, nil
}

func (h *HttpClient) Do(req *http.Request) (*http.Response, net.Conn, error) {
	resp, err := h.client.Do(req)
	return resp, h.conn, err
}
