package httpclient

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
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
	httpClient := New()
	httpClient.TLSClientConfig.InsecureSkipVerify = true

	req, _ := http.NewRequest("GET", "https://httpbin.org/ip", nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("1st request failed - %s", err.Error())
	}
	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("1st failed to read body - %s", err.Error())
	}
	httpClient.FinishRequest(req)

	httpClient.ReadWriteTimeout = 20 * time.Millisecond
	req2, _ := http.NewRequest("GET", "https://httpbin.org/delay/5", nil)

	_, err = httpClient.Do(req)
	if err == nil {
		t.Fatalf("HTTPS request should have timed out")
	}
	httpClient.FinishRequest(req2)
}

func TestCustomRedirectPolicy(t *testing.T) {
	starter.Do(func() { setupMockServer(t) })

	httpClient := New()
	redirects := make(chan string, 3)
	httpClient.RedirectPolicy = func(r *http.Request, v []*http.Request) error {
		if strings.HasPrefix(r.URL.Scheme, "hc_") {
			t.Errorf("Stray hc_ in URL")
		}
		for _, i := range v {
			if strings.HasPrefix(i.URL.Scheme, "hc_") {
				t.Errorf("Stray hc_ in URL")
			}
		}
		redirects <- v[len(v)-1].URL.String()
		return DefaultRedirectPolicy(r, v)
	}

	req, _ := http.NewRequest("GET", "http://"+addr.String()+"/redirect2", nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("1st request failed - %s", err.Error())
	}

	urls := make([]string, 0, 3)
	close(redirects)
	for url := range redirects {
		urls = append(urls, url)
	}
	urls = append(urls, resp.Request.URL.String())
	t.Logf("%s", urls)
	for _, url := range urls {
		if strings.HasPrefix(url, "hc_") {
			t.Errorf("Stray hc_ in URL")
		}
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("1st failed to read body - %s", err.Error())
	}
	httpClient.FinishRequest(req)

	if len(urls) != 3 {
		t.Fatalf("Did not correctly redirect with custom redirect policy", err.Error())
	}

	t.Logf("%s", body)
}

func TestClose(t *testing.T) {
	starter.Do(func() { setupMockServer(t) })

	httpClient := New()
	req, _ := http.NewRequest("GET", "http://"+addr.String()+"/close", nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("1st request failed - %s", err.Error())
	}
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("1st failed to read body - %s", err.Error())
	}
	resp.Body.Close()
	httpClient.FinishRequest(req)

	req, _ = http.NewRequest("GET", "http://"+addr.String()+"/close", nil)

	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatalf("2nd request failed - %s", err.Error())
	}
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("2nd failed to read body - %s", err.Error())
	}
	resp.Body.Close()
	httpClient.FinishRequest(req)
}

func TestHttpClient(t *testing.T) {
	starter.Do(func() { setupMockServer(t) })

	httpClient := New()
	if httpClient == nil {
		t.Fatalf("failed to instantiate HttpClient")
	}

	req, _ := http.NewRequest("GET", "http://"+addr.String()+"/test", nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("1st request failed - %s", err.Error())
	}
	defer resp.Body.Close()

	if strings.HasPrefix(resp.Request.URL.Scheme, "hc_") {
		t.Errorf("Stray hc_ in response")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("1st failed to read body - %s", err.Error())
	}
	t.Logf("%s", body)
	httpClient.FinishRequest(req)

	httpClient.ReadWriteTimeout = 50 * time.Millisecond
	resp, err = httpClient.Do(req)
	if err == nil {
		t.Fatalf("2nd request should have timed out")
	}
	httpClient.FinishRequest(req)

	httpClient.ReadWriteTimeout = 250 * time.Millisecond
	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatalf("3nd request should not have timed out")
	}
	httpClient.FinishRequest(req)
}

func TestManyPosts(t *testing.T) {
	starter.Do(func() { setupMockServer(t) })

	httpClient := New()
	if httpClient == nil {
		t.Fatalf("failed to instantiate HttpClient")
	}

	data := ""
	for i := 0; i < 100; i++ {
		data = data + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}
	data = data + "\n"

	for i := 0; i < 10000; i++ {
		buffer := bytes.NewBuffer([]byte(data))
		req, _ := http.NewRequest("POST", "http://"+addr.String()+"/post", buffer)

		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("%d post request failed - %s", i, err.Error())
		}
		_, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("%d failed to read body - %s", i, err.Error())
		}
		resp.Body.Close()
		httpClient.FinishRequest(req)
	}
}
