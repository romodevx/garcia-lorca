# Demo — cómo dejar el sistema listo

> El colegio del demo se construye **por el API real**, no metiendo filas a
> Postgres por atrás. Lo que se ve en pantalla es lo que los servicios
> produjeron, con sus mismas reglas y validaciones.

## Un solo comando

```
pnpm demo:ready
```

Hace tres cosas **en este orden**, y el orden importa:

1. `demo:verify` — la suite desde cero (24 specs, escritorio + teléfono).
   **Vacía la base**: construir un colegio desde nada es justamente su tema.
2. `demo:seed` — vacía otra vez y siembra el ciclo completo.
3. `demo:smoke` — 6 specs **de solo lectura** sobre los datos sembrados.

Correr `demo:verify` *después* de sembrar dejaría un colegio vacío una hora antes
de subir al escenario. Por eso existe `demo:ready`.

## Los comandos por separado

| Comando | Qué hace | ¿Borra datos? |
|---|---|---|
| `pnpm demo:reset` | Deja solo al usuario director | **Sí** |
| `pnpm demo:verify` | Suite desde cero | **Sí** |
| `pnpm demo:seed` | Vacía y siembra el ciclo completo | **Sí** |
| `pnpm demo:smoke` | Verifica que los datos sembrados se ven | No |

## Qué queda sembrado

Ciclo **2026-2027** (31 de agosto de 2026 → 9 de julio de 2027).

| | |
|---|---|
| Niveles | Kínder, Primaria, Secundaria, Preparatoria |
| Grados / grupos | 15 grados · 24 grupos · **16-17 alumnos por grupo** |
| Alumnos | **400** (100 por nivel), todos inscritos en su grupo |
| Docentes | **20**, cada uno con exactamente 9 materias-grupo |
| Materias | 114 · 12 periodos (trimestres por nivel) |
| Colegiaturas | Kínder $2,800 · Primaria $3,200 · Secundaria $3,800 · Prepa $4,500 |
| Inscripciones | 400 generadas · **260 pagadas · 140 en adeudo ($752,500)** |
| Uniformes | 6 prendas × 9 tallas · 864 piezas · 5 ventas de mostrador |

**Las colegiaturas aún no se cobran, y está bien:** la primera vence el 10 de
septiembre y el ciclo abre el 31 de agosto. Lo que hay hoy son las inscripciones,
que es exactamente lo que un colegio tendría en su sistema esta semana. Para
mostrar el cobro mensual, el botón **«Generar cargos del mes»** en Configuración
lo hace en vivo — y se puede presionar dos veces sin cobrarle a nadie de más.

## Acceso

`director@garcialorca.edu.mx` · en `http://localhost:3002`

## Ajustes

El sembrador lee del entorno: `SEED_STUDENTS_PER_LEVEL`, `SEED_TEACHERS`,
`SEED_UNIFORM_SALES`, `API_URL`. La forma del colegio (niveles, grados, materias,
precios) vive al principio de `scripts/seed-demo.mjs`, en tablas legibles.
