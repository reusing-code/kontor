package ledger

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
	"net"
	"os"
	"slices"
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

	// capabilities is the CAPABILITY answer, uidValidity what SELECT reports.
	// refuse answers commands starting with a key (e.g. "UID MOVE") with NO.
	// deleted holds the \Deleted flags per folder ("" is the default folder),
	// commands everything the clients sent.
	capabilities string
	uidValidity  uint32
	refuse       map[string]bool
	deleted      map[string]map[uint32]bool
	commands     []string
}

func newFakeIMAP(t *testing.T, fixtures ...string) *fakeIMAP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeIMAP{ln: ln, messages: map[uint32][]byte{}, rejectHeader: map[uint32]bool{}, rejectBody: map[uint32]bool{}, dropOnHeader: map[uint32]bool{},
		capabilities: "IMAP4rev1 UIDPLUS MOVE", uidValidity: 1, refuse: map[string]bool{}, deleted: map[string]map[uint32]bool{}}
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

// box returns the messages of a folder; any folder without its own messages
// is the default one. Callers hold f.mu.
func (f *fakeIMAP) box(name string) (string, map[uint32][]byte) {
	if msgs, ok := f.folders[name]; ok {
		return name, msgs
	}
	return "", f.messages
}

// uids lists the messages of a folder ("" for the default folder).
func (f *fakeIMAP) uids(folder string) []uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, msgs := f.box(folder)
	return slices.Sorted(maps.Keys(msgs))
}

// sent returns the recorded commands that start with the prefix.
func (f *fakeIMAP) sent(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.commands {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeIMAP) setRefuse(prefix string, on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refuse[prefix] = on
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

// fakeUIDArg reads the uid of `UID <VERB> <uid> ...`.
func fakeUIDArg(command string) uint32 {
	fields := strings.Fields(command)
	if len(fields) < 3 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[2], 10, 32)
	return uint32(n)
}

func fakeLastQuoted(command string) string {
	command = strings.TrimSuffix(command, `"`)
	return command[strings.LastIndex(command, `"`)+1:]
}

func (f *fakeIMAP) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	write := func(format string, args ...any) { _, _ = fmt.Fprintf(conn, format, args...) }
	write("* OK ready\r\n")
	selected := ""
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		tag, command, _ := strings.Cut(strings.TrimSpace(line), " ")
		if f.handle(conn, write, tag, command, &selected) {
			return
		}
	}
}

// handle answers one command and reports whether the connection ends.
func (f *fakeIMAP) handle(conn net.Conn, write func(string, ...any), tag, command string, selected *string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.HasPrefix(command, "LOGIN") {
		f.commands = append(f.commands, command)
	}
	for prefix, on := range f.refuse {
		if on && strings.HasPrefix(command, prefix) {
			write("%s NO refused, not OK\r\n", tag)
			return false
		}
	}
	name, messages := f.box(*selected)
	switch {
	case strings.HasPrefix(command, "LOGIN"):
		f.logins = append(f.logins, command)
		if f.password != "" && !strings.HasSuffix(command, ` "`+f.password+`"`) {
			write("%s NO [AUTHENTICATIONFAILED] Invalid credentials\r\n", tag)
			return false
		}
		write("%s OK done\r\n", tag)
	case strings.HasPrefix(command, "CAPABILITY"):
		write("* CAPABILITY %s\r\n%s OK done\r\n", f.capabilities, tag)
	case strings.HasPrefix(command, "SELECT"):
		*selected = fakeLastQuoted(command)
		write("* OK [UIDVALIDITY %d] UIDs valid\r\n%s OK [READ-WRITE] done\r\n", f.uidValidity, tag)
	case strings.HasPrefix(command, "LIST"):
		for _, l := range f.listLines {
			write("%s\r\n", l)
		}
		write("%s OK done\r\n", tag)
	case strings.HasPrefix(command, "UID SEARCH"):
		uids := make([]string, 0, len(messages))
		for _, uid := range slices.Sorted(maps.Keys(messages)) {
			if strings.HasPrefix(command, "UID SEARCH HEADER") && !bytes.Contains(headerBlock(messages[uid]), []byte(fakeLastQuoted(command))) {
				continue
			}
			uids = append(uids, strconv.Itoa(int(uid)))
		}
		write("* SEARCH %s\r\n%s OK done\r\n", strings.Join(uids, " "), tag)
	case strings.HasPrefix(command, "UID FETCH"):
		uid := fakeUIDArg(command)
		raw, ok := messages[uid]
		header := strings.Contains(command, "HEADER")
		switch {
		case !ok, header && f.rejectHeader[uid], !header && f.rejectBody[uid]:
			write("%s NO fetch failed\r\n", tag)
		case header && f.dropOnHeader[uid]:
			return true
		default:
			data := raw
			if header {
				data = append(headerBlock(raw), '\r', '\n', '\r', '\n')
			}
			write("* %d FETCH (UID %d BODY[] {%d}\r\n", uid, uid, len(data))
			_, _ = conn.Write(data)
			write(")\r\n%s OK done\r\n", tag)
		}
	case strings.HasPrefix(command, "UID MOVE"), strings.HasPrefix(command, "UID COPY"):
		uid := fakeUIDArg(command)
		target, ok := f.folders[fakeLastQuoted(command)]
		if !ok {
			write("%s NO [TRYCREATE] no such mailbox\r\n", tag)
			return false
		}
		if raw, ok := messages[uid]; ok {
			next := uint32(1)
			for existing := range target {
				next = max(next, existing+1)
			}
			target[next] = raw
			if strings.HasPrefix(command, "UID MOVE") {
				delete(messages, uid)
			}
		}
		write("%s OK done\r\n", tag)
	case strings.HasPrefix(command, "UID STORE"):
		if f.deleted[name] == nil {
			f.deleted[name] = map[uint32]bool{}
		}
		f.deleted[name][fakeUIDArg(command)] = strings.Contains(command, `+FLAGS.SILENT (\Deleted)`)
		write("%s OK done\r\n", tag)
	case strings.HasPrefix(command, "UID EXPUNGE"):
		if uid := fakeUIDArg(command); f.deleted[name][uid] {
			delete(messages, uid)
			delete(f.deleted[name], uid)
		}
		write("%s OK done\r\n", tag)
	case strings.HasPrefix(command, "EXPUNGE"):
		for uid := range f.deleted[name] {
			delete(messages, uid)
		}
		delete(f.deleted, name)
		write("%s OK done\r\n", tag)
	case strings.HasPrefix(command, "LOGOUT"):
		write("* BYE\r\n%s OK done\r\n", tag)
		return true
	default:
		write("%s BAD unknown\r\n", tag)
	}
	return false
}
