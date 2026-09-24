"use client";

import { useCallback, useRef, useState, type ReactElement } from "react";
import { pesosToCents } from "../../_lib/money";
import { Btn } from "../../_design/Controls";
import { MoneyField, TextField, useFocusFirstInvalid, useRequired } from "../../_design/form";
import { createItem, updateItem, UniformsError, upsertSizes, type ItemCategory, type Level, type SizeSpec, type UniformItem } from "../../uniforms/api";
import { Drawer } from "./InventoryPanel";
import { EmptyState, ErrorNote, field, FieldLabel, label, LevelChips, money, type SchoolLevel } from "./parts";

import { API_URL } from "../../api-url";


// What the school sells. The module was only ever about clothing in its
// vocabulary — the stock rules, the ledger and the counter sale never were.
const CATEGORIES: { id: ItemCategory; label: string }[] = [
  { id: "uniform", label: "Uniforme" },
  { id: "book", label: "Libro" },
  { id: "supply", label: "Papelería" },
  { id: "other", label: "Otro" },
];

const CATEGORY_LABEL: Record<string, string> = {
  uniform: "Uniforme", book: "Libro", supply: "Papelería", other: "Otro",
};

/**
 * The catalogue.
 *
 * Used once at the start of a cycle and then almost never, which is exactly why
 * it is its own tab rather than a drawer inside the storeroom: creating a garment
 * and counting stock have different rhythms and should not compete for the same
 * screen.
 *
 * "Todos los niveles" is the default because most garments are the same from
 * kinder to prepa. The requirement is that a school MAY dress levels differently,
 * not that it must, and making somebody say "all four" four times is busywork.
 */
export function CatalogPanel({
  items,
  levels,
  onChanged,
}: {
  items: UniformItem[];
  /** The school's own levels, from Configuración. */
  levels: readonly SchoolLevel[];
  onChanged: () => void;
}): ReactElement {
  const [creating, setCreating] = useState(false);
  const [sizing, setSizing] = useState<UniformItem | null>(null);
  const [failure, setFailure] = useState<UniformsError | null>(null);
  const [note, setNote] = useState("");

  const retire = useCallback(
    async (item: UniformItem): Promise<void> => {
      try {
        await updateItem(API_URL, item.id, {
          name: item.name,
          description: item.description,
          levelId: item.levelId,
          priceCents: item.priceCents,
          active: !item.active,
        });
        onChanged();
      } catch (err) {
        setFailure(err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal"));
      }
    },
    [onChanged],
  );

  return (
    <div style={{ display: "grid", gap: 16 }}>
      {failure !== null && <ErrorNote message={failure.message} detail={failure.detail} />}
      {note !== "" && (
        <div style={{ padding: "12px 16px", borderRadius: "var(--r-lg)", background: "var(--ok-bg)", color: "var(--ok-fg)", fontWeight: 600, fontSize: 14 }}>
          {note}
        </div>
      )}

      <div style={{ display: "flex", justifyContent: "flex-end" }}>
        <Btn icon="plus" onClick={() => setCreating(true)}>
          Nuevo artículo
        </Btn>
      </div>

      {items.length === 0 ? (
        <EmptyState
          icon="uniforms"
          title="Crea tu primera prenda"
          hint="Una prenda es un artículo del uniforme: playera, pantalón, suéter. Después le generas todas sus tallas de un golpe."
          action={
            <Btn icon="plus" onClick={() => setCreating(true)}>
              Nuevo artículo
            </Btn>
          }
        />
      ) : (
        <div style={{ display: "grid", gap: 10 }}>
          {items.map((item) => (
            <div
              key={item.id}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 14,
                flexWrap: "wrap",
                padding: 16,
                borderRadius: "var(--r-lg)",
                background: "var(--surface)",
                border: "1px solid var(--border)",
                opacity: item.active ? 1 : 0.6,
              }}
            >
              <div style={{ flex: "1 1 200px", minWidth: 0 }}>
                <p style={{ fontWeight: 700, fontSize: 15.5 }}>
                  {item.name}
                  {!item.active && <span style={{ fontSize: 12.5, color: "var(--text-faint)" }}> · retirada</span>}
                </p>
                <p style={{ fontSize: 13.5, color: "var(--text-muted)" }}>
                  {CATEGORY_LABEL[item.category] ?? "Uniforme"} · {money(item.priceCents)} ·{" "}
                  {item.levelId === "" ? "todos los niveles" : (levels.find((l) => l.id === item.levelId)?.name ?? "nivel sin nombre")}
                </p>
              </div>
              <span style={{ fontSize: 13.5, color: item.sizes.length === 0 ? "var(--warn-fg)" : "var(--text-muted)" }}>
                {item.sizes.length === 0 ? "sin tallas" : `${item.sizes.length} tallas`}
              </span>
              <Btn kind="ghost" icon="plus" onClick={() => setSizing(item)}>
                Tallas
              </Btn>
              <Btn kind="ghost" onClick={() => void retire(item)}>
                {item.active ? "Retirar" : "Reactivar"}
              </Btn>
            </div>
          ))}
        </div>
      )}

      {creating && (
        <CreateDrawer
          levels={levels}
          onClose={() => setCreating(false)}
          onDone={(created) => {
            setCreating(false);
            onChanged();
            // Straight into sizes: a garment with no sizes cannot be sold, so
            // stopping here would leave the job half done and looking finished.
            setSizing(created);
          }}
          onError={setFailure}
        />
      )}

      {sizing !== null && (
        <SizesDrawer
          item={sizing}
          onClose={() => setSizing(null)}
          onDone={(message) => {
            setSizing(null);
            setNote(message);
            onChanged();
          }}
          onError={setFailure}
        />
      )}
    </div>
  );
}

