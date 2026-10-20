"use client";

import { Fragment, useCallback, useEffect, useMemo, useState, type ReactElement } from "react";
import { ticketPath } from "../../receipts/api";
import { Btn } from "../../_design/Controls";
import { listOrders, UniformsError, type Level, type UniformFulfillment, type UniformOrder } from "../../uniforms/api";
import { EmptyState, ErrorNote, LevelChips, money, SkeletonRows, type SchoolLevel } from "./parts";

import { API_URL } from "../../api-url";
import { useRoomLabels } from "../../_design/students/roomLabels";
import { ReturnSalePanel } from "./ReturnSalePanel";

const PAGE_SIZE = 20;

/**
 * The order list.
 *
 * ONE ROW IS ONE DELIVERY, not one order. The order is a container with no
 * status of its own; the delivery is what gets picked, charged, expires and is
 * handed over, so it is the delivery that has a state worth a row and a deadline
 * worth a column. An order split in two is two rows tied by the same folio stem
 * and the same accent colour — which also spares the table an accordion the
 * design system does not have and paging cannot count.
 *
 * In stage one every order is a counter sale, so every row is terminal and the
 * list reads as a sales log. The deadline column and the pick actions arrive with
 * the family ordering slice, when there is something to wait for.
 */
export function OrdersPanel({ narrow, levels }: { narrow: boolean; levels: readonly SchoolLevel[] }): ReactElement {
  const [orders, setOrders] = useState<UniformOrder[] | null>(null);
  const roomLabels = useRoomLabels();
  const rows = useMemo(() => (orders === null ? null : toRows(orders, roomLabels)), [orders, roomLabels]);
  const [total, setTotal] = useState(0);
  const [failure, setFailure] = useState<UniformsError | null>(null);
  const [level, setLevel] = useState<Level>("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  // C2-015: the row whose return panel is open, and what the last return said.
  const [returning, setReturning] = useState<string | undefined>(undefined);
  const [notice, setNotice] = useState<string | undefined>(undefined);
  const orderById = useMemo(() => new Map((orders ?? []).map((o) => [o.id, o])), [orders]);

  const load = useCallback(async (): Promise<void> => {
    setOrders(null);
    setFailure(null);
    try {
      const res = await listOrders(API_URL, {
        levelId: level === "" ? undefined : level,
        q: query.trim() === "" ? undefined : query.trim(),
        pageSize: PAGE_SIZE,
        pageToken: page === 0 ? undefined : String(page * PAGE_SIZE),
      });
      setOrders(res.orders);
      setTotal(res.totalCount);
    } catch (err) {
      setOrders([]);
      setFailure(err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal"));
    }
  }, [level, page, query]);

  useEffect(() => {
    void load();
  }, [load]);

  const filtering = level !== "" || query.trim() !== "";

  const returnPanel = (row: Row): ReactElement | null => {
    const order = row.returnOrderId === undefined ? undefined : orderById.get(row.returnOrderId);
    if (returning !== row.key || order === undefined) return null;
    return (
      <ReturnSalePanel
        order={order}
        onClose={() => setReturning(undefined)}
        onDone={(_, message) => {
          setReturning(undefined);
          setNotice(message);
          void load();
        }}
      />
    );
  };
  const rowActions = (row: Row): ReactElement => (
    <span style={{ display: "inline-flex", gap: 6, flexWrap: "wrap" }}>
      {row.receiptOrderId !== undefined && <ReprintButton orderId={row.receiptOrderId} folio={row.folioStem} />}
      {row.returnOrderId !== undefined && (
        <Btn kind="ghost" type="button" aria-label={`${row.refundPending === true ? "Terminar el reembolso de" : "Devolver"} ${row.folioStem}`} onClick={() => { setNotice(undefined); setReturning(row.key); }}>
          {row.refundPending === true ? "Terminar reembolso" : "Devolver"}
        </Btn>
      )}
    </span>
  );

  return (
    <div style={{ display: "grid", gap: 16 }}>
      {failure !== null && <ErrorNote message={failure.message} detail={failure.detail} onRetry={() => void load()} />}
      {notice !== undefined && <p role="status" style={{ margin: 0, fontWeight: 600, color: "var(--good-fg, var(--text))" }}>{notice}</p>}

      <div style={{ display: "flex", gap: 12, flexWrap: "wrap", alignItems: "center" }}>
        <input
          aria-label="Buscar por alumno o folio"
          placeholder="Buscar por alumno o folio"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
          style={{
            flex: "1 1 240px",
            minWidth: 0,
            height: 44,
            padding: "0 16px",
            borderRadius: 99,
            border: "1px solid var(--border)",
            background: "var(--surface)",
            color: "var(--text)",
            fontSize: 15,
            outline: "none",
          }}
        />
        <LevelChips
          levels={levels}
          value={level}
          onChange={(next) => {
            setLevel(next as Level);
            setPage(0);
          }}
        />
      </div>

      {rows === null ? (
        <SkeletonRows />
      ) : rows.length === 0 ? (
        filtering ? (
          <EmptyState
            icon="search"
            title="Ningún pedido coincide"
            hint="Prueba con otro nombre, otro folio, o quita los filtros."
            action={
              <Btn
                kind="ghost"
                onClick={() => {
                  setQuery("");
                  setLevel("");
                  setPage(0);
                }}
              >
                Limpiar filtros
              </Btn>
            }
          />
        ) : (
          <EmptyState
            icon="uniforms"
            title="Todavía no hay pedidos"
            hint="Las ventas de mostrador aparecen aquí en cuanto las registras. Los pedidos que las familias hagan desde la app llegarán con la siguiente etapa."
          />
        )
      ) : narrow ? (
        <div style={{ display: "grid", gap: 10 }}>
          {rows.map((row) => (
            <div
              key={row.key}
              style={{
                padding: 14,
                borderRadius: "var(--r-lg)",
                background: "var(--surface)",
                border: "1px solid var(--border)",
                borderLeft: `4px solid ${row.accent}`,
                display: "grid",
                gap: 6,
              }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", gap: 10, flexWrap: "wrap" }}>
                <FolioText stem={row.folioStem} suffix={row.folioSuffix} />
                <span style={{ fontWeight: 700 }}>{money(row.amountCents)}</span>
              </div>
              <p style={{ fontSize: 14 }}>{row.who}</p>
              <p style={{ fontSize: 13, color: "var(--text-faint)" }}>
                {row.itemsLabel} · {row.statusLabel}
              </p>
              {rowActions(row)}
              {returnPanel(row)}
            </div>
          ))}
        </div>
      ) : (
        <div style={{ borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", overflow: "hidden" }}>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr>
                {["Folio", "Alumno", "Nivel", "Prendas", "Total", "Estado", "Comprobante"].map((head) => (
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
              {rows.map((row) => (
                <Fragment key={row.key}>
                <tr style={{ borderBottom: "1px solid var(--border)" }}>
                  <td style={{ ...cell, borderLeft: `4px solid ${row.accent}` }}>
                    <FolioText stem={row.folioStem} suffix={row.folioSuffix} />
                  </td>
                  <td style={cell}>{row.who}</td>
                  <td style={cell}>{row.levelLabel}</td>
                  <td style={cell}>{row.itemsLabel}</td>
                  <td style={{ ...cell, fontWeight: 700 }}>{money(row.amountCents)}</td>
                  <td style={cell}>{row.statusLabel}</td>
                  <td style={cell}>{rowActions(row)}</td>
                </tr>
                {returning === row.key && (
                  <tr>
                    <td colSpan={7}>{returnPanel(row)}</td>
                  </tr>
                )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {total > PAGE_SIZE && (
        <div style={{ display: "flex", alignItems: "center", gap: 12, justifyContent: "flex-end", flexWrap: "wrap" }}>
          <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>
            Mostrando {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, total)} de {total}
          </span>
          <Btn kind="ghost" onClick={() => setPage((p) => Math.max(0, p - 1))} disabled={page === 0}>
            Anterior
          </Btn>
          <Btn kind="ghost" onClick={() => setPage((p) => p + 1)} disabled={(page + 1) * PAGE_SIZE >= total}>
            Siguiente
          </Btn>
        </div>
      )}
    </div>
  );
}

const cell = { padding: "12px 16px", fontSize: 14.5, color: "var(--text)" } as const;

/**
 * The folio, typeset so the stem recedes and the delivery stands out — but the
 * whole string is present, because that is what the parent reads at the window.
 */
function FolioText({ stem, suffix }: { stem: string; suffix: string }): ReactElement {
  return (
    <span style={{ fontFamily: "ui-monospace, monospace", fontSize: 13.5 }}>
      <span style={{ color: "var(--text-faint)" }}>{stem}</span>
      {suffix !== "" && <span style={{ color: "var(--text)", fontWeight: 700 }}>{suffix}</span>}
    </span>
  );
}

/** Prints a counter sale's receipt again, on its own 80 mm page (task 12). */
function ReprintButton({ orderId, folio }: { orderId: string; folio: string }): ReactElement {
  return (
    <Btn kind="ghost" type="button" aria-label={`Reimprimir ${folio}`} onClick={() => window.open(ticketPath("sale", orderId), "_blank", "noopener")}>
      Reimprimir
    </Btn>
  );
}

export interface Row {
  key: string;
  folioStem: string;
  folioSuffix: string;
  who: string;
  levelLabel: string;
  itemsLabel: string;
  amountCents: number;
  statusLabel: string;
  accent: string;
  /** Set on a counter sale: its receipt can be printed again (task 12). */
  receiptOrderId?: string;
  /** Set when the sale can be returned, or its pending refund finished (C2-015). */
  returnOrderId?: string;
  /** The return's refund is still to be recorded: the button finishes it. */
  refundPending?: boolean;
}

const STATUS_LABELS: Record<string, string> = {
  picking: "Procesando…",
  ready_for_pickup: "Lista para recoger",
  paid: "Pagada, sin entregar",
  dispatched: "Entregada",
  expired: "Vencida",
  cancelled: "Cancelada",
  sold_at_counter: "Vendida en mostrador",
  returned: "Devuelta",
};

/** A counter sale, sold or returned, is one act filed under its receipt's folio. */
const COUNTER_STATUSES: ReadonlySet<string> = new Set(["sold_at_counter", "returned"]);

/** A returned sale says so even when it also needs attention (C2-015). */
function statusLabelOf(f: UniformFulfillment): string {
  if (f.status === "returned") {
    return f.return?.refundStatus === "pending" ? "Devuelta · reembolso pendiente" : "Devuelta";
  }
  return f.requiresAttention ? "Requiere atención" : (STATUS_LABELS[f.status] ?? f.status);
}

/** A counter sale may be returned; a return with its refund pending, finished. */
function returnable(f: UniformFulfillment): boolean {
  return f.status === "sold_at_counter" || (f.status === "returned" && f.return?.refundStatus === "pending");
}

const ACCENTS = ["var(--forest)", "var(--orange)", "var(--purple)", "var(--yellow)", "var(--lime)"];

/**
 * Flattens orders into delivery rows, plus a ghost row for whatever is still
 * waiting on the supplier.
 *
 * The pending row carries NO deadline, and that absence says the thing a
 * sentence would have to explain: nothing is held yet, so no clock is running.
 */
export function toRows(orders: UniformOrder[], roomLabels: ReadonlyMap<string, string> = new Map()): Row[] {
  return orders.flatMap((order, index) => {
    const accent = ACCENTS[index % ACCENTS.length] as string;
    const who = order.studentName !== "" ? order.studentName : "Venta sin alumno";
    // The pupil's classroom with its grade (ISSUE-T14-204): the order stores only
    // the level and the letter, so the cycle's enrolment names it; else the stored level.
    const room = order.studentId !== undefined ? roomLabels.get(order.studentId) : undefined;
    const levelLabel = room ?? (order.studentLevel !== undefined && order.studentLevel !== "" ? order.studentLevel : "—");

    const rows: Row[] = order.fulfillments.map((f) => ({
      key: f.id,
      folioStem: order.folio,
      // A counter sale is one act, filed under the folio its receipt prints
      // (T14-019); the letter names one delivery of an order handed over in parts.
      folioSuffix: !COUNTER_STATUSES.has(f.status) && f.folio.startsWith(order.folio) ? f.folio.slice(order.folio.length) : "",
      who,
      levelLabel,
      itemsLabel: itemsLabel(order, f),
      amountCents: f.amountCents,
      statusLabel: statusLabelOf(f),
      accent,
      ...(f.status === "sold_at_counter" || f.status === "returned" ? { receiptOrderId: order.id } : {}),
      ...(returnable(f) ? { returnOrderId: order.id } : {}),
      ...(f.return?.refundStatus === "pending" ? { refundPending: true } : {}),
    }));

    const pending = order.lines.filter((line) => line.status === "awaiting_stock");
    if (pending.length > 0) {
      rows.push({
        key: `${order.id}-pending`,
        folioStem: order.folio,
        folioSuffix: "",
        who,
        levelLabel,
        itemsLabel: `${pending.length} ${pending.length === 1 ? "prenda" : "prendas"}`,
        amountCents: pending.reduce((sum, line) => sum + line.unitPriceCents * line.qty, 0),
        statusLabel: "Por surtir",
        accent,
      });
    }
    return rows;
  });
}

function itemsLabel(order: UniformOrder, fulfillment: UniformFulfillment): string {
  const count = order.lines.filter((line) => line.fulfillmentId === fulfillment.id).length;
  const n = count > 0 ? count : order.lines.length;
  return `${n} ${n === 1 ? "prenda" : "prendas"}`;
}
