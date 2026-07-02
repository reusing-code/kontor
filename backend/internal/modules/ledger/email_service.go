package ledger

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/html/charset"
)

type Importer interface {
	Info() LedgerEmailImporterInfo
	Matches(from string, subject string) bool
	Parse(parsed ParsedEmail) ([]ParsedOrder, []string, error)
	CounterpartyPattern() *regexp.Regexp
}

type ParsedEmail struct {
	MessageID string
	From      string
	Subject   string
	Date      time.Time
	TextBody  string
	HTMLBody  string
}

type ParsedOrder struct {
	ExternalOrderID string
	OrderDate       string
	TotalMinor      int64
	Currency        string
	Items           []LedgerEmailOrderItem
}

type UploadedMessage struct {
	Filename  string
	Reader    io.Reader
	MessageID string
}

type EmailService struct {
	store     *Store
	logger    *slog.Logger
	importers []Importer
}

func NewEmailService(s *Store, logger *slog.Logger) *EmailService {
	return &EmailService{
		store:  s,
		logger: logger,
		importers: []Importer{
			newAmazonDEImporter(),
			newPayPalDEImporter(),
		},
	}
}

func (s *EmailService) ListImporters() []LedgerEmailImporterInfo {
	items := make([]LedgerEmailImporterInfo, 0, len(s.importers))
	for _, importer := range s.importers {
		items = append(items, importer.Info())
	}
	return items
}

func (s *EmailService) ScanUploadedMessages(ctx context.Context, userID string, account LedgerEmailAccount, files []UploadedMessage) (LedgerEmailScanResult, error) {
	result := LedgerEmailScanResult{Warnings: []string{}, Orders: []LedgerEmailOrder{}}
	for _, file := range files {
		result.EmailsScanned++
		parsed, err := parseMessage(file.Reader)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}
		if parsed.MessageID == "" {
			parsed.MessageID = strings.TrimSpace(file.MessageID)
		}
		importer := s.matchImporter(parsed.From, parsed.Subject)
		if importer == nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: no importer matched sender/subject", file.Filename))
			continue
		}
		orders, warnings, err := importer.Parse(parsed)
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}
		result.OrdersFound += len(orders)
		for _, parsedOrder := range orders {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			default:
			}
			messageID := parsed.MessageID
			if messageID == "" {
				messageID = file.Filename + ":" + parsedOrder.ExternalOrderID + ":" + parsedOrder.OrderDate
			}
			existing, err := s.store.GetLedgerEmailOrderByMessageID(ctx, userID, messageID)
			if err == nil {
				result.Orders = append(result.Orders, existing)
				continue
			}
			if err != nil && err != storage.ErrNotFound {
				return result, err
			}
			now := time.Now().UTC()
			order := LedgerEmailOrder{
				ID:              uuid.New(),
				EmailAccountID:  account.ID,
				ImporterID:      importer.Info().ID,
				ExternalOrderID: parsedOrder.ExternalOrderID,
				OrderDate:       parsedOrder.OrderDate,
				TotalMinor:      parsedOrder.TotalMinor,
				Currency:        parsedOrder.Currency,
				Items:           parsedOrder.Items,
				EmailMessageID:  messageID,
				EmailSubject:    parsed.Subject,
				MatchStatus:     LedgerEmailOrderStatusUnmatched,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			linked, warnings, err := s.autoLinkOrder(ctx, userID, importer, &order)
			result.Warnings = append(result.Warnings, warnings...)
			if err != nil {
				return result, err
			}
			if err := s.store.CreateLedgerEmailOrder(ctx, userID, order); err != nil {
				return result, err
			}
			if linked {
				if _, err := s.store.LinkLedgerEmailOrder(ctx, userID, order.ID, LedgerEmailOrderLinkInput{TransactionIDs: order.LinkedTransactionIDs}); err != nil {
					return result, err
				}
				result.OrdersLinked++
			}
			result.OrdersNew++
			stored, err := s.store.GetLedgerEmailOrder(ctx, userID, order.ID)
			if err != nil {
				return result, err
			}
			result.Orders = append(result.Orders, stored)
		}
	}
	return result, nil
}

