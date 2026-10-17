"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement, type RefObject } from "react";
import { pesosToCents } from "../../_lib/money";
import { Btn } from "../../_design/Controls";
import { MoneyField, useFocusFirstInvalid } from "../../_design/form";
import { TenderEditor } from "../../_design/payments/TenderEditor";
import { useTenders } from "../../_design/payments/useTenders";
import { ClosedCutNotice } from "../../_design/payments/ClosedCutNotice";
import { useClosedCut } from "../../_design/payments/useClosedCut";
import { Icon } from "../../_design/Icon";
import { SizeQuantityPicker, type SizePick } from "../../_design/SizeQuantityPicker";
import { counterSale, UniformsError, type SaleLine, type UniformItem, type UniformOrder } from "../../uniforms/api";
import type { Student } from "../../students/api";
import { ReceiptActions } from "../receipts/ReceiptActions";
import { StudentPicker } from "./StudentPicker";
import { EmptyState, ErrorNote, FieldError, fieldWithError, FieldLabel, label, money } from "./parts";

import { API_URL } from "../../api-url";

const EMPTY_CART = "Agrega al menos una prenda al carrito.";
const NO_STUDENT = "Elige al alumno, o marca «Venta sin alumno».";
const NO_BUYER = "Escribe quién compró: sin eso la venta no se puede reclamar después.";

/** What stops the sale, in the order the screen asks for it; undefined when nothing does. */
type SaleGap = "cart" | "student" | "buyer";


/**
 * The till.
 *
 * This is the only screen in the module where a human waits, standing, in front
 * of another human. Everything about it is arranged around that: cash is
 * preselected because the school takes cash, the garment grid is a grid because
 * a school shop has six SKUs and not a marketplace, and the whole sale is four
 * interactions — garment, size, (quantity defaults to one), Cobrar.
 *
 * The pupil is ON by default and "venta sin alumno" is an explicit toggle. The
 * temptation is the other way round, because naming the pupil is the slowest
 * step; but skipping it is what quietly breaks the promise that a uniform lands
 * on the family's account statement, and a default should favour the correct
 * path and let people opt out loudly.
 */
