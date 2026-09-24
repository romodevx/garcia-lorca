import { readFileSync } from "node:fs";
import { globSync } from "node:fs";
import { describe, expect, it } from "vitest";

/**
 * El contrato de la capa de captura, defendido sobre el código fuente.
 *
 * Estas reglas se pidieron para TODO el sistema, y una prueba por pantalla
 * defiende sólo las pantallas que alguien se acordó de probar. Lo que sigue
 * mira los 60 archivos a la vez, así que una pantalla nueva que teclee una
 * fecha o capture un correo a mano falla aquí el día que se escribe.
 */

/**
 * El código sin sus comentarios.
 *
 * Hace falta: estas pruebas describen en un comentario el patrón que prohíben
 * ("aquí vivía un `Number(x.replace(...)) * 100`"), y sin esto el escáner se
 * denuncia a sí mismo leyendo la explicación.
 */
function soloCodigo(fuente: string): string {
  return fuente
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");
}

const SCREENS = globSync("app/_screens/**/*.tsx", { cwd: process.cwd() })
  .filter((f) => !f.includes(".test."))
  .map((f) => [f, soloCodigo(readFileSync(f, "utf8"))] as const);

/** La pantalla de entrada y las de contraseña no pasan por la capa. */
const AUTH = ["Login.tsx", "ChangePassword.tsx", "SetPassword.tsx"];
const isAuth = (file: string): boolean => AUTH.some((a) => file.endsWith(a));

/** Cada `<input … />` que captura un correo, como texto del elemento. */
function camposDeCorreo(src: string): string[] {
  return (src.match(/<input\b[\s\S]*?\/>/g) ?? []).filter((campo) =>
    /type="email"|inputMode="email"/.test(campo),
  );
}

