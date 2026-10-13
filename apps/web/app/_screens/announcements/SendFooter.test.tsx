import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SendFooter, type SendFooterProps } from "./SendFooter";

const BASE: SendFooterProps = { scope: "group", recipients: [], error: undefined, sent: null, sending: false, attempt: 0 };

function renderFooter(props: Partial<SendFooterProps> = {}) {
  return render(<form><SendFooter {...BASE} {...props} /></form>);
}

describe("SendFooter", () => {
  it("is the one submit button of the form", () => {
    renderFooter();
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(1);
    expect(buttons[0]).toHaveAccessibleName("Enviar aviso");
    expect(buttons[0]).toHaveAttribute("type", "submit");
  });

  // D8: up to three names, then "y N más"; as text, never as buttons.
  it("says who the announcement goes to", () => {
    const { rerender } = renderFooter({ recipients: ["2° PRIMARIA · A"] });
    expect(screen.getByText(/^Para/)).toHaveTextContent("Para 2° PRIMARIA · A");
    rerender(<form><SendFooter {...BASE} recipients={["A", "B"]} /></form>);
    expect(screen.getByText(/^Para/)).toHaveTextContent("Para A y B");
    rerender(<form><SendFooter {...BASE} recipients={["A", "B", "C", "D", "E"]} /></form>);
    expect(screen.getByText(/^Para/)).toHaveTextContent("Para A, B, C y 2 más");
  });

  it("goes to the whole school without naming anybody", () => {
    renderFooter({ scope: "cycle" });
    expect(screen.getByText(/^Para/)).toHaveTextContent("Para toda la escuela");
  });

  // E9: a hint, not the D3 error, and gone while the error shows.
  it("hints that nobody is picked yet, until an error takes its place", () => {
    const { rerender } = renderFooter();
    expect(screen.getByText("Todavía no eliges destinatarios")).toBeInTheDocument();
    rerender(<form><SendFooter {...BASE} error="Elige a quién va dirigido." attempt={1} /></form>);
    expect(screen.queryByText("Todavía no eliges destinatarios")).not.toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Elige a quién va dirigido.");
  });

  // D12: the alert takes focus after every failed send, never on its own.
  it("moves focus to the alert after a failed send", () => {
    const { rerender } = renderFooter({ error: "No pudimos leer los ciclos." });
    expect(screen.getByRole("alert")).not.toHaveFocus();
    rerender(<form><SendFooter {...BASE} error="Un aviso necesita título y mensaje." attempt={1} /></form>);
    expect(screen.getByRole("alert")).toHaveFocus();
    screen.getByRole("button").focus();
    rerender(<form><SendFooter {...BASE} error="Un aviso necesita título y mensaje." attempt={2} /></form>);
    expect(screen.getByRole("alert")).toHaveFocus();
  });

  it("reports what the send reached, in the singular at one", () => {
    const { rerender } = renderFooter({ sent: { created: 1, audience: 1 } });
    expect(screen.getByRole("status")).toHaveTextContent(/^Enviado a 1 alumno$/);
    rerender(<form><SendFooter {...BASE} sent={{ created: 0, audience: 28 }} /></form>);
    expect(screen.getByRole("status")).toHaveTextContent("Enviado a 28 alumnos · ya se había enviado, no se repitió");
  });

  // D7/D17: busy, still pressable.
  it("reads Enviando… while sending and stays enabled", () => {
    renderFooter({ sending: true });
    const button = screen.getByRole("button", { name: "Enviando…" });
    expect(button).toBeEnabled();
    expect(button).toHaveStyle({ opacity: "0.5" });
  });
});
