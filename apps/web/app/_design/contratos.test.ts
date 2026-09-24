import { readFileSync, globSync } from "node:fs";
import { describe, expect, it } from "vitest";

/**
 * Un contrato que nadie lee no es un contrato: es una promesa.
 *
 * Esta prueba existe por un caso que costó caro. `RequiredState.firstMissing` se
 * declaraba, se calculaba con un `useMemo`, se probaba, y su propio comentario
 * decía "a donde salta la vista". **Nadie lo consumía.** Durante meses cualquiera
 * que abriera la capa de captura leía que la vista saltaba al primer campo que
 * falta — y no saltaba: al guardar el alta de alumno vacía salían cinco errores,
 * sólo se veían cuatro, y el foco se quedaba en el botón.
 *
 * Lo mismo con `AssignmentsPanel.refreshKey`, documentado como "lo incrementa el
 * padre cuando cambian los grupos", metido en un array de dependencias, y que
 * ningún padre incrementaba nunca.
 *
 * El patrón es siempre el mismo y por eso engaña: **código muerto que se lee
 * como funcionalidad existente.** Una prueba que sólo ejercita ese campo no lo
 * detecta; lo mantiene vivo.
 */

/** El código sin comentarios: si no, el propio texto de arriba da falsos usos. */
function soloCodigo(fuente: string): string {
  return fuente.replace(/\/\*[\s\S]*?\*\//g, "").replace(/^\s*\/\/.*$/gm, "");
}

const TODOS = globSync("app/**/*.{ts,tsx}", { cwd: process.cwd() })
  .filter((f) => !f.includes(".test."))
  .map((f) => [f, soloCodigo(readFileSync(f, "utf8"))] as const);

/**
 * Qué se vigila:
 *   - TODA interfaz exportada de la capa de diseño — es la que comparten las 23
 *     pantallas, así que un campo muerto ahí engaña a todo el mundo.
 *   - Toda interfaz `…Props` de cualquier pantalla: es el contrato entre dos
 *     componentes, y una prop que nadie pasa es una promesa rota.
 *
 * Los tipos internos de una pantalla (un `Row` que sólo vive en su archivo) NO
 * entran: ahí "sin lector fuera" es lo normal.
 */
const vigilada = (archivo: string, iface: string): boolean =>
  archivo.startsWith("app/_design/") || iface.endsWith("Props");

/** Lo que React o el DOM ya justifican por sí solos. */
const EXENTOS = new Set(["children", "style", "className", "id", "key"]);

const BLOQUE = /^export\s+interface\s+(\w+)\s*(?:extends[^{]*)?\{(.*?)^\}/gms;
const CAMPO = /^ {2}(?:readonly\s+)?([a-zA-Z_]\w*)\??\s*:/gm;

interface Campo {
  archivo: string;
  iface: string;
  campo: string;
}

function declarados(): Campo[] {
  const out: Campo[] = [];
  for (const [archivo, src] of TODOS) {
    for (const m of src.matchAll(BLOQUE)) {
      const iface = m[1] ?? "";
      if (!vigilada(archivo, iface)) continue;
      for (const c of (m[2] ?? "").matchAll(CAMPO)) {
        const campo = c[1] ?? "";
        if (!EXENTOS.has(campo)) out.push({ archivo, iface, campo });
      }
    }
  }
  return out;
}

/** ¿Alguien lo nombra fuera del archivo que lo declara? */
function tieneLector({ archivo, campo }: Campo): boolean {
  const palabra = new RegExp(`\\b${campo}\\b`);
  return TODOS.some(([f, src]) => f !== archivo && palabra.test(src));
}

describe("los contratos del sistema", () => {
  it("ningún campo declarado se queda sin un solo lector", () => {
    const muertos = declarados()
      .filter((c) => !tieneLector(c))
      .map((c) => `${c.iface}.${c.campo} (${c.archivo})`);
    expect(muertos).toEqual([]);
  });

  it("mira lo suficiente como para que la regla signifique algo", () => {
    // Un escáner que no encuentra nada que vigilar pasa siempre. Si la capa se
    // reorganiza y estos números se caen, esta prueba dejó de proteger.
    const campos = declarados();
    expect(campos.length).toBeGreaterThan(60);
    expect(new Set(campos.map((c) => c.archivo)).size).toBeGreaterThan(10);
  });
});