/** Cada `<TextField … />` cuya etiqueta dice que es un correo. */
function textFieldsDeCorreo(src: string): string[] {
  return (src.match(/<TextField\b[\s\S]*?\/>/g) ?? []).filter((campo) =>
    /label=["{`'][^"`']*correo/i.test(campo),
  );
}

describe("la capa de captura, en todo el sistema", () => {
  it("ninguna fecha se teclea a mano", () => {
    // El calendario es la única forma de capturar una fecha. Un type="date"
    // suelto no basta: sin la primitiva no lleva asterisco, ni estado de error,
    // ni el ARIA que lo acompaña.
    const offenders = SCREENS.filter(([, src]) => /<input[^>]*type="date"/.test(src)).map(([f]) => f);
    expect(offenders).toEqual([]);
  });

  it("ningún correo se captura fuera de EmailField", () => {
    // Login tiene su propio diseño (ícono dentro del campo) y no usa la
    // primitiva, pero sí la regla, que defiende la prueba siguiente.
    const offenders = SCREENS
      .filter(([f]) => !isAuth(f))
      .flatMap(([f, src]) => [
        ...camposDeCorreo(src).map(() => f),
        ...textFieldsDeCorreo(src).map(() => f),
      ]);
    expect(offenders).toEqual([]);
  });

  it("todo campo de correo a mano baja a minúsculas sin mover el cursor", () => {
    // Se revisa CADA campo, no el archivo: un archivo con un campo correcto
    // escondía a su vecino que no bajaba nada (así pasó con «¿Olvidaste tu
    // contraseña?» en Login.tsx).
    const offenders = SCREENS.flatMap(([f, src]) =>
      camposDeCorreo(src)
        .filter((campo) => !/lowercaseKeepingCaret\(/.test(campo) || /type="email"/.test(campo))
        .map(() => f),
    );
    expect(offenders).toEqual([]);
  });

  it("el escáner de correos ve lo que dice que ve", () => {
    // Sin esto, una expresión rota deja pasar todo y las dos pruebas de arriba
    // pasan en vacío.
    const onChangeAntes = `<input onChange={(e) => set(e.target.value)} type="email" />`;
    expect(camposDeCorreo(onChangeAntes)).toHaveLength(1);
    expect(camposDeCorreo(`<input inputMode="email" value={v} />`)).toHaveLength(1);
    expect(camposDeCorreo(`<input type="text" value={v} />`)).toHaveLength(0);
    expect(textFieldsDeCorreo(`<TextField label="Correo del tutor" value={v} onChange={set} />`)).toHaveLength(1);
    expect(textFieldsDeCorreo(`<TextField label="Nombre" value={v} onChange={set} />`)).toHaveLength(0);
  });

  it("nadie dibuja su propia etiqueta con asterisco", () => {
    // Había tres implementaciones del asterisco, cada una con su propio
    // criterio de cuándo pintarlo y ninguna con estado de error.
    const offenders = SCREENS
      .filter(([f]) => !isAuth(f))
      // AdminScreen conserva una etiqueta propia para la contraseña temporal,
      // que a propósito NO pasa por la capa de captura.
      .filter(([f]) => !f.endsWith("AdminScreen.tsx"))
      .filter(([, src]) => /color:\s*"var\(--red\)"[^}]*\}\}>\s*\*/.test(src))
      .map(([f]) => f);
    expect(offenders).toEqual([]);
  });

  it("nadie reimplementa el formateo de dinero", () => {
    // Llegaron a existir cuatro formateadores de pesos; uno redondeaba a pesos
    // enteros y mostraba $1,851 por un saldo de $1,850.50.
    const offenders = SCREENS
      .filter(([, src]) => /Intl\.NumberFormat\([^)]*currency/.test(src) || /currency:\s*"MXN"/.test(src))
      .map(([f]) => f);
    expect(offenders).toEqual([]);
  });

  it("nadie LEE pesos con su propio parser", () => {
    // Esta es la que faltaba, y por faltar dejó pasar dos sobrecobros de cien
    // veces en uniformes. El patrón es siempre el mismo: quitar caracteres a la
    // brava y multiplicar por 100. Al borrar TODA coma, "1850,50" se vuelve
    // 185050 pesos — el precio de un suéter, o el cambio que se le devuelve a
    // un padre en el mostrador.
    //
    // Formatear mal se ve; leer mal no se ve hasta que el dinero ya se movió.
    const offenders = SCREENS
      .filter(([, src]) => /Number\([^)]*replace\([^)]*\)\s*\)\s*\*\s*100/.test(src) || /parseFloat\([^)]*\)\s*\*\s*100/.test(src))
      .map(([f]) => f);
    expect(offenders).toEqual([]);
  });

  it("ninguna pantalla pinta un valor de ejemplo dentro de un campo", () => {
    // El defecto que este trabajo vino a cerrar: "ADA", "3200.00",
    // "XXXX000000XXXXXX00" en gris DENTRO del recuadro se leen como datos ya
    // capturados. El director duda, y en el peor caso guarda sin escribir.
    //
    // La capa de captura ya no tiene la prop, así que el compilador defiende
    // sus campos. Esto defiende los inputs a mano, donde TypeScript no llega.
    //
    // Los buscadores se quedan fuera a propósito: ahí el texto es una
    // instrucción ("Buscar alumno, CURP o tutor…"), el campo no guarda nada y
    // no hay registro con el que confundirlo.
    const esBuscador = (linea: string): boolean =>
      /type="search"/.test(linea) ||
      /aria-label=\{?["'`]?Buscar/i.test(linea) ||
      /placeholder="Buscar/i.test(linea) ||
      // El autocompletado de alumno de uniformes: se teclea para buscar, no
      // para capturar.
      /Escribe el nombre del alumno/.test(linea);

    const offenders = SCREENS.flatMap(([file, src]) =>
      src
        .split("\n")
        .map((linea, i) => [linea, i + 1] as const)
        .filter(([linea]) => /placeholder[=:]/.test(linea))
        .filter(([linea]) => !esBuscador(linea))
        .map(([, n]) => `${file}:${n}`),
    );
    expect(offenders).toEqual([]);
  });
});