export function CounterSalePanel({
  items,
  canSell,
  onSold,
}: {
  items: UniformItem[];
  /** False when the account lacks finance:write — the entry point says why. */
  canSell: boolean;
  onSold: () => void;
}): ReactElement {
  const [selectedItem, setSelectedItem] = useState<UniformItem | null>(null);
  const [cart, setCart] = useState<CartLine[]>([]);
  const [anonymous, setAnonymous] = useState(false);
  const [student, setStudent] = useState<Student | null>(null);
  const [note, setNote] = useState("");
  const [received, setReceived] = useState("");
  const [pending, setPending] = useState(false);
  // A ref and not the state: a double click lands before `pending` repaints.
  // The sale key already stops a second charge; this stops a second request.
  const inFlight = useRef(false);
  const [gap, setGap] = useState<SaleGap | undefined>(undefined);
  // Where each gap sends the cursor: the first garment, the pupil, the buyer.
  const firstItemRef = useRef<HTMLButtonElement>(null);
  const studentInputRef = useRef<HTMLInputElement>(null);
  const noteInputRef = useRef<HTMLInputElement>(null);
  // A counter sale enters today's cut: said at the till when it is closed (ISSUE-026).
  const cutClosed = useClosedCut("", canSell);
  const [failure, setFailure] = useState<UniformsError | null>(null);
  const [sold, setSold] = useState<UniformOrder | null>(null);
  // "Agregar al carrito" unmounts the picker with the button that had focus;
  // focus goes to the cart instead of falling to the page (task 11).
  const cartRef = useRef<HTMLElement>(null);
  const [focusCart, setFocusCart] = useState(false);
  useEffect(() => {
    if (focusCart) {
      cartRef.current?.focus();
      setFocusCart(false);
    }
  }, [focusCart]);

  // Minted when the drawer OPENS, not when the button is pressed: a retry of
  // this sale carries the same key, while the next parent in the queue carries a
  // new one. Without that distinction a double click is a second sale of goods
  // that left once.
  const [saleKey, setSaleKey] = useState(() => newSaleKey());

  const total = useMemo(() => cart.reduce((sum, line) => sum + line.unitPriceCents * line.qty, 0), [cart]);
  // How the sale is paid, method by method (T8). The server prices the sale and
  // checks the tenders against its own total.
  const tenders = useTenders(total);
  const resetTenders = tenders.reset;
  const focusFirstInvalid = useFocusFirstInvalid();
  // The change is counted against what the CASH tenders cover: with a credit
  // note paying part, the parent hands over cash for the rest only.
  const cashDue = useMemo(
    () => tenders.rows.reduce((sum, row) => {
      const cents = row.method === "cash" ? pesosToCents(row.amount) : 0;
      return Number.isFinite(cents) ? sum + cents : sum;
    }, 0),
    [tenders.rows],
  );
  const change = useMemo(() => {
    // Estricto a propósito: el lector permisivo que había aquí leía "1850,50"
    // como 185050 pesos y anunciaba un cambio cien veces mayor del que hay en
    // la caja. Un monto que no se entiende no produce cambio.
    const paid = pesosToCents(received);
    return !Number.isNaN(paid) && paid > cashDue ? paid - cashDue : 0;
  }, [received, cashDue]);

  const reset = useCallback((): void => {
    setCart([]);
    setSelectedItem(null);
    setStudent(null);
    setNote("");
    setReceived("");
    resetTenders();
    setAnonymous(false);
    setFailure(null);
    setSaleKey(newSaleKey());
  }, [resetTenders]);

  // Every size chosen for the garment, in one go (task 11): a size already in
  // the cart grows, a new one joins it.
  const addToCart = useCallback((item: UniformItem, picks: SizePick[]): void => {
    setCart((current) => {
      let next = current;
      for (const pick of picks) {
        const existing = next.find((line) => line.itemId === item.id && line.sizeLabel === pick.sizeLabel);
        next = existing !== undefined
          ? next.map((line) => (line === existing ? { ...line, qty: line.qty + pick.qty } : line))
          : [...next, { itemId: item.id, itemName: item.name, sizeLabel: pick.sizeLabel, qty: pick.qty, unitPriceCents: item.priceCents }];
      }
      return next;
    });
    setSelectedItem(null);
    setFocusCart(true);
  }, []);

  const sell = useCallback(async (): Promise<void> => {
    if (inFlight.current) return;
    // Never a greyed-out button: pressing it says what the sale still needs.
    const missing = saleGap(cart.length, anonymous, student !== null, note);
    setGap(missing);
    if (missing !== undefined) {
      focusGap({ cart: firstItemRef, student: studentInputRef, buyer: noteInputRef }, missing);
      return;
    }
    if (!tenders.validate()) {
      focusFirstInvalid();
      return;
    }
    const split = tenders.toTenders();
    const [first] = split;
    inFlight.current = true;
    setPending(true);
    setFailure(null);
    try {
      const lines: SaleLine[] = cart.map((line) => ({
        itemId: line.itemId,
        sizeLabel: line.sizeLabel,
        qty: line.qty,
      }));
      const order = await counterSale(
        API_URL,
        {
          // The pupil's ID, never what somebody typed: the first version of this
          // field sent the typed name where an id belongs, and the service —
          // correctly — had no idea what to do with it.
          studentId: anonymous ? undefined : (student?.id ?? ""),
          lines,
          // The first tender, for a server that reads only one method.
          method: first?.method ?? "cash",
          reference: first?.reference,
          tenders: split,
          note,
        },
        saleKey,
      );
      setSold(order);
      onSold();
    } catch (err) {
      setFailure(err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal"));
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }, [anonymous, cart, focusFirstInvalid, note, onSold, saleKey, student, tenders]);

  // The account may run the storeroom without being allowed to take money. Say
  // so at the door, with the reason — not after six lines have been keyed in.
  if (!canSell) {
    return (
      <EmptyState
        icon="shield"
        title="Necesitas permiso de cobros"
        hint="Tu cuenta puede administrar el almacén, pero vender en mostrador cobra dinero y eso requiere el permiso de Finanzas. Pídeselo a la dirección."
      />
    );
  }

  if (sold !== null) {
    return (
      <SaleReceipt
        order={sold}
        change={change}
        onNext={() => {
          setSold(null);
          reset();
        }}
      />
    );
  }

  const activeItems = items.filter((item) => item.active && item.sizes.length > 0);

  if (activeItems.length === 0) {
    return (
      <EmptyState
        icon="uniforms"
        title="Aún no hay prendas con tallas"
        hint="Crea una prenda en Catálogo y genera sus tallas de un golpe. Después podrás vender aquí."
      />
    );
  }

  // A gap the cashier already filled stops being shown, without a second press.
  const shownGap = gap !== undefined && saleGap(cart.length, anonymous, student !== null, note) === gap ? gap : undefined;
  const buyerError = shownGap === "buyer" ? NO_BUYER : undefined;

  return (
    <div style={{ display: "grid", gap: 18 }}>
      {failure !== null && (
        <ErrorNote
          message={failure.message}
          detail={failure.detail}
          // Out of stock means the shelf moved under us, so the catalogue is
          // stale and the honest fix is to reload it — not to retry the sale.
          onRetry={failure.isOutOfStock ? onSold : undefined}
        />
      )}

      <section>
        <p style={label}>1. Prenda</p>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 10 }}>
          {activeItems.map((item, index) => (
            <button
              key={item.id}
              ref={index === 0 ? firstItemRef : undefined}
              onClick={() => setSelectedItem(item)}
              aria-pressed={selectedItem?.id === item.id}
              style={{
                flex: "1 1 150px",
                minWidth: 0,
                padding: "14px 16px",
                textAlign: "left",
                borderRadius: "var(--r-lg)",
                border: selectedItem?.id === item.id ? "1.5px solid var(--forest)" : "1px solid var(--border)",
                background: "var(--surface)",
              }}
            >
              <p style={{ fontWeight: 700, fontSize: 15, color: "var(--text)" }}>{item.name}</p>
              <p style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{money(item.priceCents)}</p>
            </button>
          ))}
        </div>
      </section>

      {selectedItem !== null && (
        <section>
          <p style={label}>2. Talla</p>
          <SizeQuantityPicker
            key={selectedItem.id}
            sizes={[...selectedItem.sizes]
              .sort((a, b) => a.sizeOrder - b.sizeOrder)
              .map((size) => ({ label: size.sizeLabel, available: size.available }))}
            unitPriceCents={selectedItem.priceCents}
            inCart={inCartOf(cart, selectedItem.id)}
            onAdd={(picks) => addToCart(selectedItem, picks)}
          />
        </section>
      )}

      {cart.length > 0 && (
        <section
          ref={cartRef}
          tabIndex={-1}
          aria-label="Carrito"
          style={{
            padding: 16,
            borderRadius: "var(--r-lg)",
            background: "var(--surface)",
            border: "1px solid var(--border)",
            display: "grid",
            gap: 10,
          }}
        >
          {cart.map((line) => (
            <div key={`${line.itemId}-${line.sizeLabel}`} style={{ display: "flex", alignItems: "center", gap: 12 }}>
              <span style={{ flex: 1, minWidth: 0, fontSize: 14.5 }}>
                {line.itemName} · talla {line.sizeLabel}
              </span>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>× {line.qty}</span>
              <span style={{ fontWeight: 700, fontSize: 14.5, minWidth: 90, textAlign: "right" }}>
                {money(line.unitPriceCents * line.qty)}
              </span>
              <button
                aria-label={`Quitar ${line.itemName} talla ${line.sizeLabel}`}
                onClick={() => setCart((c) => c.filter((l) => l !== line))}
                style={{ color: "var(--text-faint)" }}
              >
                <Icon name="close" size={18} />
              </button>
            </div>
          ))}
          <div style={{ display: "flex", justifyContent: "space-between", borderTop: "1px solid var(--border)", paddingTop: 10 }}>
            <span style={{ fontWeight: 700 }}>Total</span>
            <span style={{ fontWeight: 800, fontSize: 19, fontFamily: "var(--font-display)" }}>{money(total)}</span>
          </div>
        </section>
      )}

      <section style={{ display: "grid", gap: 12 }}>
        <p style={label}>3. Para quién</p>
        {!anonymous ? (
          <StudentPicker value={student} onChange={setStudent} inputRef={studentInputRef} />
        ) : (
          <div
            style={{
              padding: "12px 14px",
              borderRadius: "var(--r-md)",
              background: "var(--warn-bg)",
              color: "var(--warn-fg)",
              fontSize: 13.5,
              fontWeight: 600,
            }}
          >
            Esta venta no quedará en ningún estado de cuenta. Escribe abajo quién la compró.
          </div>
        )}
        <button
          onClick={() => setAnonymous((v) => !v)}
          style={{ fontSize: 13.5, color: "var(--text-muted)", textDecoration: "underline", justifySelf: "start" }}
        >
          {anonymous ? "Es para un alumno" : "Venta sin alumno"}
        </button>
        {/*
          La etiqueta CAMBIA con el modo, y por eso tiene que verse: vivía en el
          placeholder mientras el aria-label decía siempre "Nota", así que lo que
          se veía y lo que oía un lector de pantalla eran cosas distintas.
        */}
        <div>
          <FieldLabel
            htmlFor="counter-note"
            help={
              anonymous
                ? "Sin alumno, anota quién se llevó la mercancía: sin eso la venta no se puede reclamar después."
                : "Lo que quieras recordar de esta venta."
            }
          >
            {anonymous ? "Quién compró (requerido)" : "Nota (opcional)"}
          </FieldLabel>
          <input
            id="counter-note"
            ref={noteInputRef}
            style={fieldWithError(buyerError !== undefined)}
            aria-describedby={buyerError !== undefined ? "counter-note-help counter-note-error" : "counter-note-help"}
            aria-invalid={buyerError !== undefined ? true : undefined}
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
          <FieldError htmlFor="counter-note" message={buyerError} />
        </div>
      </section>

      <section style={{ display: "grid", gap: 12 }}>
        <p style={label}>4. Cobro</p>
        <TenderEditor tenders={tenders} />
        {cashDue > 0 && (
          <div style={{ display: "flex", gap: 12, alignItems: "center", flexWrap: "wrap" }}>
            <div style={{ maxWidth: 180, minWidth: 0 }}>
              <MoneyField label="Efectivo recibido" value={received} onChange={setReceived} />
            </div>
            {change > 0 && (
              <span style={{ fontSize: 16, fontWeight: 700, color: "var(--forest)", fontFamily: "var(--font-display)" }}>
                Cambio: {money(change)}
              </span>
            )}
          </div>
        )}
      </section>

      <ClosedCutNotice closed={cutClosed} day="" />
      {(shownGap === "cart" || shownGap === "student") && (
        <p role="alert" style={{ fontSize: 14, fontWeight: 600, color: "var(--bad-fg)" }}>
          {shownGap === "cart" ? EMPTY_CART : NO_STUDENT}
        </p>
      )}
      <Btn icon="check" onClick={() => void sell()} busy={pending}>
        {pending ? "Cobrando…" : `Cobrar y entregar · ${money(total)}`}
      </Btn>
    </div>
  );
}