/** Fuera del componente: useRequired lo toma como dependencia. */
const CREATE_REQUIRED = ["name", "price"] as const;

function CreateDrawer({
  levels,
  onClose,
  onDone,
  onError,
}: {
  levels: readonly SchoolLevel[];
  onClose: () => void;
  onDone: (created: UniformItem) => void;
  onError: (err: UniformsError) => void;
}): ReactElement {
  const [name, setName] = useState("");
  const [level, setLevel] = useState<Level>("");
  const [category, setCategory] = useState<ItemCategory>("uniform");
  const [pesos, setPesos] = useState("");
  const [pending, setPending] = useState(false);
  // Un ref y no el estado: dos clics llegan antes de que `pending` repinte, y
  // sin clave única nada en el servidor frena un segundo artículo igual.
  const inFlight = useRef(false);
  const form = useRequired(CREATE_REQUIRED);
  const [priceError, setPriceError] = useState<string | undefined>(undefined);
  const focusFirstInvalid = useFocusFirstInvalid();

  // El lector ESTRICTO de _lib/money. El que vivía aquí hacía
  // `Number(pesos.replace(/[^0-9.]/g, "")) * 100`, que borra toda coma incluida
  // la decimal: un precio escrito "1850,50" se guardaba como $185,050.
  // MoneyField no salva de esto — cuando el texto no es un monto reconocible
  // deliberadamente no lo corrige, para que la persona lo vea y lo arregle.
  const priceCents = pesosToCents(pesos);

  const save = useCallback(async (): Promise<void> => {
    if (inFlight.current) return;
    // Nunca se apaga el botón: al guardar se marca en el campo lo que falta.
    const complete = form.check({ name, price: pesos });
    const unreadable = pesos.trim() !== "" && Number.isNaN(priceCents);
    setPriceError(unreadable ? "Escribe el precio en pesos, por ejemplo 450." : undefined);
    if (!complete || unreadable) {
      focusFirstInvalid();
      return;
    }
    inFlight.current = true;
    setPending(true);
    try {
      const created = await createItem(API_URL, { name: name.trim(), levelId: level, priceCents, category });
      onDone(created);
    } catch (err) {
      onError(err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal"));
      onClose();
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }, [category, focusFirstInvalid, form, level, name, onClose, onDone, onError, pesos, priceCents]);

  return (
    <Drawer title="Nuevo artículo" icon="plus" onClose={onClose}>
      <p style={label}>Tipo</p>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 16 }}>
        {CATEGORIES.map((option) => (
          <button
            key={option.id}
            aria-pressed={category === option.id}
            aria-label={option.label}
            onClick={() => setCategory(option.id)}
            style={{
              height: 38,
              padding: "0 14px",
              borderRadius: 99,
              fontSize: 13.5,
              fontWeight: 600,
              cursor: "pointer",
              border: `1.5px solid ${category === option.id ? "var(--purple)" : "var(--border)"}`,
              background: category === option.id ? "color-mix(in oklab, var(--purple) 12%, var(--surface))" : "var(--surface)",
              color: category === option.id ? "var(--purple)" : "var(--text-muted)",
            }}>
            {option.label}
          </button>
        ))}
      </div>

      <TextField
        label="Nombre"
        value={name}
        onChange={setName}
        uppercase
        required
        error={form.errors.name}
        onClearError={() => form.clear("name")}
        help="Cómo se llama la prenda o el libro. Es lo que verá la familia."
      />

      <p style={{ ...label, marginTop: 16 }}>Nivel</p>
      <LevelChips levels={levels} value={level} onChange={setLevel} allLabel="Todos los niveles" />

      <div style={{ marginTop: 16 }}>
        <MoneyField
          label="Precio"
          value={pesos}
          onChange={setPesos}
          required
          error={form.errors.price ?? priceError}
          onClearError={() => {
            form.clear("price");
            setPriceError(undefined);
          }}
          help="Lo que cuesta una pieza."
        />
      </div>

      <div style={{ display: "flex", gap: 10, marginTop: 20 }}>
        <Btn kind="ghost" onClick={onClose}>
          Cancelar
        </Btn>
        <Btn icon="check" onClick={() => void save()} style={{ opacity: pending ? 0.6 : 1 }}>
          {pending ? "Guardando…" : "Crear y poner tallas"}
        </Btn>
      </div>
    </Drawer>
  );
}

