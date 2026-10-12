"use client";

import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { useIntent } from "../../routing/pendingIntent";
import { listItems, listUniformLevels, UniformsError, type SchoolLevel, type UniformItem } from "../../uniforms/api";
import { CatalogPanel } from "./CatalogPanel";
import { CounterSalePanel } from "./CounterSalePanel";
import { InventoryPanel } from "./InventoryPanel";
import { OrdersPanel } from "./OrdersPanel";
import { ErrorNote, money, NARROW, SkeletonRows, StatCards } from "./parts";

import { API_URL } from "../../api-url";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";


type TabId = "counter" | "orders" | "inventory" | "catalog";

/**
 * The uniform shop.
 *
 * FOUR TABS, and that is a new pattern in this system — Estudiantes, Finanzas
 * and Asistencia all resolve to one screen. It is a deliberate exception, not an
 * inherited habit: the four jobs here run at genuinely different rhythms.
 * Mostrador is used every day, Existencias every week, Catálogo once a cycle, and
 * Pedidos is a log. Folding them together would make the daily job compete with
 * the yearly one for the same space.
 *
 * MOSTRADOR IS THE LANDING TAB, because it is the only screen in the module where
 * a person waits, standing, in front of another person.
 *
 * A note on the numbers above: the design review specified stat cards for the
 * finished module — "se pueden surtir hoy", "pagadas sin entregar". In stage one
 * every order is a counter sale, so both of those would be a permanent zero, and
 * a number that can never move is worse than no number. These are the ones that
 * mean something today; the reviewed set arrives with the family ordering slice,
 * when there is a queue to measure.
 */
export function UniformsScreen({
  onBack,
  permissions,
}: {
  onBack: () => void;
  permissions?: readonly string[];
}): ReactElement {
  const intent = useIntent("uniforms");
  const [tab, setTab] = useState<TabId>(intent === "inventory" ? "inventory" : "counter");
  const [items, setItems] = useState<UniformItem[] | null>(null);
  const [failure, setFailure] = useState<UniformsError | null>(null);
  const [narrow, setNarrow] = useState(false);

  // The storeroom is worked from a phone, standing between the shelves. A table
  // that scrolls sideways there is a table nobody uses.
  //
  // Guarded because matchMedia is not everywhere: a test renderer does not have
  // it, and neither do some older browsers. A screen that throws over a LAYOUT
  // question is worse than one that quietly shows the wide version.
  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return;
    const query = window.matchMedia(`(max-width: ${NARROW}px)`);
    const sync = (): void => setNarrow(query.matches);
    sync();
    query.addEventListener("change", sync);
    return () => query.removeEventListener("change", sync);
  }, []);

  // The school's own levels, served by the uniforms module (no config:read
  // needed). A failure leaves the chips at "Todos" alone rather than blocking
  // the till: levels only narrow.
  const [levels, setLevels] = useState<SchoolLevel[]>([]);
  useEffect(() => {
    listUniformLevels(API_URL)
      .then(setLevels)
      .catch(() => setLevels([]));
  }, []);

  const load = useCallback(async (): Promise<void> => {
    setFailure(null);
    try {
      setItems(await listItems(API_URL, { includeInactive: true }));
    } catch (err) {
      setItems([]);
      setFailure(err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal"));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const held = useMemo(() => new Set(permissions ?? []), [permissions]);
  const canSell = held.has("finance:write");
  const canSeeOrders = held.has("uniforms:orders");

  const cards = useMemo(() => {
    const all = items ?? [];
    const sizes = all.flatMap((item) => item.sizes);
    const low = sizes.filter((s) => s.belowMinimum).length;
    const empty = sizes.filter((s) => s.available <= 0).length;
    const held = sizes.reduce((n, s) => n + s.reserved, 0);
    const value = sizes.reduce((sum, size) => {
      const item = all.find((i) => i.id === size.itemId);
      return sum + (item?.priceCents ?? 0) * size.available;
    }, 0);
    return [
      {
        icon: "warning" as const,
        color: "orange" as const,
        label: "Bajo mínimo",
        value: String(low),
        sub: low === 0 ? "nada por reponer" : "tallas por reponer",
        onClick: () => setTab("inventory"),
      },
      {
        icon: "close" as const,
        color: "red" as const,
        label: "Sin existencia",
        value: String(empty),
        sub: empty === 0 ? "todo disponible" : "tallas agotadas",
        onClick: () => setTab("inventory"),
      },
      {
        icon: "clock" as const,
        color: "purple" as const,
        label: "Apartado",
        value: String(held),
        sub: "piezas comprometidas",
      },
      {
        icon: "finance" as const,
        color: "forest" as const,
        label: "Valor disponible",
        value: money(value),
        sub: "a precio de catálogo",
      },
    ];
  }, [items]);

  const tabs: { id: TabId; label: string; hidden?: boolean }[] = [
    { id: "counter", label: "Mostrador" },
    { id: "orders", label: "Pedidos", hidden: !canSeeOrders },
    { id: "inventory", label: "Existencias" },
    { id: "catalog", label: "Catálogo" },
  ];

  return (
    <Page>
      <button
        onClick={onBack}
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <header style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <ModuleHeaderIcon icon="uniforms" color="purple" />
        <div style={{ minWidth: 0 }}>
          <h1 style={{ fontSize: narrow ? 24 : 30, fontWeight: 800, color: "var(--text)" }}>Inventario</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>
            {items === null ? "Cargando…" : `${items.length} ${items.length === 1 ? "prenda" : "prendas"} en catálogo`}
          </p>
        </div>
      </header>

      <div style={{ display: "grid", gap: 18 }}>
        {failure !== null && <ErrorNote message={failure.message} detail={failure.detail} onRetry={() => void load()} />}

        <StatCards cards={cards} />

        <div
          role="tablist"
          aria-label="Secciones de uniformes"
          style={{
            display: "flex",
            gap: 6,
            padding: 5,
            background: "var(--surface-3)",
            borderRadius: "var(--r-pill)",
            border: "1px solid var(--border)",
            flexWrap: "wrap",
          }}
        >
          {tabs
            .filter((definition) => definition.hidden !== true)
            .map((definition) => {
              const on = definition.id === tab;
              return (
                <button
                  key={definition.id}
                  role="tab"
                  aria-selected={on}
                  onClick={() => setTab(definition.id)}
                  style={{
                    height: 42,
                    padding: "0 18px",
                    borderRadius: 99,
                    background: on ? "var(--surface)" : "transparent",
                    color: on ? "var(--text)" : "var(--text-muted)",
                    fontWeight: 600,
                    fontSize: 14.5,
                    fontFamily: "var(--font-display)",
                    boxShadow: on ? "var(--shadow-card)" : "none",
                  }}
                >
                  {definition.label}
                </button>
              );
            })}
        </div>

        {items === null ? (
          <SkeletonRows />
        ) : tab === "counter" ? (
          <CounterSalePanel items={items} canSell={canSell} onSold={() => void load()} />
        ) : tab === "orders" ? (
          <OrdersPanel narrow={narrow} levels={levels} />
        ) : tab === "inventory" ? (
          <InventoryPanel items={items} narrow={narrow} onChanged={() => void load()} />
        ) : (
          <CatalogPanel items={items} levels={levels} onChanged={() => void load()} />
        )}
      </div>
    </Page>
  );
}
