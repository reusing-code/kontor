package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/config"
)

// slowBackground is an app whose one background job needs a moment after
// its context ends, like a scan writing its result.
func slowBackground(finished *atomic.Bool) *app {
	a := &app{handler: http.NotFoundHandler(), background: &sync.WaitGroup{}}
	a.start = func(ctx context.Context) {
		a.background.Go(func() {
			<-ctx.Done()
			time.Sleep(50 * time.Millisecond)
			finished.Store(true)
		})
	}
	return a
}

func testServer() *Server {
	return New(config.Config{}, slog.New(slog.DiscardHandler), nil)
}

func TestServe_WaitsForBackgroundWorkOnSignal(t *testing.T) {
	var finished atomic.Bool
	a := slowBackground(&finished)
	srv := newHTTPServer(0, a.handler)
	srv.Addr = "127.0.0.1:0"
	quit := make(chan os.Signal, 1)
	quit <- syscall.SIGTERM

	if err := testServer().serve(a, srv, quit); err != nil {
		t.Fatalf("serve = %v", err)
	}
	if !finished.Load() {
		t.Error("serve returned before the background work finished")
	}
}

func TestServe_ServeErrorStillStopsAndWaits(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()

	var finished atomic.Bool
	a := slowBackground(&finished)
	srv := newHTTPServer(0, a.handler)
	srv.Addr = taken.Addr().String()

	if err := testServer().serve(a, srv, make(chan os.Signal)); err == nil {
		t.Fatal("serve on a taken port returned no error")
	}
	if !finished.Load() {
		t.Error("serve returned before the background work finished")
	}
}

func TestWaitTimeout(t *testing.T) {
	var wg sync.WaitGroup
	release := make(chan struct{})
	wg.Go(func() { <-release })
	if waitTimeout(&wg, 20*time.Millisecond) {
		t.Error("waitTimeout reported a running job as done")
	}
	close(release)
	if !waitTimeout(&wg, time.Second) {
		t.Error("waitTimeout did not see the job finish")
	}
}

func TestBackgroundServicesStopWithTheirContext(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) {
		c.BackupDir = t.TempDir()
		c.BackupInterval = time.Hour
		c.BackupKeep = 1
		c.LedgerEmailScanInterval = time.Hour
		c.PaperlessPipelineInterval = time.Hour
	})
	ctx, cancel := context.WithCancel(t.Context())
	a.start(ctx)
	time.Sleep(20 * time.Millisecond)
	cancel()
	if !waitTimeout(a.background, 5*time.Second) {
		t.Fatal("background services did not stop")
	}
}

func TestWelcomeMailCountsAsBackgroundWork(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			accepted <- c
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	a := newTestApp(t, func(c *config.Config) {
		c.SMTPHost, c.SMTPPort, c.SMTPProtocol, c.SMTPFrom = "127.0.0.1", port, "none", "kontor@example.com"
	})

	a.register(t, "welcome@example.com")
	conn := <-accepted
	if waitTimeout(a.background, 50*time.Millisecond) {
		t.Fatal("the welcome mail is not counted as background work")
	}
	conn.Close()
	if !waitTimeout(a.background, 5*time.Second) {
		t.Fatal("the welcome mail did not finish after the server hung up")
	}
}
