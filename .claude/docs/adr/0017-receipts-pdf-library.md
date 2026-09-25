# ADR 0017: Receipts are PDFs rendered in the gateway with gopdf

- Status: Accepted
- Date: 2026-09-25
- Context: Task 12 of the September adjustments. The approved plan said "PDF in
  Go with go-pdf/fpdf (check Context7 first)".

## Problem

Context7 lists go-pdf/fpdf as **archived**: no more releases, no security
fixes. A dependency added today for a document families keep should not start
life unmaintained.

## Decision

- **Library:** `github.com/signintech/gopdf` (maintained, MIT). It has no
  built-in faces, so a TrueType font is embedded: **Poppins** (the web's display
  face, SIL Open Font License; the licence ships beside it in
  `services/gateway/internal/receipt/fonts/OFL.txt`). Regular and SemiBold only
  (~320 KB); DM Sans, the web's text face, is published as a variable font only.
- **Where:** the gateway (`internal/receipt`) assembles one `Receipt` from the
  services that own its parts — finance (the deposit and what it paid, its
  folio), uniforms (the order), students (the pupil), config (the school's
  details) — and renders it. JSON feeds the 80 mm ticket the browser prints
  (`window.print`, `@page { size: 80mm auto }`); the PDF is a letter page
  downloaded as `comprobante-<folio>.pdf`, `no-store`.
- **Tests read the PDF back** (`github.com/ledongthuc/pdf`, test-only): the
  folio, the pupil, the concepts and the total must be in its text, accents
  included.

## Consequences

- A sale's receipt has no "Recibió": uniforms does not expose who sold an
  order. Its payment method comes from finance, the payment on the delivery's
  charge.
- Folio years follow finance's clock (UTC): a deposit keyed after 18:00 local
  on 31 December takes the next year's number.