func (s *EmailService) ScanMailbox(ctx context.Context, userID string, account LedgerEmailAccount, password string) (LedgerEmailScanResult, error) {
	result := LedgerEmailScanResult{Warnings: []string{}, Orders: []LedgerEmailOrder{}}
	client, err := newIMAPClient(account.IMAPHost, account.IMAPPort, account.UseTLS)
	if err != nil {
		return result, err
	}
	defer client.Close()
	if err := client.Login(account.Username, password); err != nil {
		return result, err
	}
	defer client.Logout()
	if err := client.SelectInbox(); err != nil {
		return result, err
	}
	searchSince := account.ScanSince
	if account.LastScanAt != nil {
		lastScan := account.LastScanAt.Add(-48 * time.Hour).Format("2006-01-02")
		if searchSince == "" || lastScan > searchSince {
			searchSince = lastScan
		}
	}
	uids, err := client.SearchSince(searchSince)
	if err != nil {
		return result, err
	}
	if len(uids) == 0 {
		return result, nil
	}
	const maxFetchedMessages = 250
	if len(uids) > maxFetchedMessages {
		result.Warnings = append(result.Warnings, fmt.Sprintf("scan limited to the newest %d matching inbox messages", maxFetchedMessages))
		uids = uids[len(uids)-maxFetchedMessages:]
	}
	for i := len(uids) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		uid := uids[i]
		rawMessage, err := client.FetchMessage(uid)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("uid %d: %v", uid, err))
			continue
		}
		result.EmailsScanned++
		parsed, err := parseMessage(bytes.NewReader(rawMessage))
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("uid %d: %v", uid, err))
			continue
		}
		messageResult, err := s.processParsedEmail(ctx, userID, account, parsed, fmt.Sprintf("uid-%d", uid))
		if err != nil {
			return result, err
		}
		result.OrdersFound += messageResult.OrdersFound
		result.OrdersNew += messageResult.OrdersNew
		result.OrdersLinked += messageResult.OrdersLinked
		result.Warnings = append(result.Warnings, messageResult.Warnings...)
		result.Orders = append(result.Orders, messageResult.Orders...)
	}
	return result, nil
}

func (s *EmailService) TestConnection(account LedgerEmailAccount, password string) error {
	client, err := newIMAPClient(account.IMAPHost, account.IMAPPort, account.UseTLS)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Login(account.Username, password); err != nil {
		return err
	}
	defer client.Logout()
	if err := client.SelectInbox(); err != nil {
		return err
	}
	return nil
}

func (s *EmailService) processParsedEmail(ctx context.Context, userID string, account LedgerEmailAccount, parsed ParsedEmail, sourceLabel string) (LedgerEmailScanResult, error) {
	result := LedgerEmailScanResult{Warnings: []string{}, Orders: []LedgerEmailOrder{}}
	importer := s.matchImporter(parsed.From, parsed.Subject)
	if importer == nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: no importer matched sender/subject", sourceLabel))
		return result, nil
	}
	orders, warnings, err := importer.Parse(parsed)
	result.Warnings = append(result.Warnings, warnings...)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", sourceLabel, err))
		return result, nil
	}
	result.OrdersFound += len(orders)
	for _, parsedOrder := range orders {
		messageID := parsed.MessageID
		if messageID == "" {
			messageID = sourceLabel + ":" + parsedOrder.ExternalOrderID + ":" + parsedOrder.OrderDate
		}
		existing, err := s.store.GetLedgerEmailOrderByMessageID(ctx, userID, messageID)
		if err == nil {
			result.Orders = append(result.Orders, existing)
			continue
		}
		if err != nil && err != storage.ErrNotFound {
			return result, err
		}
		now := time.Now().UTC()
		order := LedgerEmailOrder{
			ID:              uuid.New(),
			EmailAccountID:  account.ID,
			ImporterID:      importer.Info().ID,
			ExternalOrderID: parsedOrder.ExternalOrderID,
			OrderDate:       parsedOrder.OrderDate,
			TotalMinor:      parsedOrder.TotalMinor,
			Currency:        parsedOrder.Currency,
			Items:           parsedOrder.Items,
			EmailMessageID:  messageID,
			EmailSubject:    parsed.Subject,
			MatchStatus:     LedgerEmailOrderStatusUnmatched,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		linked, linkWarnings, err := s.autoLinkOrder(ctx, userID, importer, &order)
		result.Warnings = append(result.Warnings, linkWarnings...)
		if err != nil {
			return result, err
		}
		if err := s.store.CreateLedgerEmailOrder(ctx, userID, order); err != nil {
			return result, err
		}
		if linked {
			if _, err := s.store.LinkLedgerEmailOrder(ctx, userID, order.ID, LedgerEmailOrderLinkInput{TransactionIDs: order.LinkedTransactionIDs}); err != nil {
				return result, err
			}
			result.OrdersLinked++
		}
		result.OrdersNew++
		stored, err := s.store.GetLedgerEmailOrder(ctx, userID, order.ID)
		if err != nil {
			return result, err
		}
		result.Orders = append(result.Orders, stored)
	}
	return result, nil
}

