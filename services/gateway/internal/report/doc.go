// Package report is the gateway's reusable report engine (T10, reused by the
// attendance reports of T12): a tabular Document, the periods a school reads
// by (day, week, month, year), and two renderers — a letter-size PDF with the
// same engine and faces as the receipts (gopdf + Poppins, ADR 0017) and an
// Excel workbook (.xlsx, excelize) whose money cells are numbers the school
// can add up.
//
// The engine knows nothing of finance or attendance: a report is a title,
// a few header lines and tables of text, money and counts.
package report
