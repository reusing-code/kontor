package ledger

import (
	"bufio"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"
)

const (
	fakeMailA = "Message-ID: <a@example.org>\nSubject: A\n\nbody a\n"
	fakeMailB = "Message-ID: <b@example.org>\nSubject: B\n\nbody b\n"
)

// actionIMAP serves fakeMailA and fakeMailB as uid 1 and 2 of "Queue" next
// to an empty "Done" and "Trash", and returns a client that selected "Queue".
func actionIMAP(t *testing.T, capabilities string) (*fakeIMAP, *imapClient) {
	t.Helper()
	fake := newFakeIMAP(t)
	fake.capabilities = capabilities
	fake.addFolder("Queue", fakeMailA, fakeMailB)
	fake.addFolder("Done")
	fake.addFolder("Trash")
	host, port := fake.host()
	client, err := newIMAPClient(host, port, false)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Login("user", "secret"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := client.SelectFolder("Queue"); err != nil {
		t.Fatalf("select: %v", err)
	}
	return fake, client
}

func TestIMAPSelectFolder_ReadsUIDValidity(t *testing.T) {
	fake := newFakeIMAP(t)
	fake.uidValidity = 4711
	host, port := fake.host()
	client, err := newIMAPClient(host, port, false)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()
	if err := client.SelectFolder("INBOX"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if got := client.UIDValidity(); got != 4711 {
		t.Errorf("UIDValidity = %d, want 4711", got)
	}
}

func TestIMAPMove(t *testing.T) {
	tests := []struct {
		name         string
		capabilities string
		want         []string
	}{
		{"with MOVE", "IMAP4rev1 UIDPLUS MOVE", []string{`UID MOVE 1 "Done"`}},
		{"fallback", "IMAP4rev1 UIDPLUS", []string{`UID COPY 1 "Done"`, `UID STORE 1 +FLAGS.SILENT (\Deleted)`, "UID EXPUNGE 1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, client := actionIMAP(t, tt.capabilities)
			if err := client.Move(1, "Done"); err != nil {
				t.Fatalf("Move: %v", err)
			}
			if got := fake.uids("Queue"); !slices.Equal(got, []uint32{2}) {
				t.Errorf("Queue = %v, want [2]", got)
			}
			if got := fake.uids("Done"); len(got) != 1 {
				t.Errorf("Done = %v, want one message", got)
			}
			if got := fake.sent("UID "); !slices.Equal(got, tt.want) {
				t.Errorf("commands = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIMAPMove_WithoutUIDPLUSLeavesTheMessageFlagged(t *testing.T) {
	fake, client := actionIMAP(t, "IMAP4rev1")
	if err := client.Move(1, "Done"); !errors.Is(err, errIMAPNotExpunged) {
		t.Fatalf("Move: err = %v, want errIMAPNotExpunged", err)
	}
	if got := fake.uids("Queue"); !slices.Equal(got, []uint32{1, 2}) {
		t.Errorf("Queue = %v, want [1 2]", got)
	}
	if got := fake.uids("Done"); len(got) != 1 {
		t.Errorf("Done = %v, want the copy", got)
	}
	if got := append(fake.sent("EXPUNGE"), fake.sent("UID EXPUNGE")...); len(got) != 0 {
		t.Errorf("expunge sent without UIDPLUS: %q", got)
	}
}

func TestIMAPMove_RefusedCopyKeepsTheMessage(t *testing.T) {
	for _, capabilities := range []string{"IMAP4rev1 UIDPLUS MOVE", "IMAP4rev1 UIDPLUS"} {
		t.Run(capabilities, func(t *testing.T) {
			fake, client := actionIMAP(t, capabilities)
			// the fake answers an unknown target with NO [TRYCREATE]
			if err := client.Move(1, "Missing"); !errors.Is(err, errIMAPCommandFailed) {
				t.Fatalf("Move: err = %v, want errIMAPCommandFailed", err)
			}
			if got := fake.uids("Queue"); !slices.Equal(got, []uint32{1, 2}) {
				t.Errorf("Queue = %v, want [1 2]", got)
			}
			if got := fake.sent("UID STORE"); len(got) != 0 {
				t.Errorf("message flagged after a refused copy: %q", got)
			}
		})
	}
}

func TestIMAPDelete(t *testing.T) {
	fake, client := actionIMAP(t, "IMAP4rev1 UIDPLUS")
	if err := client.Delete(2); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := fake.uids("Queue"); !slices.Equal(got, []uint32{1}) {
		t.Errorf("Queue = %v, want [1]", got)
	}
}

func TestIMAPSearchHeader(t *testing.T) {
	_, client := actionIMAP(t, "IMAP4rev1")
	got, err := client.SearchHeader("Message-ID", "<b@example.org>")
	if err != nil {
		t.Fatalf("SearchHeader: %v", err)
	}
	if !slices.Equal(got, []uint32{2}) {
		t.Errorf("uids = %v, want [2]", got)
	}
}

func TestIMAPTrashFolder(t *testing.T) {
	fake, client := actionIMAP(t, "IMAP4rev1")
	if got, err := client.TrashFolder(); err != nil || got != "" {
		t.Fatalf("without a special-use mailbox: %q, %v", got, err)
	}
	fake.listLines = []string{
		`* LIST (\HasNoChildren) "/" "INBOX"`,
		`* LIST (\HasNoChildren \Trash) "/" "[Gmail]/Papierkorb"`,
	}
	got, err := client.TrashFolder()
	if err != nil || got != "[Gmail]/Papierkorb" {
		t.Fatalf("TrashFolder = %q, %v", got, err)
	}
}

// A NO whose text contains "OK" must not pass as success: a copy taken for
// done would be followed by deleting the original.
func TestIMAPCommand_StatusIsReadFromTheTaggedLineOnly(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		_, _ = bufio.NewReader(server).ReadString('\n')
		_, _ = server.Write([]byte("A0001 NO Mailbox BOOKS does not exist\r\n"))
	}()
	c := &imapClient{conn: client, r: bufio.NewReader(client), w: bufio.NewWriter(client), caps: map[string]bool{}}
	_, err := c.command(`UID COPY 1 "BOOKS"`)
	if !errors.Is(err, errIMAPCommandFailed) || !strings.Contains(err.Error(), "NO") {
		t.Fatalf("err = %v, want errIMAPCommandFailed", err)
	}
}

func TestIMAPDelete_RefusedStoreIsNotAnIOError(t *testing.T) {
	fake, client := actionIMAP(t, "IMAP4rev1 UIDPLUS")
	fake.setRefuse("UID STORE", true)
	if err := client.Delete(1); !errors.Is(err, errIMAPCommandFailed) {
		t.Fatalf("Delete: err = %v, want errIMAPCommandFailed", err)
	}
	fake.setRefuse("UID STORE", false)
	if err := client.Delete(1); err != nil {
		t.Fatalf("Delete on the same connection: %v", err)
	}
	if got := fake.uids("Queue"); !slices.Equal(got, []uint32{2}) {
		t.Errorf("Queue = %v, want [2]", got)
	}
}
