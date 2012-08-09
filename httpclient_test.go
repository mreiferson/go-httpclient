package httpclient

import (
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
)

func testHandler(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, "hello, world!\n")
}

func setupMockServer(t *testing.T) net.Addr {
	http.HandleFunc("/test", testHandler)
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
	return ln.Addr()
}

func TestHttpClient(t *testing.T) {
	addr := setupMockServer(t)
	httpClient := New(false)
	if httpClient == nil {
		t.Fatalf("failed to instantiate HttpClient")
	}

	req, _ := http.NewRequest("GET", "http://"+addr.String()+"/test", nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("1st request failed - %s", err.Error())
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("1st failed to read body - %s", err.Error())
	}
	t.Logf("%s", body)

	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatalf("2nd request failed - %s", err.Error())
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("2nd failed to read body - %s", err.Error())
	}
	t.Logf("%s", body)
}