func (s *EmailService) matchImporter(from string, subject string) Importer {
	for _, importer := range s.importers {
		if importer.Matches(from, subject) {
			return importer
		}
	}
	return nil
}

func (s *EmailService) autoLinkOrder(ctx context.Context, userID string, importer Importer, order *LedgerEmailOrder) (bool, []string, error) {
	transactions, err := s.store.ListLedgerTransactionsFiltered(ctx, userID, LedgerTransactionListOptions{Limit: 500})
	if err != nil {
		return false, nil, err
	}
	candidates := make([]LedgerTransaction, 0)
	for _, txn := range transactions.Items {
		if txn.AmountMinor >= 0 {
			continue
		}
		if len(txn.EmailOrderIDs) > 0 {
			continue
		}
		if !importer.CounterpartyPattern().MatchString(strings.TrimSpace(txn.CounterpartyName + "\n" + txn.Purpose)) {
			continue
		}
		if order.OrderDate != "" && ledgerDateDeltaDays(txn.BookingDate, order.OrderDate) > 7 {
			continue
		}
		candidates = append(candidates, txn)
	}
	if len(candidates) == 0 {
		return false, nil, nil
	}
	target := order.TotalMinor
	if target < 0 {
		target = -target
	}
	exact := make([]LedgerTransaction, 0)
	for _, candidate := range candidates {
		if -candidate.AmountMinor == target {
			exact = append(exact, candidate)
		}
	}
	if len(exact) == 1 {
		order.LinkedTransactionIDs = []uuid.UUID{exact[0].ID}
		order.MatchStatus = LedgerEmailOrderStatusMatched
		return true, nil, nil
	}
	if len(exact) > 1 {
		return false, []string{fmt.Sprintf("multiple exact transaction matches for email order %s", order.ExternalOrderID)}, nil
	}
	combo := findTransactionCombination(candidates, target)
	if len(combo) > 0 {
		ids := make([]uuid.UUID, 0, len(combo))
		for _, txn := range combo {
			ids = append(ids, txn.ID)
		}
		order.LinkedTransactionIDs = ids
		order.MatchStatus = LedgerEmailOrderStatusMatched
		return true, []string{fmt.Sprintf("linked split payment for order %s", order.ExternalOrderID)}, nil
	}
	return false, nil, nil
}

func findTransactionCombination(candidates []LedgerTransaction, target int64) []LedgerTransaction {
	ordered := append([]LedgerTransaction(nil), candidates...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].BookingDate > ordered[j].BookingDate
	})
	var best []LedgerTransaction
	var walk func(start int, sum int64, picked []LedgerTransaction)
	walk = func(start int, sum int64, picked []LedgerTransaction) {
		if best != nil {
			return
		}
		if sum == target && len(picked) > 1 {
			best = append([]LedgerTransaction(nil), picked...)
			return
		}
		if sum > target || len(picked) >= 4 {
			return
		}
		for i := start; i < len(ordered); i++ {
			amount := -ordered[i].AmountMinor
			if amount <= 0 {
				continue
			}
			walk(i+1, sum+amount, append(picked, ordered[i]))
		}
	}
	walk(0, 0, nil)
	return best
}

