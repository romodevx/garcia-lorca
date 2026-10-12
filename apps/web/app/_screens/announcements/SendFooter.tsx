"use client";

import { Fragment, useEffect, useRef, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { AnnouncementScope } from "../../comms/api";
import { summarizeRecipients } from "./audience";
import { sentMessage, type SendResult } from "./useAnnouncementComposer";

/** D7: the busy look while sending; the button stays pressable. */
const SENDING_OPACITY = 0.5;

export interface SendFooterProps {
  scope: AnnouncementScope;
  /** The picked recipients by name, in the order they were picked. */
  recipients: string[];
  error: string | undefined;
  sent: SendResult | null;
  sending: boolean;
  /** How many sends were tried; each new one that fails takes the focus to the alert (D12). */
  attempt: number;
}

/**
 * The foot of the message column (PLAN-redesign-avisos §10): who it goes to,
 * the one alert and the one status of the send, and the one "Enviar aviso".
 * The texts are today's; only where they sit and the focus are new.
 */
export function SendFooter({ scope, recipients, error, sent, sending, attempt }: SendFooterProps): ReactElement {
  const alertRef = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    if (attempt > 0 && error !== undefined) alertRef.current?.focus();
  }, [attempt, error]);

  return (
    <div className="announce-card announce-footer">
      {error !== undefined && <p ref={alertRef} role="alert" tabIndex={-1} className="announce-alert">{error}</p>}
      {sent !== null && <p role="status" className="announce-sent">{sentMessage(sent)}</p>}
      <div className="announce-footer__row">
        <Addressee scope={scope} recipients={recipients} hideHint={error !== undefined} />
        <Btn icon="check" type="submit" style={{ opacity: sending ? SENDING_OPACITY : 1 }}>
          {sending ? "Enviando…" : "Enviar aviso"}
        </Btn>
      </div>
    </div>
  );
}

interface AddresseeProps {
  scope: AnnouncementScope;
  recipients: string[];
  hideHint: boolean;
}

function Addressee({ scope, recipients, hideHint }: AddresseeProps): ReactElement {
  if (scope === "cycle") return <p className="announce-for">Para <strong>toda la escuela</strong></p>;
  if (recipients.length === 0) {
    return <p className="announce-for announce-for--hint">{hideHint ? "" : "Todavía no eliges destinatarios"}</p>;
  }
  const { shown, more } = summarizeRecipients(recipients);
  return (
    <p className="announce-for">
      Para{" "}
      {shown.map((name, i) => (
        <Fragment key={`${i}:${name}`}>
          {separatorBefore(i, shown.length, more)}
          <strong>{name}</strong>
        </Fragment>
      ))}
      {more > 0 && ` y ${more} más`}
    </p>
  );
}

/** "A, B y C"; with more to count, the "y" goes before the count instead. */
function separatorBefore(index: number, count: number, more: number): string {
  if (index === 0) return "";
  return index === count - 1 && more === 0 ? " y " : ", ";
}
