"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { listOrders, UniformsError, type Level, type UniformFulfillment, type UniformOrder } from "../../uniforms/api";
import { EmptyState, ErrorNote, LevelChips, money, SkeletonRows } from "./parts";

import { API_URL } from "../../api-url";

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
export function OrdersPanel({ narrow }: { narrow: boolean }): ReactElement {
  const [rows, setRows] = useState<Row[] | null>(null);
  const [total, setTotal] = useState(0);
  const [failure, setFailure] = useState<UniformsError | null>(null);
  const [level, setLevel] = useState<Level>("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);

  const load = useCallback(async (): Promise<void> => {
    setRows(null);
    setFailure(null);
    try {
      const res = await listOrders(API_URL, {
        level: level === "" ? undefined : level,
        q: query.trim() === "" ? undefined : query.trim(),
        pageSize: PAGE_SIZE,
        pageToken: page === 0 ? undefined : String(page * PAGE_SIZE),
      });
      setRows(toRows(res.orders));
      setTotal(res.totalCount);
    } catch (err) {
      setRows([]);
      setFailure(err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal"));
    }
  }, [level, page, query]);

  useEffect(() => {
    void load();
  }, [load]);

  const filtering = level !== "" || query.trim() !== "";

  return (
    <div style={{ display: "grid", gap: 16 }}>
      {failure !== null && <ErrorNote message={failure.message} detail={failure.detail} onRetry={() => void load()} />}

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
            </div>
          ))}
        </div>
      ) : (
        <div style={{ borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", overflow: "hidden" }}>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr>
                {["Folio", "Alumno", "Nivel", "Prendas", "Total", "Estado"].map((head) => (
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
                <tr key={row.key} style={{ borderBottom: "1px solid var(--border)" }}>
                  <td style={{ ...cell, borderLeft: `4px solid ${row.accent}` }}>
                    <FolioText stem={row.folioStem} suffix={row.folioSuffix} />
                  </td>
                  <td style={cell}>{row.who}</td>
                  <td style={cell}>{row.levelLabel}</td>
                  <td style={cell}>{row.itemsLabel}</td>
                  <td style={{ ...cell, fontWeight: 700 }}>{money(row.amountCents)}</td>
                  <td style={cell}>{row.statusLabel}</td>
                </tr>
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
}

const LEVEL_LABELS: Record<string, string> = {
  "": "—",
  kinder: "Kínder",
  primaria: "Primaria",
  secundaria: "Secundaria",
  preparatoria: "Prepa",
};

const STATUS_LABELS: Record<string, string> = {
  picking: "Procesando…",
  ready_for_pickup: "Lista para recoger",
  paid: "Pagada, sin entregar",
  dispatched: "Entregada",
  expired: "Vencida",
  cancelled: "Cancelada",
  sold_at_counter: "Vendida en mostrador",
};

const ACCENTS = ["var(--forest)", "var(--orange)", "var(--purple)", "var(--yellow)", "var(--lime)"];

/**
 * Flattens orders into delivery rows, plus a ghost row for whatever is still
 * waiting on the supplier.
 *
 * The pending row carries NO deadline, and that absence says the thing a
 * sentence would have to explain: nothing is held yet, so no clock is running.
 */
export function toRows(orders: UniformOrder[]): Row[] {
  return orders.flatMap((order, index) => {
    const accent = ACCENTS[index % ACCENTS.length] as string;
    const who = order.studentName !== "" ? order.studentName : "Venta sin alumno";
    const levelLabel = LEVEL_LABELS[order.studentLevel] ?? "—";

    const rows: Row[] = order.fulfillments.map((f) => ({
      key: f.id,
      folioStem: order.folio,
      folioSuffix: f.folio.startsWith(order.folio) ? f.folio.slice(order.folio.length) : "",
      who,
      levelLabel,
      itemsLabel: itemsLabel(order, f),
      amountCents: f.amountCents,
      statusLabel: f.requiresAttention ? "Requiere atención" : (STATUS_LABELS[f.status] ?? f.status),
      accent,
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
