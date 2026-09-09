package usecase

import (
	"fmt"
	"html/template"
	"strings"
	"time"
)

// The invitation message.
//
// It lives in the use case rather than in the SMTP adapter because WHAT it says
// is a product decision — who invited you, how long the link lasts, what to do
// when it does not work — while HOW it travels is the adapter's problem. Swap
// SMTP for anything else and this text is untouched.
//
// It is the first thing a new teacher ever sees of this system, before the
// activation screen, so it is written rather than generated: a bare link from an
// unfamiliar address is what spam filters and people discard alike.

// monthsES renders a month in Spanish. time.Format cannot: Go's layout language
// only speaks English month names, and "8 de Sep" in a message to a Mexican
// school is the kind of half-translated detail that makes software feel foreign.
var monthsES = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// mexicoCity is where the school is. Rendering the deadline in UTC would tell a
// teacher in Monterrey that their link expires at a time that means nothing to
// them.
var mexicoCity = time.FixedZone("CST", -6*60*60)

// expiryPhrase renders the deadline the way a person would say it out loud.
func expiryPhrase(t time.Time) string {
	local := t.In(mexicoCity)
	return fmt.Sprintf("el %d de %s a las %02d:%02d",
		local.Day(), monthsES[local.Month()-1], local.Hour(), local.Minute())
}

// invitationHTML is escaped by html/template on every substitution. That is the
// point: displayName is typed by the director, and a hand-built string would
// make it an injection surface in the one place where the recipient is a mail
// client rather than a browser.
var invitationHTML = template.Must(template.New("invitation").Parse(`
<div style="font-family:system-ui,-apple-system,'Segoe UI',sans-serif;max-width:520px;margin:0 auto;color:#1a2b20">
  <p style="font-size:18px;font-weight:600;margin:0 0 18px">Colegio García Lorca</p>
  <p>Hola{{if .Name}} {{.Name}}{{end}}:</p>
  <p>Te damos acceso al sistema del colegio. Para entrar, sólo falta que elijas tu contraseña.</p>
  <p style="margin:26px 0">
    <a href="{{.Link}}" style="background:#1f5c3a;color:#fff;text-decoration:none;padding:13px 22px;border-radius:999px;font-weight:600;display:inline-block">Crear mi contraseña</a>
  </p>
  <p style="font-size:14px;color:#5a6b62">Si el botón no funciona, copia esta dirección en tu navegador:</p>
  <p style="font-size:13px;word-break:break-all"><a href="{{.Link}}">{{.Link}}</a></p>
  <p style="font-size:14px;color:#5a6b62">El enlace funciona hasta {{.Expires}}. Si se te pasa, pide uno nuevo en la dirección del colegio.</p>
  <p style="font-size:13px;color:#8a9a92;margin-top:28px">¿Problemas para entrar? Escribe a soporte@garcialorca.edu.mx</p>
</div>`))

// invitationMessage builds the subject and both bodies.
//
// The plain-text half is WRITTEN, not stripped from the HTML. Corporate mail
// clients show it, and an auto-generated version reads like debris — which is
// exactly the impression this message cannot afford to make on somebody who has
// never heard of this system.
func invitationMessage(displayName, link string, expires time.Time) (subject, html, text string) {
	name := strings.TrimSpace(displayName)
	phrase := expiryPhrase(expires)

	var b strings.Builder
	if err := invitationHTML.Execute(&b, struct{ Name, Link, Expires string }{name, link, phrase}); err != nil {
		// Unreachable with a parsed template and string fields, and if it ever
		// happens the text body still carries everything that matters.
		b.Reset()
	}

	greeting := "Hola:"
	if name != "" {
		greeting = "Hola " + name + ":"
	}
	text = strings.Join([]string{
		"Colegio García Lorca",
		"",
		greeting,
		"",
		"Te damos acceso al sistema del colegio. Para entrar, sólo falta que elijas tu contraseña:",
		"",
		link,
		"",
		"El enlace funciona hasta " + phrase + ". Si se te pasa, pide uno nuevo en la dirección del colegio.",
		"",
		"¿Problemas para entrar? Escribe a soporte@garcialorca.edu.mx",
	}, "\n")

	return "Te damos acceso al sistema del Colegio García Lorca", b.String(), text
}
