package server

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHTTPServer_SetsTimeouts(t *testing.T) {
	srv := newHTTPServer(8080, http.NotFoundHandler())
	if srv.Addr != ":8080" || srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Errorf("server = %+v", srv)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want none (long syncs and streaming)", srv.WriteTimeout)
	}
}

func TestNewHTTPServer_DropsSlowHeaders(t *testing.T) {
	srv := newHTTPServer(0, http.NotFoundHandler())
	srv.ReadHeaderTimeout = 50 * time.Millisecond
	ts := httptest.NewUnstartedServer(srv.Handler)
	ts.Config = srv
	ts.Start()
	defer ts.Close()

	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	// an answer (408) or a closed connection is fine, a hang is not
	_, err = bufio.NewReader(conn).ReadString('\n')
	if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
		t.Fatal("server kept the connection open past ReadHeaderTimeout")
	}
}
