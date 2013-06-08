package httpclient

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

var starter sync.Once
var addr net.Addr

func testHandler(w http.ResponseWriter, req *http.Request) {
	time.Sleep(200 * time.Millisecond)
	io.WriteString(w, "hello, world!\n")
}

func postHandler(w http.ResponseWriter, req *http.Request) {
	ioutil.ReadAll(req.Body)
	w.Header().Set("Content-Length", "2")
	io.WriteString(w, "OK")
}

func closeHandler(w http.ResponseWriter, req *http.Request) {
	hj, _ := w.(http.Hijacker)
	conn, bufrw, _ := hj.Hijack()
	defer conn.Close()
	bufrw.WriteString("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n")
	bufrw.Flush()
}

func redirectHandler(w http.ResponseWriter, req *http.Request) {
	ioutil.ReadAll(req.Body)
	http.Redirect(w, req, "/post", 302)
}

func redirect2Handler(w http.ResponseWriter, req *http.Request) {
	ioutil.ReadAll(req.Body)
	http.Redirect(w, req, "/redirect", 302)
}

func setupMockServer(t *testing.T) {
	http.HandleFunc("/test", testHandler)
	http.HandleFunc("/post", postHandler)
	http.HandleFunc("/redirect", redirectHandler)
	http.HandleFunc("/redirect2", redirect2Handler)
	http.HandleFunc("/close", closeHandler)
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen - %s", err.Error())
	}
	go func() {
		err = http.Serve(ln, nil)
		if err != nil {
			t.Fatalf("failed to start HTTP server - %s", err.Error())
		}
	}()
	addr = ln.Addr()
}

func TestHttpsConnection(t *testing.T) {
	transport := &Transport{
		ConnectTimeout: 1 * time.Second,
		RequestTimeout: 2 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	defer transport.Close()
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", "https://httpbin.org/ip", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("1st request failed - %s", err.Error())
	}
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("1st failed to read body - %s", err.Error())
	}
	resp.Body.Close()

	req2, _ := http.NewRequest("GET", "https://httpbin.org/delay/5", nil)
	_, err = client.Do(req2)
	if err == nil {
		t.Fatalf("HTTPS request should have timed out")
	}
}

func TestHttpClient(t *testing.T) {
	starter.Do(func() { setupMockServer(t) })

	transport := &Transport{
		ConnectTimeout: 1 * time.Second,
		RequestTimeout: 5 * time.Second,
	}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", "http://"+addr.String()+"/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("1st request failed - %s", err.Error())
	}
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("1st failed to read body - %s", err.Error())
	}
	resp.Body.Close()
	transport.Close()

	transport = &Transport{
		ConnectTimeout: 25 * time.Millisecond,
		RequestTimeout: 50 * time.Millisecond,
	}
	client = &http.Client{Transport: transport}

	req2, _ := http.NewRequest("GET", "http://"+addr.String()+"/test", nil)
	resp, err = client.Do(req2)
	if err == nil {
		t.Fatalf("2nd request should have timed out")
	}
	transport.Close()

	transport = &Transport{
		ConnectTimeout: 25 * time.Millisecond,
		RequestTimeout: 250 * time.Millisecond,
	}
	client = &http.Client{Transport: transport}

	req3, _ := http.NewRequest("GET", "http://"+addr.String()+"/test", nil)
	resp, err = client.Do(req3)
	if err != nil {
		t.Fatalf("3nd request should not have timed out")
	}
	resp.Body.Close()
	transport.Close()
}
