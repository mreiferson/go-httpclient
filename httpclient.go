package httpclient

import (
	"bufio"
	"container/list"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// returns the current version
func Version() string {
	return "0.3"
}

type connCache struct {
	dl          *list.List
	outstanding int
}

// HttpClient wraps Go's built in HTTP client providing an API to:
//    * set connect timeout
//    * set read/write timeout
//    * easy access to the connection object for a given request
//
// TODO: https support
type HttpClient struct {
	sync.RWMutex
	client           *http.Client
	cachedConns      map[string]*connCache
	connMap          map[*http.Request]net.Conn
	ConnectTimeout   time.Duration
	ReadWriteTimeout time.Duration
	MaxRedirects     int
	MaxConnsPerHost  int
}

func New() *HttpClient {
	client := &http.Client{}
	h := &HttpClient{
		client:           client,
		cachedConns:      make(map[string]*connCache),
		connMap:          make(map[*http.Request]net.Conn),
		ConnectTimeout:   5 * time.Second,
		ReadWriteTimeout: 5 * time.Second,
		MaxConnsPerHost:  5,
	}

	redirFunc := func(r *http.Request, v []*http.Request) error {
		return h.redirectPolicy(r, v)
	}

	transport := &http.Transport{}
	transport.RegisterProtocol("hc_http", h)

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

func (h *HttpClient) RoundTrip(req *http.Request) (*http.Response, error) {
	var c net.Conn
	var err error

	addr := canonicalAddr(req.URL.Host)
	c, err = h.checkConnCache(addr)
	if err != nil {
		return nil, err
	}

	if c == nil {
		c, err = net.DialTimeout("tcp", addr, h.ConnectTimeout)
		if err != nil {
			return nil, err
		}
	}

	h.Lock()
	h.connMap[req] = c
	h.Unlock()

	return h.exec(c, req)
}

func (h *HttpClient) checkConnCache(addr string) (net.Conn, error) {
	var c net.Conn

	h.Lock()
	defer h.Unlock()

	cc, ok := h.cachedConns[addr]
	if ok {
		// address is in map, check the connection list
		e := cc.dl.Front()
		if e != nil {
			cc.dl.Remove(e)
			c = e.Value.(net.Conn)
		}
	} else {

		// this client hasnt seen this address before
		cc = &connCache{
			dl: list.New(),
		}
		h.cachedConns[addr] = cc
	}

	// TODO: implement accounting for outstanding connections
	if cc.outstanding > h.MaxConnsPerHost {
		return nil, errors.New("too many outstanding conns on this addr")
	}

	return c, nil
}

func (h *HttpClient) cacheConn(addr string, conn net.Conn) error {
	h.Lock()
	defer h.Unlock()

	cc, ok := h.cachedConns[addr]
	if !ok {
		return errors.New("addr %s not in cache map")
	}
	cc.dl.PushBack(conn)

	return nil
}

func (h *HttpClient) exec(conn net.Conn, req *http.Request) (*http.Response, error) {
	deadline := time.Now().Add(h.ReadWriteTimeout)
	conn.SetDeadline(deadline)

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	err := req.Write(bw)
	if err != nil {
		return nil, err
	}
	bw.Flush()

	return http.ReadResponse(br, req)
}

func (h *HttpClient) GetConn(req *http.Request) (net.Conn, error) {
	h.RLock()
	defer h.RUnlock()

	conn, ok := h.connMap[req]
	if !ok {
		return nil, errors.New("connection not in map")
	}

	return conn, nil
}

func (h *HttpClient) Do(req *http.Request) (*http.Response, error) {
	// h@x0r Go's http client to use our RoundTripper
	req.URL.Scheme = "hc_http"

	resp, err := h.client.Do(req)
	if err != nil {
		conn, _ := h.GetConn(req)
		if conn == nil {
			log.Panicf("PANIC: could not find connection for failed request")
		}

		conn.Close()

		h.Lock()
		delete(h.connMap, req)
		h.Unlock()
	}

	return resp, err
}

func (h *HttpClient) FinishRequest(req *http.Request) error {
	conn, err := h.GetConn(req)
	if err != nil {
		return err
	}

	h.Lock()
	delete(h.connMap, req)
	h.Unlock()

	return h.cacheConn(canonicalAddr(req.URL.Host), conn)
}

func canonicalAddr(s string) string {
	if !hasPort(s) {
		s = s + ":80"
	}
	return s
}

// Given a string of the form "host", "host:port", or "[ipv6::address]:port",
// return true if the string includes a port.
func hasPort(s string) bool { return strings.LastIndex(s, ":") > strings.LastIndex(s, "]") }