/**
 * The whole size run in one go.
 *
 * A school opening the module has six garments and eight sizes each. Asking for
 * forty-eight separate forms is how a module gets abandoned on its first
 * afternoon, so this takes a range or a list and reports created and updated
 * separately — "8 tallas creadas, 2 ya existían" beats saving in silence.
 */
function SizesDrawer({
  item,
  onClose,
  onDone,
  onError,
}: {
  item: UniformItem;
  onClose: () => void;
  onDone: (message: string) => void;
  onError: (err: UniformsError) => void;
}): ReactElement {
  const [raw, setRaw] = useState(item.sizes.map((s) => s.sizeLabel).join(", "));
  const [minStock, setMinStock] = useState("");
  const [pending, setPending] = useState(false);

  const labels = parseSizes(raw);
  const min = Number(minStock);

  const save = useCallback(async (): Promise<void> => {
    setPending(true);
    try {
      const specs: SizeSpec[] = labels.map((sizeLabel, index) => ({
        sizeLabel,
        sizeOrder: index + 1,
        minStock: Number.isFinite(min) && min > 0 ? min : 0,
      }));
      const res = await upsertSizes(API_URL, item.id, specs);
      onDone(sizesMessage(res.createdCount, res.updatedCount));
    } catch (err) {
      onError(err instanceof UniformsError ? err : new UniformsError("internal", "Algo salió mal"));
      onClose();
    } finally {
      setPending(false);
    }
  }, [item.id, labels, min, onClose, onDone, onError]);

  return (
    <Drawer title={`Tallas de ${item.name}`} icon="plus" onClose={onClose}>
      <FieldLabel
        htmlFor="sizes-raw"
        help="Escribe un rango como 4-16, una lista separada por comas, o letras: CH, M, G. El orden es el que escribas."
      >
        Tallas
      </FieldLabel>
      <input
        id="sizes-raw"
        style={field}
        aria-describedby="sizes-raw-help"
        value={raw}
        onChange={(e) => setRaw(e.target.value)}
      />

      {labels.length > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginTop: 12 }}>
          {labels.map((size) => (
            <span
              key={size}
              style={{
                padding: "5px 12px",
                borderRadius: 99,
                background: "var(--surface-3)",
                fontSize: 13,
                fontWeight: 600,
              }}
            >
              {size}
            </span>
          ))}
        </div>
      )}

      <FieldLabel
        htmlFor="sizes-min"
        style={{ marginTop: 16 }}
        help="Cuántas piezas quieres tener siempre. Cuando queden menos piezas libres que este número, la talla aparece en Bajo mínimo."
      >
        Mínimo por talla
      </FieldLabel>
      <input
        id="sizes-min"
        style={field}
        inputMode="numeric"
        aria-describedby="sizes-min-help"
        value={minStock}
        onChange={(e) => setMinStock(e.target.value)}
      />

      <div style={{ display: "flex", gap: 10, marginTop: 20 }}>
        <Btn kind="ghost" onClick={onClose}>
          Cancelar
        </Btn>
        <Btn icon="check" onClick={() => void save()} disabled={labels.length === 0 || pending}>
          {pending ? "Guardando…" : `Guardar ${labels.length} tallas`}
        </Btn>
      </div>
    </Drawer>
  );
}

/**
 * Turns what somebody typed into a size run.
 *
 * "4-16" means the even sizes a school actually stocks, not every integer:
 * nobody sells a size 5 jumper. A list is taken verbatim, because letter runs
 * have no arithmetic.
 */
export function parseSizes(raw: string): string[] {
  const text = raw.trim();
  if (text === "") return [];

  const range = /^(\d{1,2})\s*-\s*(\d{1,2})$/.exec(text);
  if (range !== null) {
    const from = Number(range[1]);
    const to = Number(range[2]);
    if (from >= to) return [String(from)];
    const step = from % 2 === 0 ? 2 : 1;
    const out: string[] = [];
    for (let n = from; n <= to; n += step) out.push(String(n));
    return out;
  }

  const seen = new Set<string>();
  return text
    .split(/[,\s]+/)
    .map((part) => part.trim())
    .filter((part) => part !== "" && !seen.has(part) && seen.add(part) !== undefined);
}

/** "8 tallas creadas, 2 ya existían" beats saving in silence. */
export function sizesMessage(created: number, updated: number): string {
  const parts: string[] = [];
  if (created > 0) parts.push(`${created} ${created === 1 ? "talla creada" : "tallas creadas"}`);
  if (updated > 0) parts.push(`${updated} ${updated === 1 ? "ya existía" : "ya existían"}`);
  return parts.length === 0 ? "Sin cambios" : parts.join(", ");
}
