package httpclient

import (
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"
)

func testHandler(w http.ResponseWriter, req *http.Request) {
	time.Sleep(200 * time.Millisecond)
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
