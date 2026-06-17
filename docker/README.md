# Local Docker stack

Levanta **todo el backend + web** en una sola red, con hot-reload. El **mobile
(Expo) NO va aquí** por diseño: corre nativo (`pnpm --filter @garcia-lorca/mobile
start`) apuntando a este stack.

## Qué levanta
- **postgres** (16) — rol `app` endurecido: no owner, `NOBYPASSRLS`, no superuser (postura RLS).
- **pubsub** — emulador de Google Pub/Sub.
- **11 servicios Go** — `attendance, billing, comms, docs, finance, gateway, grades,
  iam, scholarships, students, uniforms`. Hot-reload con `air` (editas Go → recompila solo).
- **web** — Next.js (`next dev`, hot-reload).

## Comandos (Makefile)
| Comando | Qué hace |
|---|---|
| `make run` | Levanta TODO (build + up en segundo plano) |
| `make run-fg` | Igual pero en primer plano (logs en vivo) |
| `make run-new` | **Borra todo** (contenedores + volúmenes) y reconstruye desde cero |
| `make stop` | Detiene todo (conserva datos) |
| `make down` | Elimina contenedores (conserva volúmenes) |
| `make restart` | Reinicia todo el stack |
| `make ps` | Estado de los servicios |
| `make logs` | Logs de todo (follow) |
| `make clean` | `down -v` + borra imágenes locales |
| `make gen` | `buf generate` en contenedor (sin buf en el host) |
| `make psql` | Abre `psql` en la base (rol `app`) |
| **Por servicio** | |
| `make start_iam` | Levanta un servicio |
| `make stop_iam` | Detiene un servicio |
| `make restart_gateway` | Reinicia un servicio |
| `make rebuild_web` | Reconstruye y relevanta un servicio |
| `make logs_gateway` | Logs de un servicio (follow) |
| `make sh_iam` | Shell dentro de un servicio |

## Puertos (host)
| Servicio | URL |
|---|---|
| web | http://localhost:3000 |
| gateway (edge) | http://localhost:8080 (`/healthz`, `/readyz`) |
| iam | http://localhost:8091 |
| students/finance/grades/attendance | 8092 / 8093 / 8094 / 8095 |
| comms/docs/scholarships/uniforms/billing | 8096 / 8097 / 8098 / 8099 / 8100 |
| postgres | localhost:5432 (user `app`, db `garcia_lorca`; password from `.env`) |
| pubsub emulator | localhost:8085 |

## Notas
- `gen/go` es generado (`buf generate`, gitignored). El stack crea un **stub** offline
  para que `go.work` cargue sin buf; usa `make gen` para los contratos reales.
- Los servicios hoy son **FASE 0** (solo `Health`/`Ping`) — el stack valida el pipeline
  completo (gateway → iam responde end-to-end), no lógica de negocio aún.
- Hot-reload: el código se **monta**; `air` recompila el servicio Go al guardar y
  `next dev` recarga la web. `make restart_<svc>` para forzar.