/** The first thing the sale lacks, in the order the screen asks for it. */
function saleGap(cartLines: number, anonymous: boolean, hasStudent: boolean, note: string): SaleGap | undefined {
  if (cartLines === 0) return "cart";
  if (anonymous) return note.trim() === "" ? "buyer" : undefined;
  return hasStudent ? undefined : "student";
}

/** Takes the cashier to what is missing: the garments, the pupil or the buyer. */
function focusGap(targets: Record<SaleGap, RefObject<HTMLElement | null>>, gap: SaleGap): void {
  targets[gap].current?.focus();
}

interface CartLine {
  itemId: string;
  itemName: string;
  sizeLabel: string;
  qty: number;
  unitPriceCents: number;
}

/**
 * What the cashier sees after the money moved.
 *
 * A toast is not enough for an operation that moved goods and cash: the folio is
 * the only handle anyone has on this sale afterwards, so it is large, and the
 * change is repeated because that is the number being counted out by hand.
 */
function SaleReceipt({
  order,
  change,
  onNext,
}: {
  order: UniformOrder;
  change: number;
  onNext: () => void;
}): ReactElement {
  return (
    <div
      style={{
        display: "grid",
        placeItems: "center",
        gap: 12,
        padding: "40px 24px",
        textAlign: "center",
        background: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--r-xl)",
      }}
    >
      <div
        style={{
          width: 56,
          height: 56,
          borderRadius: 18,
          display: "grid",
          placeItems: "center",
          background: "var(--forest)",
          color: "#fff",
        }}
      >
        <Icon name="check" size={28} />
      </div>
      <p style={{ fontSize: 15, color: "var(--text-muted)" }}>Venta registrada</p>
      <p style={{ fontSize: 30, fontWeight: 800, fontFamily: "var(--font-display)", letterSpacing: 0.5 }}>{order.folio}</p>
      <p style={{ fontSize: 18, fontWeight: 700 }}>{money(order.totalCents)}</p>
      {change > 0 && (
        <p style={{ fontSize: 17, fontWeight: 700, color: "var(--forest)" }}>Cambio: {money(change)}</p>
      )}
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap", justifyContent: "center" }}>
        <ReceiptActions kind="sale" id={order.id} />
      </div>
      <Btn icon="plus" onClick={onNext}>
        Siguiente venta
      </Btn>
    </div>
  );
}

/** Units of each size of one garment already in the cart. */
function inCartOf(cart: CartLine[], itemId: string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const line of cart) {
    if (line.itemId === itemId) {
      out[line.sizeLabel] = (out[line.sizeLabel] ?? 0) + line.qty;
    }
  }
  return out;
}

/**
 * A fresh idempotency key.
 *
 * crypto.randomUUID is not available in every browser this school uses, so there
 * is a fallback — a sale that cannot mint a key is a sale that cannot be
 * protected from a double click, which is worse than an ugly string.
 */
export function newSaleKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `till-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}
