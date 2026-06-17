/**
 * Flat → nested message transform.
 *
 * next-intl and react-i18next both interpret "." in a key as nesting and cannot
 * consume flat dotted keys. Catalogs are authored flat (type-safe via
 * `keyof typeof`, simpler parity); each framework binding runs its catalogs
 * through this transform before handing them to the engine.
 *
 *   { "action.save": "Guardar", "action.cancel": "Cancelar" }
 *      └────────────── toNested ──────────────┘
 *   { action: { save: "Guardar", cancel: "Cancelar" } }
 */

export type NestedMessages = { [key: string]: string | NestedMessages };

export function toNested(flat: Record<string, string>): NestedMessages {
  const root: NestedMessages = {};

  for (const [path, value] of Object.entries(flat)) {
    const parts = path.split(".");
    let node: NestedMessages = root;

    for (let i = 0; i < parts.length - 1; i += 1) {
      const part = parts[i] as string;
      const existing = node[part];
      if (typeof existing === "string") {
        throw new Error(
          `i18n: key collision at '${parts.slice(0, i + 1).join(".")}' — a leaf and a branch share the same path`,
        );
      }
      node = existing ?? (node[part] = {});
    }

    const leaf = parts[parts.length - 1] as string;
    if (typeof node[leaf] === "object") {
      throw new Error(`i18n: key collision at '${path}' — a branch already exists where a leaf is expected`);
    }
    node[leaf] = value;
  }

  return root;
}
