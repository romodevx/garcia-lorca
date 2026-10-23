package service

import (
	"fmt"
	"strings"
	"time"
)

// copy.go renders the Spanish a family reads. It is pure: a decoded event plus
// the pupil's first name in, a notification out.

// fallbackStudentName names a pupil the directory could not name.
const fallbackStudentName = "Tu hijo(a)"

// The routing payload a push carries, read by the app to open the right screen.
const (
	dataType      = "type"
	dataStudentID = "studentId"
	// dataStudentIDs lists, comma separated, every child one announcement push
	// stands for: a parent is buzzed once for all of them (ADR 0041 D4).
	dataStudentIDs = "studentIds"
	dataDate       = "date"
	// dataReportCardID opens the issued report card (T7).
	dataReportCardID = "reportCardId"
)

// eventKind groups events by the copy and push payload they get.
type eventKind int

const (
	kindAttendance eventKind = iota + 1
	kindPayment
	kindReportCard
	// kindPaymentCorrected is a payment the office corrected (T9).
	kindPaymentCorrected
)

// decodedEvent is what the copy of one consumed event is made of. class is the
// per-class list's label ("" for a daily list, or when the saver could not
// resolve it — the copy then reads as daily rather than naming nothing).
type decodedEvent struct {
	key         string
	studentID   string
	kind        eventKind
	status      string
	previous    string // the exception this one corrects ("" = a new mark)
	date        string
	class       string
	amountCents int64
	// termName, reportCardID and version describe an issued report card.
	termName     string
	reportCardID string
	version      int32
}

// namesStudent reports whether the copy uses the pupil's name (so the directory
// is only asked when it matters).
func (e decodedEvent) namesStudent() bool {
	return e.kind == kindAttendance || e.kind == kindReportCard
}

// render builds the notification for this event, for a pupil called name.
func (e decodedEvent) render(name string) Notification {
	n := Notification{StudentID: e.studentID}
	if e.kind == kindPayment {
		n.Type, n.Title = typePayment, "Pago recibido"
		n.Body = fmt.Sprintf("Recibimos tu pago de %s MXN.", pesos(e.amountCents))
		return n
	}
	if e.kind == kindPaymentCorrected {
		// Same icon as any payment: it is one, with the right amount now.
		n.Type, n.Title = typePayment, "Pago actualizado"
		n.Body = fmt.Sprintf("Tu pago fue actualizado: %s MXN.", pesos(e.amountCents))
		return n
	}
	if e.kind == kindReportCard {
		n.Type = typeReportCard
		n.Title, n.Body = reportCardCopy(e.termName, name, e.version)
		return n
	}
	n.Type, n.Title, n.Body = attendanceCopy(e.status, name, e.class, spanishDate(e.date))
	// A change between two exceptions (absent → late) corrects what the family
	// was already told; a clear back to present already reads as a correction.
	if before, ok := exceptionNames[e.previous]; ok && e.previous != e.status && e.status != statusPresent {
		n.Title = correctionTitle
		n.Body = fmt.Sprintf("Corrección: %s (antes: %s).", strings.TrimSuffix(n.Body, "."), before)
	}
	return n
}

// attendanceCopy is the per-status copy. A per-class mark names its class, so
// the absences of one day in two classes read apart instead of as duplicates.
// statusPresent only ever arrives as a CORRECTION (a stored exception cleared).
func attendanceCopy(status, name, class, date string) (kind, title, body string) {
	perClass := class != ""
	switch status {
	case statusAbsent:
		if perClass {
			return typeAbsence, "Ausencia registrada", fmt.Sprintf("%s faltó a %s el %s.", name, class, date)
		}
		return typeAbsence, "Ausencia registrada", fmt.Sprintf("%s faltó el %s.", name, date)
	case statusLate:
		if perClass {
			return typeAttendance, "Retardo registrado", fmt.Sprintf("%s llegó con retardo a %s el %s.", name, class, date)
		}
		return typeAttendance, "Retardo registrado", fmt.Sprintf("%s llegó con retardo el %s.", name, date)
	case statusExcused:
		if perClass {
			return typeAttendance, "Ausencia justificada", fmt.Sprintf("%s tiene falta justificada en %s el %s.", name, class, date)
		}
		return typeAttendance, "Ausencia justificada", fmt.Sprintf("%s tiene falta justificada el %s.", name, date)
	default: // statusPresent: decode lets nothing else through
		if perClass {
			return typeAttendance, "Corrección de asistencia", fmt.Sprintf("Corrección: %s sí asistió el %s (%s).", name, date, class)
		}
		return typeAttendance, "Corrección de asistencia", fmt.Sprintf("Corrección: %s sí asistió el %s.", name, date)
	}
}

// correctionTitle heads every correction of what a family was already told.
const correctionTitle = "Corrección de asistencia"

// exceptionNames is how a family reads a replaced exception in a correction
// ("antes: falta"). Only the stored exceptions can be corrected from.
var exceptionNames = map[string]string{
	statusAbsent:  "falta",
	statusLate:    "retardo",
	statusExcused: "falta justificada",
}

// reportCardCopy is the copy of an issued report card: "available" for the
// first version of a period, "updated" for a later one, which replaces it.
func reportCardCopy(termName, name string, version int32) (title, body string) {
	term := strings.TrimSpace(termName)
	if version > 1 {
		if term == "" {
			return "Boleta actualizada", fmt.Sprintf("Boleta actualizada de %s.", name)
		}
		return "Boleta actualizada", fmt.Sprintf("Boleta actualizada: %s de %s.", term, name)
	}
	if term == "" {
		return "Boleta disponible", fmt.Sprintf("La boleta de %s ya está disponible.", name)
	}
	return fmt.Sprintf("Boleta del %s disponible", term), fmt.Sprintf("La boleta del %s de %s ya está disponible.", term, name)
}

// pushData is the routing payload of this event's push: ids only.
func (e decodedEvent) pushData() map[string]string {
	if e.kind == kindPayment || e.kind == kindPaymentCorrected {
		return map[string]string{dataType: typePayment, dataStudentID: e.studentID}
	}
	if e.kind == kindReportCard {
		return map[string]string{dataType: typeReportCard, dataStudentID: e.studentID, dataReportCardID: e.reportCardID}
	}
	return map[string]string{dataType: typeAttendance, dataStudentID: e.studentID, dataDate: e.date}
}

// spanishMonths names the months as a family reads them ("15 de septiembre").
var spanishMonths = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// isoDateLayout is the school-day form events carry.
const isoDateLayout = "2006-01-02"

// spanishDate renders an ISO school day as "15 de septiembre". A value that is
// not an ISO date is shown as it came rather than dropped.
func spanishDate(iso string) string {
	d, err := time.Parse(isoDateLayout, iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%d de %s", d.Day(), spanishMonths[d.Month()-1])
}
