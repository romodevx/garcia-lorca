// Package smtp delivers mail over an authenticated SMTP relay.
//
// It exists because the identity provider's own sender lands in spam.
//
// The first version of this comment said it "does not deliver", which was
// wrong: the messages arrived, Gmail filed them as junk, and the search used to
// check could not see the spam folder. The failure was real and the diagnosis
// was not, so the reasoning is worth keeping straight — what this adapter buys
// is a sender we can ALIGN (SPF, and DKIM once the school's DNS is reachable),
// which is the only thing that moves a message out of the junk folder. Google's
// default `noreply@<project>.firebaseapp.com` is shared, unaligned with any
// domain the recipient recognises, and nothing we can improve.
//
// What stands from the original reasoning: with email-enumeration protection on,
// `sendOobCode` answers HTTP 200 for an address that has no account at all, so
// the provider's success is not evidence a message went anywhere. That is why
// this port reports a real error and the row records it.
package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// dialTimeout bounds the whole conversation.
//
// net/smtp takes no context and sets no deadlines of its own, so without this a
// half-open TCP connection to a shared mail host hangs until the OS gives up —
// minutes — holding an HTTP request open the entire time. Ten seconds is longer
// than any healthy relay needs and shorter than a director will sit staring at
// a spinner.
const dialTimeout = 10 * time.Second

// Mailer sends through an authenticated relay.
type Mailer struct {
	host     string // "mail.example.com"
	port     string // "587"
	username string
	password string
	// fromAddr is the envelope sender; fromName is what a person actually reads
	// in their inbox list, and it matters more. A message from
	// "no-reply@ceglorca.com" with no name is what both spam filters and people
	// discard.
	fromAddr string
	fromName string
}

// Config is what the composition root reads from the environment.
type Config struct {
	Host, Port, Username, Password, FromAddr, FromName string
}

// New builds a mailer, or reports that this environment has none.
//
// A missing configuration is NOT an error the service dies on: with no relay,
// inviting still works and the use case says the message did not go out, so the
// director can hand the link over another way. Refusing to start would turn "we
// have not been given an SMTP password yet" into "nobody can be given an
// account".
func New(cfg Config) (*Mailer, bool) {
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.Username) == "" {
		return nil, false
	}
	port := cfg.Port
	if port == "" {
		port = "587"
	}
	from := cfg.FromAddr
	if from == "" {
		from = cfg.Username
	}
	name := cfg.FromName
	if name == "" {
		name = "Colegio García Lorca"
	}
	return &Mailer{
		host: cfg.Host, port: port,
		username: cfg.Username, password: cfg.Password,
		fromAddr: from, fromName: name,
	}, true
}

// Send delivers one message as multipart/alternative.
//
// The whole exchange runs under a deadline and a goroutine closes the
// connection if the caller's context is cancelled first, because neither
// net/smtp nor its client accepts a context anywhere.
func (m *Mailer) Send(ctx context.Context, to, subject, htmlBody, textBody string) error {
	if err := guardHeader(to); err != nil {
		return err
	}
	msg, err := m.compose(to, subject, htmlBody, textBody)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(m.host, m.port)
	conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "no se pudo contactar al servidor de correo", err)
	}
	// One deadline for the entire conversation, plus a watchdog so a cancelled
	// request does not leave a socket held open until the deadline fires.
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		_ = conn.Close()
		return glerrors.Wrap(glerrors.KindInternal, "el servidor de correo no respondió", err)
	}
	defer func() { _ = client.Quit() }()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "no se pudo cifrar la conexión con el servidor de correo", err)
		}
	}
	if err := client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
		// The message names the cause in the words the director needs, and the
		// server's own text rides along for whoever has to fix it.
		return glerrors.Wrap(glerrors.KindInternal, "el servidor de correo rechazó nuestra contraseña", err)
	}
	if err := client.Mail(m.fromAddr); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "el servidor de correo rechazó nuestro remitente", err)
	}
	if err := client.Rcpt(to); err != nil {
		return glerrors.Wrap(glerrors.KindInvalid, "el servidor de correo rechazó esa dirección", err)
	}
	w, err := client.Data()
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "el servidor de correo no aceptó el mensaje", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "se cortó el envío del mensaje", err)
	}
	if err := w.Close(); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "el servidor de correo no confirmó el mensaje", err)
	}
	return nil
}

// guardHeader refuses anything that could break out of a header line.
//
// A display name or an address carrying CR or LF would let whoever typed it
// append headers of their own — a Bcc, a different From — and the director
// types both. The encoder below handles the display name; this catches the
// address, which is not encoded.
func guardHeader(v string) error {
	if strings.ContainsAny(v, "\r\n") {
		return glerrors.New(glerrors.KindInvalid, "invalid characters in the recipient address")
	}
	return nil
}

// compose builds the MIME message.
//
// Every human-readable header goes through mime.QEncoding: subjects and names
// in this product carry accents and ñ, and a raw 8-bit header is both illegal
// and a header-injection surface. Encoding is what makes "García" arrive as
// "García" instead of as mojibake or as a new header line.
func (m *Mailer) compose(to, subject, htmlBody, textBody string) (string, error) {
	if err := guardHeader(m.fromAddr); err != nil {
		return "", err
	}
	boundary := fmt.Sprintf("gl-%d", time.Now().UnixNano())
	enc := mime.QEncoding

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s <%s>\r\n", enc.Encode("utf-8", m.fromName), m.fromAddr)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", enc.Encode("utf-8", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)

	// Plain text first: a multipart/alternative is ordered least to most rich,
	// and the client picks the last part it can render.
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", boundary, textBody)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n", boundary, htmlBody)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.String(), nil
}

// compile-time proof the adapter satisfies the port it exists for.
var _ ports.Mailer = (*Mailer)(nil)
