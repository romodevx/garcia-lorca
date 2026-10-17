"use client";

import { type ReactElement, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { SetPassword } from "../_screens/SetPassword";
import { acceptInvitation } from "../auth/api";
import { Icon } from "../_design/Icon";
import { Btn } from "../_design/Controls";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "";

/**
 * The activation page — where an invitation link lands.
 *
 * It replaces the identity provider's own action handler, which was in English,
 * lived on a firebaseapp.com domain, and carried no sign of the school
 * anywhere. For a teacher who has never used this system, that page was the
 * first impression and it looked like somebody else's.
 *
 * There are exactly TWO failure messages, not three, and that is a security
 * decision rather than a copy one: a token that is unknown, one that expired,
 * and one already used must be indistinguishable, or the difference becomes a
 * way to probe which tokens exist. Only "the link has no token at all" is
 * separate, because that says nothing about anybody's account.
 */
export default function ActivatePage(): ReactElement {
  const params = useSearchParams();
  const router = useRouter();
  const token = params.get("token") ?? "";

  const [pending, setPending] = useState(false);
  // A ref and not the state: two presses inside one render both read `pending` as false.
  const inFlight = useRef(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [done, setDone] = useState(false);

  if (token === "") {
    return (
      <Notice
        icon="warning"
        title="Este enlace no funciona"
        body="Ábrelo directamente desde el correo que te enviamos. Si lo copiaste a mano, es fácil que se haya cortado."
      />
    );
  }

  if (done) {
    return (
      <Notice
        icon="check"
        title="Tu contraseña quedó lista"
        body="Ya puedes entrar al sistema con tu correo y la contraseña que acabas de elegir."
        action={
          <Btn icon="logout" onClick={() => router.push("/")} style={{ height: 50 }}>
            Entrar al sistema
          </Btn>
        }
      />
    );
  }

  const onSubmit = (password: string): void => {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setError(undefined);
    void acceptInvitation(API_URL, token, password)
      .then(() => setDone(true))
      .catch(() => {
        // One message for every dead token. See the note above: the three ways
        // a token dies are deliberately not distinguished here.
        setError(
          "Esta invitación ya no es válida. Pide una nueva en la dirección del colegio o escribe a soporte@garcialorca.edu.mx.",
        );
      })
      .finally(() => {
        inFlight.current = false;
        setPending(false);
      });
  };

  return (
    <SetPassword
      heading="Crea tu contraseña"
      intro="Elige la contraseña con la que vas a entrar al sistema del colegio. Sólo tú la vas a conocer."
      cta="Crear mi contraseña"
      pending={pending}
      error={error}
      onSubmit={onSubmit}
    />
  );
}

/** A full-screen message with the school's face on it, not a bare paragraph. */
function Notice({
  icon,
  title,
  body,
  action,
}: {
  icon: "warning" | "check";
  title: string;
  body: string;
  action?: ReactElement;
}): ReactElement {
  return (
    <div className="app-root" style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: "24px 20px" }}>
      <div style={{ width: "100%", maxWidth: 440, textAlign: "center" }}>
        <div
          style={{
            width: 60,
            height: 60,
            borderRadius: 20,
            margin: "0 auto 20px",
            display: "grid",
            placeItems: "center",
            background: icon === "check" ? "var(--ok-bg)" : "var(--bad-bg)",
            color: icon === "check" ? "var(--ok-fg)" : "var(--bad-fg)",
          }}
        >
          <Icon name={icon} size={30} />
        </div>
        <h1 style={{ fontFamily: "var(--font-display)", fontSize: 25, fontWeight: 800, color: "var(--text)", marginBottom: 10 }}>{title}</h1>
        <p style={{ color: "var(--text-muted)", fontSize: 15.5, lineHeight: 1.55 }}>{body}</p>
        {action !== undefined ? <div style={{ marginTop: 24 }}>{action}</div> : null}
        <p style={{ color: "var(--text-faint)", fontSize: 13, marginTop: 28 }}>
          Colegio García Lorca · <span style={{ color: "var(--forest)", fontWeight: 600 }}>soporte@garcialorca.edu.mx</span>
        </p>
      </div>
    </div>
  );
}
