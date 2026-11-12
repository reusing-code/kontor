package ledger

import (
	"bufio"
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
