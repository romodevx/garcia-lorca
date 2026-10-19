"use client";

import { useCallback, useMemo, useRef, useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { Icon } from "../../_design/Icon";
import { adjustStock, receiveStock, UniformsError, type UniformItem } from "../../uniforms/api";
import { API_URL } from "../../api-url";
import { EmptyState, ErrorNote, field, FieldError, fieldWithError, FieldLabel, label, StockPill } from "./parts";

/*
 * Both drawers are a <form>: Enter in any field submits, and a submit that
 * cannot go says why (CC-403 — without it Enter did nothing at all).
 * noValidate: the browser's own bubbles would block the submit before save()
 * marks the field.
 */
const asUniformsError = (err: unknown): UniformsError =>
  err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal. Intenta de nuevo.");

const pieces = (n: number): string => `${n} ${n === 1 ? "pieza" : "piezas"}`;

/**
 * ISSUE-CC-112: stock lives per size, so an article with no size was simply
 * absent from this screen — no row, no receipt — and nobody said why.
 */
function WithoutSizesNote({ items }: { items: readonly UniformItem[] }): ReactElement {
  const names = items.map((item) => item.name).join(", ");
  return (
    <p role="status" style={{ padding: "12px 16px", borderRadius: "var(--r-lg)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontWeight: 600, fontSize: 14 }}>
      {`${names} no aparece${items.length === 1 ? "" : "n"} aquí porque no tiene${items.length === 1 ? "" : "n"} tallas. En Catálogo, abre «Tallas» y elige «Usar talla única» si se vende en una sola.`}
    </p>
  );
}

const NO_PIECES = "Escribe cuántas piezas llegaron de al menos una talla.";
const NO_DELTA = "Escribe cuánto cambia la existencia, por ejemplo -2 o 3.";
const MIN_REASON = 3;
const NO_REASON = `Escribe el motivo del ajuste (al menos ${MIN_REASON} letras).`;


/**
 * The storeroom.
 *
 * Two jobs live here and they have different rhythms: seeing what is low (every
 * week) and booking in what arrived (whenever the supplier shows up). The
 * "bajo mínimo" filter is a tab with a count rather than a colour on a row,
 * because that is how it is used — somebody opens this screen specifically to
 * find out what to reorder.
 *
 * A goods receipt is ONE garment and a column of quantities by size. A box
 * arrives with forty pieces across six sizes; asking for six separate forms is
 * how a screen gets abandoned.
 */
export function InventoryPanel({
  items,
  narrow,
  onChanged,
}: {
  items: UniformItem[];
  narrow: boolean;
  onChanged: () => void;
}): ReactElement {
  const [onlyLow, setOnlyLow] = useState(false);
  const [receiving, setReceiving] = useState<UniformItem | null>(null);
  const [adjusting, setAdjusting] = useState<{ item: UniformItem; sizeId: string; sizeLabel: string } | null>(null);
  const lowCount = useMemo(
    () => items.reduce((n, item) => n + item.sizes.filter((s) => s.belowMinimum).length, 0),
    [items],
  );

  const rows = useMemo(
    () =>
      items
        .flatMap((item) => item.sizes.map((size) => ({ item, size })))
        .filter((row) => !onlyLow || row.size.belowMinimum)
        .sort((a, b) =>
          a.item.name.localeCompare(b.item.name) || a.size.sizeOrder - b.size.sizeOrder,
        ),
    [items, onlyLow],
  );

  const withSizes = items.filter((item) => item.sizes.length > 0);
  const withoutSizes = items.filter((item) => item.active && item.sizes.length === 0);

  if (withSizes.length === 0) {
    return (
      <EmptyState
        icon="uniforms"
        title="Aún no tienes prendas con tallas"
        hint="Crea una prenda en Catálogo y genera sus tallas de un golpe. Aquí llevarás sus existencias."
      />
    );
  }

  return (
    <div style={{ display: "grid", gap: 16 }}>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap", alignItems: "center" }}>
        <div role="tablist" aria-label="Existencias" style={{ display: "flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)" }}>
          <FilterTab label="Todas" count={rows.length} active={!onlyLow} onSelect={() => setOnlyLow(false)} />
          <FilterTab label="Bajo mínimo" count={lowCount} active={onlyLow} onSelect={() => setOnlyLow(true)} />
        </div>
        <div style={{ flex: 1 }} />
        <Btn icon="plus" onClick={() => setReceiving(withSizes[0] ?? null)}>
          Entrada de almacén
        </Btn>
      </div>

      {withoutSizes.length > 0 && <WithoutSizesNote items={withoutSizes} />}

      {rows.length === 0 ? (
        <EmptyState
          icon="check"
          tone="celebrate"
          title="Nada por reponer"
          hint="Ninguna talla está por debajo de su mínimo. Cuando alguna lo esté, aparecerá aquí."
        />
      ) : narrow ? (
        <div style={{ display: "grid", gap: 10 }}>
          {rows.map(({ item, size }) => (
            <div
              key={size.id}
              style={{
                padding: 14,
                borderRadius: "var(--r-lg)",
                background: "var(--surface)",
                border: "1px solid var(--border)",
                display: "grid",
                gap: 8,
              }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", gap: 10 }}>
                <span style={{ fontWeight: 700 }}>
                  {item.name} · {size.sizeLabel}
                </span>
                <StockPill available={size.available} belowMinimum={size.belowMinimum} />
              </div>
              <p style={{ fontSize: 13, color: "var(--text-faint)" }}>
                Físico {size.onHand} · apartado {size.reserved} · mínimo {size.minStock}
              </p>
              <Btn
                kind="ghost"
                icon="settings"
                onClick={() => setAdjusting({ item, sizeId: size.id, sizeLabel: size.sizeLabel })}
              >
                Ajustar
              </Btn>
            </div>
          ))}
        </div>
      ) : (
        <div style={{ borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", overflow: "hidden" }}>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr>
                {["Prenda", "Talla", "Disponible", "Físico", "Apartado", "Mínimo", ""].map((head) => (
                  <th
                    key={head}
                    style={{
                      textAlign: "left",
                      padding: "12px 16px",
                      fontSize: 11.5,
                      fontWeight: 700,
                      textTransform: "uppercase",
                      letterSpacing: 0.4,
                      color: "var(--text-faint)",
                      borderBottom: "1px solid var(--border)",
                    }}
                  >
                    {head}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map(({ item, size }) => (
                <tr key={size.id} style={{ borderBottom: "1px solid var(--border)" }}>
                  <td style={cell}>{item.name}</td>
                  <td style={{ ...cell, fontWeight: 700 }}>{size.sizeLabel}</td>
                  <td style={cell}>
                    <StockPill available={size.available} belowMinimum={size.belowMinimum} />
                  </td>
                  <td style={cell}>{size.onHand}</td>
                  <td style={cell}>{size.reserved}</td>
                  <td style={cell}>{size.minStock}</td>
                  <td style={{ ...cell, textAlign: "right" }}>
                    <button
                      aria-label={`Ajustar ${item.name} talla ${size.sizeLabel}`}
                      onClick={() => setAdjusting({ item, sizeId: size.id, sizeLabel: size.sizeLabel })}
                      style={{ color: "var(--text-muted)" }}
                    >
                      <Icon name="settings" size={18} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {receiving !== null && (
        <ReceiveDrawer
          items={withSizes}
          initial={receiving}
          onClose={() => setReceiving(null)}
          onDone={() => {
            setReceiving(null);
            onChanged();
          }}
        />
      )}

      {adjusting !== null && (
        <AdjustDrawer
          target={adjusting}
          onClose={() => setAdjusting(null)}
          onDone={() => {
            setAdjusting(null);
            onChanged();
          }}
        />
      )}
    </div>
  );
}

const cell = { padding: "12px 16px", fontSize: 14.5, color: "var(--text)" } as const;

function FilterTab({
  label: text,
  count,
  active,
  onSelect,
}: {
  label: string;
  count: number;
  active: boolean;
  onSelect: () => void;
}): ReactElement {
  return (
    <button
      role="tab"
      aria-selected={active}
      onClick={onSelect}
      style={{
        height: 38,
        padding: "0 14px",
        borderRadius: 99,
        background: active ? "var(--surface)" : "transparent",
        color: active ? "var(--text)" : "var(--text-muted)",
        fontWeight: 600,
        fontSize: 13.5,
        fontFamily: "var(--font-display)",
        boxShadow: active ? "var(--shadow-card)" : "none",
      }}
    >
      {text} · {count}
    </button>
  );
}

/**
 * One garment, a column of quantities by size, one save.
 *
 * The alternative — a form per size — is what turns booking in a box into a
 * twenty-minute job, and a twenty-minute job is one that gets done on paper
 * instead.
 */
function ReceiveDrawer({
  items,
  initial,
  onClose,
  onDone,
}: {
  items: UniformItem[];
  initial: UniformItem;
  onClose: () => void;
  onDone: () => void;
}): ReactElement {
  const [item, setItem] = useState(initial);
  const [quantities, setQuantities] = useState<Record<string, string>>({});
  const [note, setNote] = useState("");
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UniformsError | null>(null);
  // A ref and not the state: a double click lands before `pending` repaints,
  // and booking a box in twice doubles the shelf.
  const inFlight = useRef(false);
  const [piecesError, setPiecesError] = useState<string | undefined>(undefined);
  const quantitiesRef = useRef<HTMLDivElement>(null);

  const lines = useMemo(
    () =>
      item.sizes
        .map((size) => ({ stockId: size.id, qty: Number(quantities[size.id] ?? "") }))
        .filter((line) => Number.isFinite(line.qty) && line.qty > 0),
    [item, quantities],
  );

  const save = useCallback(async (): Promise<void> => {
    if (inFlight.current) return;
    // Never a greyed-out button: saving an empty receipt says what is missing.
    if (lines.length === 0) {
      setPiecesError(NO_PIECES);
      quantitiesRef.current?.querySelector("input")?.focus();
      return;
    }
    inFlight.current = true;
    setFailure(null);
    setPending(true);
    try {
      await receiveStock(API_URL, lines, note);
      onDone();
    } catch (err) {
      // ISSUE-CC-113: the drawer stays open with what was typed; the reason shows inside it.
      setFailure(asUniformsError(err));
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }, [lines, note, onDone]);

  return (
    <Drawer title="Entrada de almacén" icon="plus" onClose={onClose}>
      <form noValidate onSubmit={(e) => { e.preventDefault(); void save(); }}>
        {failure !== null && <ErrorNote message={failure.message} detail={failure.detail} />}
        <label style={label} htmlFor="receive-item">
          Prenda
        </label>
        <select
          id="receive-item"
          style={field}
          value={item.id}
          onChange={(e) => {
            const next = items.find((i) => i.id === e.target.value);
            if (next !== undefined) {
              setItem(next);
              setQuantities({});
            }
          }}
        >
          {items.map((option) => (
            <option key={option.id} value={option.id}>
              {option.name}
            </option>
          ))}
        </select>

        <p id="receive-pieces" style={{ ...label, marginTop: 16 }}>Piezas por talla</p>
        <div ref={quantitiesRef} style={{ display: "grid", gap: 8 }}>
          {[...item.sizes]
            .sort((a, b) => a.sizeOrder - b.sizeOrder)
            .map((size) => (
              <div key={size.id} style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <span style={{ width: 70, fontWeight: 700 }}>{size.sizeLabel}</span>
                <input
                  style={{ ...fieldWithError(piecesError !== undefined), maxWidth: 120 }}
                  inputMode="numeric"
                  aria-label={`Piezas talla ${size.sizeLabel}`}
                  aria-invalid={piecesError !== undefined ? true : undefined}
                  aria-describedby={piecesError !== undefined ? "receive-pieces-error" : undefined}
                  value={quantities[size.id] ?? ""}
                  onChange={(e) => {
                    setQuantities((q) => ({ ...q, [size.id]: e.target.value }));
                    if (Number(e.target.value) > 0) setPiecesError(undefined);
                  }}
                />
                <span style={{ fontSize: 13, color: "var(--text-faint)" }}>tiene {size.onHand}</span>
              </div>
            ))}
        </div>
        <FieldError htmlFor="receive-pieces" message={piecesError} />

        <FieldLabel
          htmlFor="receive-note"
          style={{ marginTop: 16 }}
          help="De dónde vino la mercancía: el proveedor, el número de factura. Queda en el historial."
        >
          Nota
        </FieldLabel>
        <input
          id="receive-note"
          style={field}
          aria-describedby="receive-note-help"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />

        <div style={{ display: "flex", gap: 10, marginTop: 20 }}>
          <Btn kind="ghost" onClick={onClose}>
            Cancelar
          </Btn>
          <Btn type="submit" icon="check" busy={pending}>
            {pending ? "Guardando…" : `Guardar ${pieces(lines.reduce((n, l) => n + l.qty, 0))}`}
          </Btn>
        </div>
      </form>
    </Drawer>
  );
}

/**
 * A stock correction.
 *
 * The reason is required, and the field says why: this is the one write that can
 * conceal a shortfall, and the ledger entry it leaves is worthless without one.
 */
function AdjustDrawer({
  target,
  onClose,
  onDone,
}: {
  target: { item: UniformItem; sizeId: string; sizeLabel: string };
  onClose: () => void;
  onDone: () => void;
}): ReactElement {
  const [delta, setDelta] = useState("");
  const [note, setNote] = useState("");
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UniformsError | null>(null);
  const inFlight = useRef(false);
  const [deltaError, setDeltaError] = useState<string | undefined>(undefined);
  const [noteError, setNoteError] = useState<string | undefined>(undefined);
  const deltaInput = useRef<HTMLInputElement>(null);
  const noteInput = useRef<HTMLInputElement>(null);

  const parsed = Number(delta);
  const deltaOk = delta.trim() !== "" && Number.isFinite(parsed) && parsed !== 0;
  const noteOk = note.trim().length >= MIN_REASON;

  const save = useCallback(async (): Promise<void> => {
    if (inFlight.current) return;
    // Never a greyed-out button: every problem is marked, the first takes the focus.
    setDeltaError(deltaOk ? undefined : NO_DELTA);
    setNoteError(noteOk ? undefined : NO_REASON);
    if (!deltaOk || !noteOk) {
      (deltaOk ? noteInput : deltaInput).current?.focus();
      return;
    }
    inFlight.current = true;
    setFailure(null);
    setPending(true);
    try {
      await adjustStock(API_URL, target.sizeId, parsed, note.trim());
      onDone();
    } catch (err) {
      // ISSUE-CC-113: the drawer stays open with what was typed; the reason shows inside it.
      setFailure(asUniformsError(err));
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }, [deltaOk, note, noteOk, onDone, parsed, target.sizeId]);

  return (
    <Drawer title={`Ajustar ${target.item.name} · ${target.sizeLabel}`} icon="settings" onClose={onClose}>
      <form noValidate onSubmit={(e) => { e.preventDefault(); void save(); }}>
        {failure !== null && <ErrorNote message={failure.message} detail={failure.detail} />}
        <FieldLabel
          htmlFor="adjust-delta"
          help="Cuánto cambia la existencia, con signo: un número negativo resta piezas y uno positivo las suma."
        >
          Diferencia
        </FieldLabel>
        <input
          id="adjust-delta"
          ref={deltaInput}
          style={fieldWithError(deltaError !== undefined)}
          inputMode="numeric"
          aria-describedby={deltaError !== undefined ? "adjust-delta-help adjust-delta-error" : "adjust-delta-help"}
          aria-invalid={deltaError !== undefined ? true : undefined}
          value={delta}
          onChange={(e) => {
            setDelta(e.target.value);
            const n = Number(e.target.value);
            if (e.target.value.trim() !== "" && Number.isFinite(n) && n !== 0) setDeltaError(undefined);
          }}
        />
        <FieldError htmlFor="adjust-delta" message={deltaError} />
        <FieldLabel
          htmlFor="adjust-note"
          style={{ marginTop: 16 }}
          help="Por qué se ajusta: merma, conteo físico, pieza dañada. Un ajuste sin explicación no se puede auditar después."
        >
          Motivo (obligatorio)
        </FieldLabel>
        <input
          id="adjust-note"
          ref={noteInput}
          style={fieldWithError(noteError !== undefined)}
          aria-describedby={noteError !== undefined ? "adjust-note-help adjust-note-error" : "adjust-note-help"}
          aria-invalid={noteError !== undefined ? true : undefined}
          value={note}
          onChange={(e) => {
            setNote(e.target.value);
            if (e.target.value.trim().length >= MIN_REASON) setNoteError(undefined);
          }}
        />
        <FieldError htmlFor="adjust-note" message={noteError} />
        <p style={{ fontSize: 12.5, color: "var(--text-faint)", marginTop: 8 }}>
          Queda registrado con tu nombre y no se puede borrar.
        </p>
        <div style={{ display: "flex", gap: 10, marginTop: 20 }}>
          <Btn kind="ghost" onClick={onClose}>
            Cancelar
          </Btn>
          <Btn type="submit" icon="check" busy={pending}>
            {pending ? "Guardando…" : "Guardar ajuste"}
          </Btn>
        </div>
      </form>
    </Drawer>
  );
}

/** The slide-over the design system calls for. */
/**
 * El cajón del almacén. Era el noveno del sistema y el único con zIndex 50
 * cuando los otros ocho usaban 70 — el corte de caja se le montaba encima.
 * Ahora es la cáscara común, así que comparte ancho, portal y Esc con todos.
 */
export function Drawer({
  title,
  icon,
  onClose,
  children,
}: {
  title: string;
  icon: Parameters<typeof Icon>[0]["name"];
  onClose: () => void;
  children: React.ReactNode;
}): ReactElement | null {
  return (
    <DrawerShell title={title} icon={icon} size="sm" onClose={onClose}>
      {children}
    </DrawerShell>
  );
}
