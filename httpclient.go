package httpclient

import (
	"bufio"
	"container/list"
	"crypto/tls"
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
	return "0.3.9"
}

type cachedConn struct {
	net.Conn
	shouldClose bool
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
type HttpClient struct {
	sync.RWMutex
	client           *http.Client
	cachedConns      map[string]*connCache
	connMap          map[*http.Request]*cachedConn
	ConnectTimeout   time.Duration
	ReadWriteTimeout time.Duration
	MaxConnsPerHost  int
	RedirectPolicy   func(*http.Request, []*http.Request) error
	TLSClientConfig  *tls.Config
	Verbose          bool
}

// create a new HttpClient
// all options should be set on the instance returned
func New() *HttpClient {
	client := &http.Client{}
	h := &HttpClient{
		client:           client,
		cachedConns:      make(map[string]*connCache),
		connMap:          make(map[*http.Request]*cachedConn),
		ConnectTimeout:   5 * time.Second,
		ReadWriteTimeout: 5 * time.Second,
		MaxConnsPerHost:  5,
		RedirectPolicy:   DefaultRedirectPolicy,
		TLSClientConfig:  &tls.Config{},
	}

	redirFunc := func(r *http.Request, v []*http.Request) error {
		lastRequest := v[len(v)-1]
		if strings.HasPrefix(lastRequest.URL.Scheme, "hc_") {
			lastRequest.URL.Scheme = lastRequest.URL.Scheme[3:]
		}
		if strings.HasPrefix(r.URL.Scheme, "hc_") {
			r.URL.Scheme = r.URL.Scheme[3:]
		}
		resp := h.RedirectPolicy(r, v)
		r.URL.Scheme = "hc_" + r.URL.Scheme
		return resp
	}

	transport := &http.Transport{
		TLSClientConfig: h.TLSClientConfig,
	}
	transport.RegisterProtocol("hc_http", h)
	transport.RegisterProtocol("hc_https", h)

	client.CheckRedirect = redirFunc
	client.Transport = transport

	return h
}

func DefaultRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) > 3 {
		return errors.New("Stopped after 3 redirects")
	}
	return nil
}

// satisfies the RoundTripper interface and handles checking
// the connection cache or dialing (with ConnectTimeout)
func (h *HttpClient) RoundTrip(req *http.Request) (*http.Response, error) {
	var c net.Conn
	var err error

	addr := canonicalAddr(req.URL.Host, req.URL.Scheme)

	if h.Verbose {
		log.Printf("DEBUG: checking cache for addr %s", addr)
	}
	c, err = h.checkConnCache(addr)
	if err != nil {
		return nil, err
	}

	if c == nil {
		if h.Verbose {
			log.Printf("DEBUG: addr not in cache, connecting...")
		}
		c, err = net.DialTimeout("tcp", addr, h.ConnectTimeout)
		if err != nil {
			return nil, err
		}

		if req.URL.Scheme == "hc_https" {
			// Initiate TLS and check remote host name against certificate.
			c = tls.Client(c, h.TLSClientConfig)
			if err = c.(*tls.Conn).Handshake(); err != nil {
				return nil, err
			}
			if h.TLSClientConfig == nil || !h.TLSClientConfig.InsecureSkipVerify {
				hostname, _, _ := net.SplitHostPort(req.URL.Host) // Remove port from host
				if err = c.(*tls.Conn).VerifyHostname(hostname); err != nil {
					return nil, err
				}
			}
		}
	}

	h.Lock()
	h.connMap[req] = &cachedConn{Conn: c}
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

	resp, err := http.ReadResponse(br, req)
	if err != nil {
		h.Lock()
		delete(h.connMap, req)
		h.Unlock()
		conn.Close()
	}
	return resp, err
}

// returns the connection associated with the specified request
// cannot be called after FinishRequest
func (h *HttpClient) GetConn(req *http.Request) (net.Conn, error) {
	h.RLock()
	defer h.RUnlock()

	conn, ok := h.connMap[req]
	if !ok {
		return nil, errors.New("connection not in map")
	}

	return conn, nil
}

// perform the specified request
func (h *HttpClient) Do(req *http.Request) (*http.Response, error) {
	// h@x0r Go's http client to use our RoundTripper
	if !strings.HasPrefix(req.URL.Scheme, "hc_") {
		req.URL.Scheme = "hc_" + req.URL.Scheme
	}

	resp, err := h.client.Do(req)
	if err == nil && (resp.Close || req.Close) {
		conn, _ := h.GetConn(req)
		conn.(*cachedConn).shouldClose = true
		if h.Verbose {
			log.Printf("DEBUG: setting close on %s, err: %s, resp.Close: %v, req.Close: %v", conn.RemoteAddr(), err, resp.Close, req.Close)
		}
	}
	if resp != nil {
		if strings.HasPrefix(resp.Request.URL.Scheme, "hc_") {
			resp.Request.URL.Scheme = resp.Request.URL.Scheme[3:]
		}
	}
	return resp, err
}

// perform final cleanup for the specified request
// *must* be called for every request performed after processing
// is finished and after which GetConn will no longer return
// successfully
func (h *HttpClient) FinishRequest(req *http.Request) error {
	conn, err := h.GetConn(req)
	if err != nil {
		return err
	}

	h.Lock()
	delete(h.connMap, req)
	h.Unlock()

	if conn.(*cachedConn).shouldClose {
		if h.Verbose {
			log.Printf("DEBUG: conn %s shouldClose, closing...", conn.RemoteAddr())
		}
		conn.Close()
		return nil
	}

	addr := canonicalAddr(req.URL.Host, req.URL.Scheme)
	if h.Verbose {
		log.Printf("DEBUG: caching conn %s as %s", conn.RemoteAddr(), addr)
	}
	return h.cacheConn(addr, conn.(*cachedConn).Conn)
}

func canonicalAddr(s string, scheme string) string {
	if !hasPort(s) {
		switch scheme {
		case "http", "hc_http":
			s = s + ":80"
		case "https", "hc_https":
			s = s + ":443"
		}
	}
	return s
}

// Given a string of the form "host", "host:port", or "[ipv6::address]:port",
// return true if the string includes a port.
func hasPort(s string) bool { return strings.LastIndex(s, ":") > strings.LastIndex(s, "]") }