func parseMessage(r io.Reader) (ParsedEmail, error) {
	msg, err := mail.ReadMessage(bufio.NewReader(r))
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("reading message: %w", err)
	}
	subject := strings.TrimSpace(msg.Header.Get("Subject"))
	if decodedSubject, err := decodeHeader(subject); err == nil && decodedSubject != "" {
		subject = decodedSubject
	}
	from := strings.TrimSpace(msg.Header.Get("From"))
	if decodedFrom, err := decodeHeader(from); err == nil && decodedFrom != "" {
		from = decodedFrom
	}
	parsed := ParsedEmail{
		MessageID: strings.TrimSpace(msg.Header.Get("Message-Id")),
		From:      from,
		Subject:   subject,
	}
	if dateHeader := strings.TrimSpace(msg.Header.Get("Date")); dateHeader != "" {
		if parsedDate, err := mail.ParseDate(dateHeader); err == nil {
			parsed.Date = parsedDate.UTC()
		}
	}
	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("reading body: %w", err)
	}
	mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(bytes.NewReader(bodyBytes), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return ParsedEmail{}, fmt.Errorf("reading multipart body: %w", err)
			}
			partBytes, err := readMessagePart(part)
			if err != nil {
				return ParsedEmail{}, fmt.Errorf("reading part body: %w", err)
			}
			partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			switch partType {
			case "text/plain":
				parsed.TextBody = string(partBytes)
			case "text/html":
				parsed.HTMLBody = string(partBytes)
			}
		}
	} else if mediaType == "text/html" {
		decodedBody, err := decodeBody(bodyBytes, msg.Header.Get("Content-Transfer-Encoding"))
		if err != nil {
			return ParsedEmail{}, fmt.Errorf("decoding html body: %w", err)
		}
		parsed.HTMLBody = string(decodedBody)
	} else {
		decodedBody, err := decodeBody(bodyBytes, msg.Header.Get("Content-Transfer-Encoding"))
		if err != nil {
			return ParsedEmail{}, fmt.Errorf("decoding text body: %w", err)
		}
		parsed.TextBody = string(decodedBody)
	}
	if parsed.TextBody == "" && parsed.HTMLBody != "" {
		parsed.TextBody = stripHTML(parsed.HTMLBody)
	}
	return parsed, nil
}

func readMessagePart(part *multipart.Part) ([]byte, error) {
	partBytes, err := io.ReadAll(part)
	if err != nil {
		return nil, err
	}
	return decodeBody(partBytes, part.Header.Get("Content-Transfer-Encoding"))
}

func decodeBody(body []byte, transferEncoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(transferEncoding)) {
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(body))
		return io.ReadAll(reader)
	case "base64":
		cleaned := bytes.ReplaceAll(body, []byte("\r"), nil)
		cleaned = bytes.ReplaceAll(cleaned, []byte("\n"), nil)
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(cleaned)))
		n, err := base64.StdEncoding.Decode(decoded, cleaned)
		if err != nil {
			return nil, err
		}
		return decoded[:n], nil
	default:
		return body, nil
	}
}

func decodeHeader(value string) (string, error) {
	decoder := &mime.WordDecoder{CharsetReader: charset.NewReaderLabel}
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(decoded), nil
}

func stripHTML(input string) string {
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n", "</div>", "\n")
	cleaned := replacer.Replace(input)
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	cleaned = tagPattern.ReplaceAllString(cleaned, " ")
	spacePattern := regexp.MustCompile(`[ \t\f\v]+`)
	cleaned = spacePattern.ReplaceAllString(cleaned, " ")
	linePattern := regexp.MustCompile(`\n\s+`)
	cleaned = linePattern.ReplaceAllString(cleaned, "\n")
	return strings.TrimSpace(cleaned)
}

const (
	imapDialTimeout = 15 * time.Second
	imapIOTimeout   = 60 * time.Second
)

type imapClient struct {
	conn   net.Conn
	r      *bufio.Reader
	w      *bufio.Writer
	tagNum int
}

func newIMAPClient(host string, port int, useTLS bool) (*imapClient, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: imapDialTimeout}
	var (
		conn net.Conn
		err  error
	)
	if useTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to imap: %w", err)
	}
	client := &imapClient{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn)}
	line, err := client.readLine()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading imap greeting: %w", err)
	}
	if !strings.HasPrefix(line, "*") {
		conn.Close()
		return nil, fmt.Errorf("unexpected imap greeting: %s", strings.TrimSpace(line))
	}
	return client, nil
}

