package ledger

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeIMAP serves fixture messages over plain TCP. uid n is the n-th fixture
// of the list. Per-uid behaviour can be changed by the tests.
type fakeIMAP struct {
	ln       net.Listener
	messages map[uint32][]byte

	mu           sync.Mutex
	rejectHeader map[uint32]bool
	dropOnHeader map[uint32]bool
}

func newFakeIMAP(t *testing.T, fixtures ...string) *fakeIMAP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeIMAP{ln: ln, messages: map[uint32][]byte{}, rejectHeader: map[uint32]bool{}, dropOnHeader: map[uint32]bool{}}
	for i, name := range fixtures {
		raw, err := os.ReadFile(fixturePath(name))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		f.messages[uint32(i+1)] = raw
	}
	t.Cleanup(func() { ln.Close() })
	go f.accept()
	return f
}

func (f *fakeIMAP) host() (string, int) {
	addr := f.ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func (f *fakeIMAP) set(m map[uint32]bool, uid uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m[uid] = true
}

func (f *fakeIMAP) flag(m map[uint32]bool, uid uint32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return m[uid]
}

func (f *fakeIMAP) accept() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.serve(conn)
	}
}

func (f *fakeIMAP) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	write := func(format string, args ...any) { _, _ = fmt.Fprintf(conn, format, args...) }
	write("* OK ready\r\n")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		tag, command, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch {
		case strings.HasPrefix(command, "LOGIN"), strings.HasPrefix(command, "SELECT"):
			write("%s OK done\r\n", tag)
		case strings.HasPrefix(command, "UID SEARCH"):
			uids := make([]string, 0, len(f.messages))
			for i := 1; i <= len(f.messages); i++ {
				uids = append(uids, strconv.Itoa(i))
			}
			write("* SEARCH %s\r\n%s OK done\r\n", strings.Join(uids, " "), tag)
		case strings.HasPrefix(command, "UID FETCH"):
			fields := strings.Fields(command)
			n, _ := strconv.ParseUint(fields[2], 10, 32)
			uid := uint32(n)
			raw, ok := f.messages[uid]
			header := strings.Contains(command, "HEADER")
			switch {
			case !ok, header && f.flag(f.rejectHeader, uid):
				write("%s NO fetch failed\r\n", tag)
			case header && f.flag(f.dropOnHeader, uid):
				return
			default:
				data := raw
				if header {
					data = append(headerBlock(raw), '\r', '\n', '\r', '\n')
				}
				write("* %d FETCH (UID %d BODY[] {%d}\r\n", uid, uid, len(data))
				_, _ = conn.Write(data)
				write(")\r\n%s OK done\r\n", tag)
			}
		case strings.HasPrefix(command, "LOGOUT"):
			write("* BYE\r\n%s OK done\r\n", tag)
			return
		default:
			write("%s BAD unknown\r\n", tag)
		}
	}
}
