"use client";

import { useCallback, useMemo, useState, type ReactElement } from "react";
import { pesosToCents } from "../../_lib/money";
import { Btn } from "../../_design/Controls";
import { MoneyField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { counterSale, UniformsError, type SaleLine, type UniformItem, type UniformOrder } from "../../uniforms/api";
import type { Student } from "../../students/api";
import { StudentPicker } from "./StudentPicker";
import { EmptyState, ErrorNote, field, label, money } from "./parts";

import { API_URL } from "../../api-url";


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
  const [method, setMethod] = useState<"cash" | "card" | "transfer">("cash");
  const [reference, setReference] = useState("");
  const [received, setReceived] = useState("");
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UniformsError | null>(null);
  const [sold, setSold] = useState<UniformOrder | null>(null);

  // Minted when the drawer OPENS, not when the button is pressed: a retry of
  // this sale carries the same key, while the next parent in the queue carries a
  // new one. Without that distinction a double click is a second sale of goods
  // that left once.
  const [saleKey, setSaleKey] = useState(() => newSaleKey());

  const total = useMemo(() => cart.reduce((sum, line) => sum + line.unitPriceCents * line.qty, 0), [cart]);
  const change = useMemo(() => {
    // Estricto a propósito: el lector permisivo que había aquí leía "1850,50"
    // como 185050 pesos y anunciaba un cambio cien veces mayor del que hay en
    // la caja. Un monto que no se entiende no produce cambio.
    const paid = pesosToCents(received);
    return !Number.isNaN(paid) && paid > total ? paid - total : 0;
  }, [received, total]);

  const reset = useCallback((): void => {
    setCart([]);
    setSelectedItem(null);
    setStudent(null);
    setNote("");
    setReference("");
    setReceived("");
    setMethod("cash");
    setAnonymous(false);
    setFailure(null);
    setSaleKey(newSaleKey());
  }, []);

  const addToCart = useCallback((item: UniformItem, sizeLabel: string): void => {
    setCart((current) => {
      const existing = current.find((line) => line.itemId === item.id && line.sizeLabel === sizeLabel);
      if (existing !== undefined) {
        return current.map((line) =>
          line === existing ? { ...line, qty: line.qty + 1 } : line,
        );
      }
      return [
        ...current,
        { itemId: item.id, itemName: item.name, sizeLabel, qty: 1, unitPriceCents: item.priceCents },
      ];
    });
    setSelectedItem(null);
  }, []);

  const sell = useCallback(async (): Promise<void> => {
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
          method,
          reference: method === "cash" ? undefined : reference,
          note,
        },
        saleKey,
      );
      setSold(order);
      onSold();
    } catch (err) {
      setFailure(err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal"));
    } finally {
      setPending(false);
    }
  }, [anonymous, cart, method, note, onSold, reference, saleKey, student]);

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

  const canConfirm =
    cart.length > 0 &&
    !pending &&
    (anonymous ? note.trim() !== "" : student !== null);

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
          {activeItems.map((item) => (
            <button
              key={item.id}
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
          <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
            {[...selectedItem.sizes]
              .sort((a, b) => a.sizeOrder - b.sizeOrder)
              .map((size) => {
                const empty = size.available <= 0;
                return (
                  <button
                    key={size.id}
                    onClick={() => addToCart(selectedItem, size.sizeLabel)}
                    disabled={empty}
                    title={empty ? "Sin existencia" : undefined}
                    style={{
                      minWidth: 64,
                      height: 56,
                      padding: "0 12px",
                      borderRadius: "var(--r-md)",
                      border: "1px solid var(--border)",
                      background: "var(--surface)",
                      opacity: empty ? 0.45 : 1,
                      cursor: empty ? "not-allowed" : "pointer",
                      display: "grid",
                      placeItems: "center",
                      gap: 2,
                    }}
                  >
                    <span style={{ fontWeight: 700, fontSize: 15 }}>{size.sizeLabel}</span>
                    <span style={{ fontSize: 11.5, color: "var(--text-faint)" }}>
                      {empty ? "agotada" : `${size.available} libres`}
                    </span>
                  </button>
                );
              })}
          </div>
        </section>
      )}

      {cart.length > 0 && (
        <section
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
          <StudentPicker value={student} onChange={setStudent} />
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
        <input
          style={field}
          placeholder={anonymous ? "Quién compró (requerido)" : "Nota (opcional)"}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          aria-label="Nota"
        />
      </section>

      <section style={{ display: "grid", gap: 12 }}>
        <p style={label}>4. Cobro</p>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          {(["cash", "card", "transfer"] as const).map((m) => (
            <button
              key={m}
              aria-pressed={method === m}
              onClick={() => setMethod(m)}
              style={{
                height: 44,
                padding: "0 18px",
                borderRadius: 99,
                fontWeight: 600,
                fontSize: 14,
                border: method === m ? "1.5px solid var(--forest)" : "1px solid var(--border)",
                background: method === m ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
                color: method === m ? "var(--forest)" : "var(--text-muted)",
              }}
            >
              {m === "cash" ? "Efectivo" : m === "card" ? "Tarjeta" : "Transferencia"}
            </button>
          ))}
        </div>
        {method !== "cash" && (
          <input
            style={field}
            placeholder="Referencia o folio del voucher"
            value={reference}
            onChange={(e) => setReference(e.target.value)}
            aria-label="Referencia"
          />
        )}
        {method === "cash" && cart.length > 0 && (
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

      <Btn icon="check" onClick={() => void sell()} disabled={!canConfirm}>
        {pending ? "Cobrando…" : `Cobrar y entregar · ${money(total)}`}
      </Btn>
    </div>
  );
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
      <p style={{ fontSize: 13.5, color: "var(--text-faint)", maxWidth: 380 }}>
        Anota el folio si el padre lo pide. No se imprime comprobante en esta versión.
      </p>
      <Btn icon="plus" onClick={onNext}>
        Siguiente venta
      </Btn>
    </div>
  );
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