func (c *imapClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *imapClient) Login(username string, password string) error {
	_, err := c.command(fmt.Sprintf("LOGIN %s %s", imapQuote(username), imapQuote(password)))
	if err != nil {
		return fmt.Errorf("logging in to imap: %w", err)
	}
	return nil
}

func (c *imapClient) Logout() {
	_, _ = c.command("LOGOUT")
}

func (c *imapClient) SelectInbox() error {
	_, err := c.command("SELECT INBOX")
	if err != nil {
		return fmt.Errorf("selecting inbox: %w", err)
	}
	return nil
}

func (c *imapClient) SearchSince(scanSince string) ([]uint32, error) {
	date, err := time.Parse("2006-01-02", scanSince)
	if err != nil {
		return nil, fmt.Errorf("invalid scan start date: %w", err)
	}
	lines, err := c.command(fmt.Sprintf("UID SEARCH SINCE %s", date.Format("2-Jan-2006")))
	if err != nil {
		return nil, fmt.Errorf("searching mailbox: %w", err)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "* SEARCH") {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "* SEARCH")))
		result := make([]uint32, 0, len(fields))
		for _, field := range fields {
			value, err := strconv.ParseUint(field, 10, 32)
			if err != nil {
				continue
			}
			result = append(result, uint32(value))
		}
		return result, nil
	}
	return nil, nil
}

func (c *imapClient) FetchMessage(uid uint32) ([]byte, error) {
	tag := c.nextTag()
	if err := c.writeLine(fmt.Sprintf("%s UID FETCH %d (BODY.PEEK[])", tag, uid)); err != nil {
		return nil, err
	}
	var body []byte
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, tag+" ") {
			if !strings.Contains(trimmed, "OK") {
				return nil, fmt.Errorf("fetch failed: %s", trimmed)
			}
			break
		}
		literalSize, ok := parseIMAPLiteralSize(line)
		if ok {
			body = make([]byte, literalSize)
			c.extendDeadline()
			if _, err := io.ReadFull(c.r, body); err != nil {
				return nil, fmt.Errorf("reading message literal: %w", err)
			}
		}
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("empty message body for uid %d", uid)
	}
	return body, nil
}

func (c *imapClient) command(command string) ([]string, error) {
	tag := c.nextTag()
	if err := c.writeLine(fmt.Sprintf("%s %s", tag, command)); err != nil {
		return nil, err
	}
	lines := make([]string, 0)
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, tag+" ") {
			if !strings.Contains(trimmed, "OK") {
				return lines, fmt.Errorf("imap command failed: %s", trimmed)
			}
			return lines, nil
		}
		if literalSize, ok := parseIMAPLiteralSize(line); ok {
			c.extendDeadline()
			if _, err := io.CopyN(io.Discard, c.r, int64(literalSize)); err != nil {
				return nil, err
			}
		}
	}
}

func (c *imapClient) nextTag() string {
	c.tagNum++
	return fmt.Sprintf("A%04d", c.tagNum)
}

// extendDeadline bounds the next read/write so a stalled IMAP server
// cannot block the calling request indefinitely.
func (c *imapClient) extendDeadline() {
	_ = c.conn.SetDeadline(time.Now().Add(imapIOTimeout))
}

func (c *imapClient) writeLine(line string) error {
	c.extendDeadline()
	if _, err := c.w.WriteString(line + "\r\n"); err != nil {
		return err
	}
	return c.w.Flush()
}

func (c *imapClient) readLine() (string, error) {
	c.extendDeadline()
	return c.r.ReadString('\n')
}

func imapQuote(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func parseIMAPLiteralSize(line string) (int, bool) {
	trimmed := strings.TrimRight(line, "\r\n")
	start := strings.LastIndex(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start == -1 || end == -1 || end <= start+1 || end != len(trimmed)-1 {
		return 0, false
	}
	size, err := strconv.Atoi(trimmed[start+1 : end])
	if err != nil || size < 0 {
		return 0, false
	}
	return size, true
}
