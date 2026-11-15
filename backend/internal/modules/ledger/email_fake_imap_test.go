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

	// listLines are the raw untagged LIST responses; password, when set, is
	// the only one LOGIN accepts. logins records the LOGIN commands.
	listLines []string
	password  string
	logins    []string

	// folders holds messages of named folders; SELECT of any other folder
	// serves messages. rejectBody refuses the full fetch of a uid.
	folders    map[string]map[uint32][]byte
	rejectBody map[uint32]bool

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
	f := &fakeIMAP{ln: ln, messages: map[uint32][]byte{}, rejectHeader: map[uint32]bool{}, rejectBody: map[uint32]bool{}, dropOnHeader: map[uint32]bool{}}
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

// addFolder serves the raw messages as uid 1..n of the named folder.
func (f *fakeIMAP) addFolder(name string, raws ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.folders == nil {
		f.folders = map[string]map[uint32][]byte{}
	}
	msgs := map[uint32][]byte{}
	for i, raw := range raws {
		msgs[uint32(i+1)] = []byte(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\n", "\r\n"))
	}
	f.folders[name] = msgs
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
	messages := f.messages
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		tag, command, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch {
		case strings.HasPrefix(command, "LOGIN"):
			f.mu.Lock()
			f.logins = append(f.logins, command)
			f.mu.Unlock()
			if f.password != "" && !strings.HasSuffix(command, ` "`+f.password+`"`) {
				write("%s NO [AUTHENTICATIONFAILED] Invalid credentials\r\n", tag)
				continue
			}
			write("%s OK done\r\n", tag)
		case strings.HasPrefix(command, "SELECT"):
			messages = f.messages
			f.mu.Lock()
			if selected, ok := f.folders[strings.Trim(strings.TrimPrefix(command, "SELECT "), `"`)]; ok {
				messages = selected
			}
			f.mu.Unlock()
			write("%s OK done\r\n", tag)
		case strings.HasPrefix(command, "LIST"):
			for _, l := range f.listLines {
				write("%s\r\n", l)
			}
			write("%s OK done\r\n", tag)
		case strings.HasPrefix(command, "UID SEARCH"):
			uids := make([]string, 0, len(messages))
			for i := 1; i <= len(messages); i++ {
				uids = append(uids, strconv.Itoa(i))
			}
			write("* SEARCH %s\r\n%s OK done\r\n", strings.Join(uids, " "), tag)
		case strings.HasPrefix(command, "UID FETCH"):
			fields := strings.Fields(command)
			n, _ := strconv.ParseUint(fields[2], 10, 32)
			uid := uint32(n)
			raw, ok := messages[uid]
			header := strings.Contains(command, "HEADER")
			switch {
			case !ok, header && f.flag(f.rejectHeader, uid), !header && f.flag(f.rejectBody, uid):
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
