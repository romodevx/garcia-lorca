import type { ReactNode } from "react";

/**
 * Says out loud that this environment is a demonstration.
 *
 * The reason is not cosmetic. This deployment is a public URL in the owner's
 * personal GCP project, billed to his own account, with no data-processing
 * agreement with the school. Its 400 pupils are synthetic (scripts/seed-demo.mjs
 * combines two fixed name catalogues), and they must stay that way: real pupil
 * records are the one thing here that `terraform destroy` cannot undo. Everything
 * else in this environment is disposable; a child's name that has been published
 * is not.
 *
 * So whoever sits in front of this screen — the director, a secretary asked to
 * "try it" — needs to know before they start typing. A line in a runbook does not
 * reach them; this does.
 *
 * Remove it when the school has its own environment under its own account.
 */
export function DemoBanner(): ReactNode {
  return (
    <div
      role="note"
      aria-label="Ambiente de demostración"
      style={{
        background: "var(--warn-bg)",
        color: "var(--warn-fg)",
        borderBottom: "1px solid var(--border-strong)",
        padding: "6px 16px",
        fontSize: 13,
        lineHeight: 1.4,
        textAlign: "center",
        fontWeight: 500,
      }}
    >
      <strong>Ambiente de demostración</strong> · Los datos son ficticios. No
      captures información real de alumnos ni de familias.
    </div>
  );
}
