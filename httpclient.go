package httpclient

import (
	"container/list"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"
)

type connCache struct {
	dl          *list.List
	outstanding int
}

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
	client              *http.Client
	cachedConns         map[string]*connCache
	Conn                net.Conn
	ConnectTimeout      time.Duration
	ReadWriteTimeout    time.Duration
	MaxRedirects        int
	MaxIdleConnsPerHost int
}

func New(skipInvalidSSL bool) *HttpClient {
	client := &http.Client{}
	h := &HttpClient{
		client:      client,
		cachedConns: make(map[string]*connCache),
	}

	dialFunc := func(netw, addr string) (net.Conn, error) {
		return h.dial(netw, addr)
	}
	redirFunc := func(r *http.Request, v []*http.Request) error {
		return h.redirectPolicy(r, v)
	}

	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: skipInvalidSSL},
		Dial:                dialFunc,
		MaxIdleConnsPerHost: -1, // disable Go's built in connection caching
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
	var c net.Conn
	var err error

	cc, ok := h.cachedConns[addr]
	if ok {
		e := cc.dl.Front()
		if e != nil {
			cc.dl.Remove(e)
			c = e.Value.(net.Conn)
		}
	}

	if c == nil && (cc == nil || cc.outstanding < h.MaxIdleConnsPerHost) {
		deadline := time.Now().Add(h.ReadWriteTimeout + h.ConnectTimeout)
		c, err = net.DialTimeout(netw, addr, h.ConnectTimeout)
		if err != nil {
			return nil, err
		}
		c.SetDeadline(deadline)

		if cc == nil {
			cc = &connCache{
				dl: list.New(),
			}
			h.cachedConns[addr] = cc
		}
		cc.dl.PushBack(c)
		cc.outstanding++
	}

	h.Conn = c

	return c, nil
}

func (h *HttpClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := h.client.Do(req)
	return resp, err
}
