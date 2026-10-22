# Tablero — QA de la app de padres, credencial flotante y comprobantes de pago

Pedido del dueño (2026-10-21). Modo de trabajo: por tarea, rama → TDD → revisión
(tech-lead, + seguridad si toca dinero/authz/datos personales) → QA visible → PR verde →
squash a `dev`.

## Decisiones del dueño

- QA **solo de la app de padres** (la web ya tuvo su QA completo); en Android **y en iOS**.
- Credencial: logo y nombre del colegio, CCT, dirección, teléfono, leyenda; foto, nombre,
  matrícula, nivel, grupo, ciclo, QR.
- Comprobantes: validar = registrar el pago con el flujo de cobro existente (completo → adeudo
  en 0; menor → parcial, recargos siguen). Dos vías: desde la cola de comprobantes o el cobro
  manual de siempre, que liga solo el comprobante pendiente del alumno. **Nunca doble pago.**
  Diseño: `.claude/docs/design/DESIGN-comprobantes-pago.md`.

## Tareas

| # | Tarea | Rama / PR | Estado |
|---|---|---|---|
| T1 | QA de la app (base local vaciada, datos creados por la web, Android + iOS) | reporte local `.gstack/qa-reports/qa-report-app-2026-10-21.md` | hecho |
| T1a | Arreglos de pantallas (Más, Perfil, Notificaciones, saldo, scroll, contraseña, huella) | #178 | en dev |
| T2 | CLABE + referencia del alumno visibles en la app | ya estaba en dev (#164); verificado | hecho |
| T3 | Botón flotante de credencial + datos del colegio en la credencial | #176 | en dev; `student_card` ON en dev (gateway-00097) |
| T4a–c | Comprobantes: finanzas, gateway, OpenAPI, web | #177 | en dev |
| T4d | Comprobantes: app (subir, ver estado; build nuevo por dependencias nativas) | #180 | en dev |
| T4e | Encender `module_payment_vouchers` en dev | — | ON en dev (gateway-00099-9d8) |
| T7 | Build de iOS para simulador + plugin UIScene (iOS 27 cerraba la app) | #179 | en dev |
| T8 | Deep link en frío (iOS + sin sesión) y «Atrás» en detalle | #181 | en dev |
| T6 | Calendario/eventos de la escuela en la app | — | espera decisión del dueño |
| T5 | Reporte final y todo en `dev` | este PR | hecho |
