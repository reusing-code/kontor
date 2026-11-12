package ledger

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestIMAPMailboxName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"INBOX", "INBOX"},
		{"Orders", "Orders"},
		{"Rechnungen & Belege", "Rechnungen &- Belege"},
		{"Bestellungen/Prüfung", "Bestellungen/Pr&APw-fung"},
		{"日本語", "&ZeVnLIqe-"},
		{"[Gmail]/Alle Nachrichten", "[Gmail]/Alle Nachrichten"},
		{"Ä&Ö", "&AMQ-&-&ANY-"},
		{"😀", "&2D3eAA-"},
	}
	for _, tt := range tests {
		if got := imapMailboxName(tt.in); got != tt.want {
			t.Errorf("imapMailboxName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIMAPSelectFolder(t *testing.T) {
	tests := []struct {
		folder  string
		command string
	}{
		{"Rechnungen & Belege", `A0001 SELECT "Rechnungen &- Belege"`},
		{`We"ird\Name`, `A0001 SELECT "We\"ird\\Name"`},
		{"INBOX", `A0001 SELECT "INBOX"`},
	}
	for _, tt := range tests {
		t.Run(tt.folder, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			got := make(chan string, 1)
			go func() {
				r := bufio.NewReader(server)
				line, err := r.ReadString('\n')
				if err != nil {
					got <- "read error: " + err.Error()
					return
				}
				got <- line
				_, _ = server.Write([]byte("* 3 EXISTS\r\nA0001 OK [READ-WRITE] SELECT completed\r\n"))
			}()

			c := &imapClient{conn: client, r: bufio.NewReader(client), w: bufio.NewWriter(client)}
			if err := c.SelectFolder(tt.folder); err != nil {
				t.Fatalf("SelectFolder: %v", err)
			}
			if line := <-got; line != tt.command+"\r\n" {
				t.Errorf("command = %q, want %q", line, tt.command+"\r\n")
			}
		})
	}
}

func TestIMAPSelectFolder_ServerRejects(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		_, _ = bufio.NewReader(server).ReadString('\n')
		_, _ = server.Write([]byte("A0001 NO [NONEXISTENT] Unknown Mailbox\r\n"))
	}()
	c := &imapClient{conn: client, r: bufio.NewReader(client), w: bufio.NewWriter(client)}
	if err := c.SelectFolder("Missing"); err == nil {
		t.Fatal("SelectFolder succeeded, want error")
	}
}

func TestIMAPFetchHeader(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	header := "Message-ID: <a@b>\r\nSubject: Hi\r\n\r\n"
	got := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(server).ReadString('\n')
		got <- line
		_, _ = fmt.Fprintf(server, "* 1 FETCH (UID 7 BODY[HEADER] {%d}\r\n%s)\r\nA0001 OK FETCH completed\r\n", len(header), header)
	}()

	c := &imapClient{conn: client, r: bufio.NewReader(client), w: bufio.NewWriter(client)}
	data, err := c.FetchHeader(7)
	if err != nil {
		t.Fatalf("FetchHeader: %v", err)
	}
	if string(data) != header {
		t.Errorf("data = %q, want %q", data, header)
	}
	if line := <-got; line != "A0001 UID FETCH 7 (BODY.PEEK[HEADER])\r\n" {
		t.Errorf("command = %q", line)
	}
}

func TestIMAPFetch_OversizedLiteralIsSkippedAndConnectionStaysUsable(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	size := maxIMAPLiteralBytes + 1
	go func() {
		r := bufio.NewReader(server)
		_, _ = r.ReadString('\n')
		_, _ = fmt.Fprintf(server, "* 1 FETCH (UID 7 BODY[] {%d}\r\n", size)
		chunk := make([]byte, 1<<20)
		for sent := 0; sent < size; {
			n := min(len(chunk), size-sent)
			if _, err := server.Write(chunk[:n]); err != nil {
				return
			}
			sent += n
		}
		_, _ = server.Write([]byte(")\r\nA0001 OK FETCH completed\r\n"))
		_, _ = r.ReadString('\n')
		_, _ = server.Write([]byte("* 1 FETCH (UID 8 BODY[] {2}\r\nhi)\r\nA0002 OK FETCH completed\r\n"))
	}()

	c := &imapClient{conn: client, r: bufio.NewReader(client), w: bufio.NewWriter(client)}
	if _, err := c.FetchMessage(7); !errors.Is(err, errMessageTooLarge) {
		t.Fatalf("oversized: err = %v, want errMessageTooLarge", err)
	}
	data, err := c.FetchMessage(8)
	if err != nil || string(data) != "hi" {
		t.Fatalf("next fetch: %q, %v", data, err)
	}
}

func TestIMAPFetch_RejectedCommandIsDistinguishedFromIOError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		_, _ = bufio.NewReader(server).ReadString('\n')
		_, _ = server.Write([]byte("A0001 NO [NONEXISTENT] gone\r\n"))
	}()
	c := &imapClient{conn: client, r: bufio.NewReader(client), w: bufio.NewWriter(client)}
	if _, err := c.FetchMessage(1); !errors.Is(err, errIMAPRejected) {
		t.Fatalf("err = %v, want errIMAPRejected", err)
	}

	client2, server2 := net.Pipe()
	defer client2.Close()
	go func() {
		_, _ = bufio.NewReader(server2).ReadString('\n')
		_ = server2.Close()
	}()
	c2 := &imapClient{conn: client2, r: bufio.NewReader(client2), w: bufio.NewWriter(client2)}
	if _, err := c2.FetchMessage(1); err == nil || errors.Is(err, errIMAPRejected) {
		t.Fatalf("err = %v, want an I/O error", err)
	}
}
