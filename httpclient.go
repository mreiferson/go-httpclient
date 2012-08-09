package httpclient

import (
	"container/list"
	"crypto/tls"
	"errors"
	"log"
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
		client:           client,
		cachedConns:      make(map[string]*connCache),
		ConnectTimeout:   5 * time.Second,
		ReadWriteTimeout: 5 * time.Second,
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

	log.Printf("checking cache")
	cc, ok := h.cachedConns[addr]
	if ok {
		log.Printf("addr in cache")
		e := cc.dl.Front()
		if e != nil {
			log.Printf("returning conn")
			cc.outstanding--
			cc.dl.Remove(e)
			c = e.Value.(net.Conn)
		}
	}

	if c == nil && (cc == nil || cc.outstanding < h.MaxIdleConnsPerHost) {
		log.Printf("new connection")
		deadline := time.Now().Add(h.ReadWriteTimeout + h.ConnectTimeout)
		c, err = net.DialTimeout(netw, addr, h.ConnectTimeout)
		if err != nil {
			return nil, err
		}
		c.SetDeadline(deadline)
	}

	log.Printf("returning %v", c)
	h.Conn = c

	return c, nil
}

func (h *HttpClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := h.client.Do(req)
	if err == nil {
		log.Printf("request succeeded... caching conn")
		addr := req.URL.Host
		cc, ok := h.cachedConns[addr]
		if !ok {
			log.Printf("addr not in cache")
			cc = &connCache{
				dl: list.New(),
			}
			h.cachedConns[addr] = cc
		}
		log.Printf("adding conn to cache")
		cc.dl.PushBack(h.Conn)
		cc.outstanding++
	} else {
		h.Conn.Close()
	}
	return resp, err
}
