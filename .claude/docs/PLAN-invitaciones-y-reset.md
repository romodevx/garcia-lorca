<!-- /autoplan restore point: /Users/andresromo/.gstack/projects/romodevx-garcia-lorca/dev-autoplan-restore-20260908-123033.md -->

# PLAN — Invitación con vigencia, reenvío y "olvidé mi contraseña"

Rama base: `master` · Rama de trabajo: `dev` · Commit de partida: `0d959918`
Fecha: 2026-09-08

## Lo que pidió el dueño

1. Al dar de alta un usuario debe llegarle un correo para que **él** ponga su contraseña.
   **Hoy no llega.**
2. El botón **"¿Olvidaste tu contraseña?"** no funciona.
3. Un botón de **reenviar invitación** en los usuarios **que todavía no están activos**.
   La invitación **vence a las 24 horas**; pasado ese plazo deja de servir y el director
   la vuelve a mandar. **Una vez activo el usuario, el botón desaparece.**

## Lo que ya existe (verificado en el código, no supuesto)

| Pieza | Estado | Dónde |
|---|---|---|
| `InviteUser` (crea cuenta sin contraseña + manda enlace) | **hecho** | `services/iam/internal/usecase/admin.go:77` |
| `ResendInvitation` (por user id) | **hecho** | `admin.go:138` |
| Adaptador `Provisioner` / `Inviter` (Admin SDK + `sendOobCode`) | **hecho** | `services/iam/internal/adapters/googleid/` |
| RPCs `InviteUser` / `ResendInvitation` | **hecho** | `proto/garcialorca/iam/v1/iam.proto:87,98` |
| Rutas REST `POST /api/v1/users/invite`, `POST /api/v1/users/{id}/invite` | **hecho** | `services/gateway/cmd/main.go:622-623` |
| Botón "Reenviar acceso" en Usuarios | **hecho, pero sale en TODOS** | `apps/web/app/_screens/admin/AdminScreen.tsx:173` |
| Terraform de Identity Platform + llave web en `dev.tfvars` | **hecho** | `deploy/terraform/identity_platform.tf` |
| Pantalla para poner la contraseña | **SÍ existe, a medias** (corregido en Fase 2) | `apps/web/app/_screens/ChangePassword.tsx` — titulada "Crea tu contraseña", con validación en vivo, montada en `AppShell.tsx:362`. Falta la variante por token, y su tratamiento visual es el más pobre del producto |
| Estado "invitado / todavía no activo" | **NO existe** | `migrations/iam/0001_init.sql:19` → `status IN ('active','suspended')` |
| "¿Olvidaste tu contraseña?" | **enlace muerto** | `apps/web/app/_screens/Login.tsx:193-197` (`href="#"` + `preventDefault()`) |

## Los tres hallazgos que cambian el plan (sondas en vivo contra el proyecto `garcia-lorca`)

### H1 — CORREGIDA (2026-09-09): el proveedor **sí entrega**; el correo cae en spam

**Lo que escribí abajo es falso y lo dejo tachado en vez de borrarlo**, porque el
error importa más que la conclusión.

El dueño recibió el correo de `noreply@garcia-lorca.firebaseapp.com`. **Estaba en
spam.** Mis cuatro sondas las di por no entregadas apoyándome en búsquedas
`in:anywhere` sobre su Gmail — y una búsqueda posterior de `in:spam` devuelve
**completamente vacío**, lo cual no es que el spam esté vacío: es que el conector
no puede verlo. Concluí "no entrega nada" desde una herramienta que era ciega
justo donde estaba la respuesta. Cuatro sondas coincidentes no son cuatro
pruebas cuando las cuatro miran por la misma ventana tapiada.

**Diagnóstico correcto:** el remitente por defecto de Identity Platform entrega,
y Gmail lo clasifica como spam. Es un problema de **reputación del remitente**,
no de entrega.

**Qué NO cambia:** ninguna de las tres decisiones. Las 24 horas siguen sin poder
salir de Firebase (1 h, sin campo configurable — verificado dos veces); el estado
`invited` y el botón que se esconde siguen necesitando dominio nuestro; y el
`200` sigue sin distinguir entregado de perdido. El enfoque B se sostiene.

**Qué SÍ cambia, y es lo que estaba mal enfocado:** el enemigo es el spam, no el
silencio. Eso vuelve la alineación del remitente un requisito, no un adorno:
- El servidor del colegio con su SPF (H4) es mejor que Firebase **justo en este
  eje**, que es el único que importaba.
- `_dmarc.ceglorca.com` está en `p=none` — el dominio del colegio es suplantable
  y no pide nada a los buzones receptores. Sube de "hito de handover" a **tarea
  con consecuencia medible**: sin DKIM+DMARC, nuestro correo repetirá el destino
  del de Google.
- Cualquier relé intermedio (Gmail con contraseña de aplicación) manda desde un
  `@gmail.com`, no desde el colegio. Sirve para probar; no para la entrega final.

**Lección de método, anotada porque volverá a pasar:** una herramienta que
devuelve "nada" y una que devuelve "no puedo ver" se leen igual desde fuera. Antes
de convertir un vacío en un hallazgo, hay que comprobar que la herramienta puede
ver algo — un `in:spam` que devuelve cero en cualquier buzón real era la señal, y
la pasé por alto.

---

### ~~H1 (versión original, INCORRECTA)~~ — El proveedor acepta el envío y no entrega nada

Tres sondas contra `accounts:sendOobCode` con la llave web de `dev.tfvars`:

| Sonda | Respuesta | ¿Llegó el correo? |
|---|---|---|
| Cuenta creada **sin** contraseña (igual que `InviteUser`) | `HTTP 200` | **No** |
| Cuenta creada **con** contraseña | `HTTP 200` | **No** |
| Dirección que **no existe** en el proyecto | `HTTP 200` | **No** |
| Cuenta real del dueño, con contraseña y con logins previos | `HTTP 200` | **No** |

Buzón revisado con búsqueda `in:anywhere` (incluye spam y papelera): **cero correos**
del proyecto. El remitente por defecto de Identity Platform no está entregando.

### H2 — El `HTTP 200` no significa nada, y por eso el error nunca se puede ver.

`emailPrivacyConfig.enableImprovedEmailPrivacy: true` en la config viva del proyecto
(protección contra enumeración de correos). Con eso encendido, `sendOobCode` responde
`200` **incluso para una dirección inexistente** — la tercera fila de arriba lo prueba.

Consecuencia dura: el mensaje de `admin.go:120` ("la cuenta se creó pero no se pudo
enviar la invitación") **es inalcanzable**. El director siempre ve *"Invitación enviada"*
aunque no salga nada. El sistema no puede distinguir enviado de perdido — y hoy, en
producción, no envía.

### H3 — Los 24 horas no se pueden pedir prestados al proveedor.

El enlace de restablecimiento de Firebase trae su propia vigencia (del orden de **1
hora**) y la config viva del proyecto no expone ningún campo para cambiarla. Si la
vigencia la fija Google, **no es la que pidió el dueño**. Para que "24 horas" sea una
promesa nuestra, el token tiene que ser nuestro.

### H4 — El colegio YA tiene un servidor de correo propio que sirve. Verificado.

`ceglorca.com` no está en Google Workspace ni en Microsoft: su MX apunta a
`mail.ceglorca.com` (LucusHost, `server2471w.entornodevhost.com`), y su SPF ya autoriza a
ese host. Sonda en vivo al puerto 587:

```
250-server2471w.entornodevhost.com Hello probe
250-SIZE 52428800
250-LIMITS MAILMAX=1000 RCPTMAX=50000
250-AUTH PLAIN LOGIN
```

STARTTLS y `AUTH PLAIN/LOGIN` funcionando, con un límite de 1000 mensajes por sesión.
Consecuencias, y son grandes:

- **No hace falta acceso al DNS.** El SPF del colegio ya cubre a ese servidor, así que un
  correo enviado desde `no-reply@ceglorca.com` sale alineado desde el primer día. La
  suposición de que hacía falta el DNS (y que por eso esto estaba bloqueado) es falsa.
- **No hace falta contratar a nadie.** Ni Resend (3.000/mes, tope de 100 al día — que un
  alta masiva de tutores rompería), ni SendGrid (ya no tiene tramo gratuito permanente),
  ni SES (pide verificación de dominio y salir del sandbox).
- Lo único que necesitamos es **una credencial**: un buzón `no-reply@ceglorca.com` con su
  contraseña. Es una petición al colegio, no una obra.
- Cloud Run permite salida por 587 (bloquea el 25). El camino está abierto.

### Corolario

Los tres requisitos del dueño (llega, 24 h, y "sólo mientras no esté activo") apuntan al
mismo sitio: **la invitación deja de ser un favor que le pedimos a Google y pasa a ser un
objeto de nuestro dominio**, con su token, su vencimiento, su estado y su correo enviado
por el servidor del propio colegio, que sí nos dice si lo aceptó.

## Propuesta

### A. La invitación como objeto del dominio (backend, `iam`)

- Tabla `user_invitations` (migración nueva, con RLS): `id`, `user_id`, `token_hash`
  (sha256; el token en claro sólo viaja en el correo), `expires_at`, `accepted_at`,
  `sent_at`, `sent_count`. Índice único parcial: una invitación viva por usuario.
- Estado del usuario: tercer valor `invited` en `users.status` (o `activated_at`
  nullable — se decide en la fase de ingeniería). *"Todavía no está activo"* = invitación
  emitida y nunca aceptada.
- `InviteUser`: crea la cuenta en el proveedor **sin contraseña**, crea la invitación con
  `expires_at = now() + 24h`, y manda el correo **nuestro**.
- `AcceptInvitation(token, password)`: valida token vivo → `UpdateUser` en el proveedor
  (fija contraseña + `emailVerified: true`) → marca `accepted_at` → el usuario pasa a
  `active`. Idempotente y de un solo uso.
- `ResendInvitation`: invalida el token anterior, emite uno nuevo con 24 h frescas,
  reenvía. Rechaza si el usuario ya está activo (el backend es la frontera, no el botón).
- `RequestPasswordReset(email)`: misma maquinaria, respuesta **siempre 204** (no filtra si
  la dirección existe). Vigencia propia y corta.

### B. El correo lo mandamos nosotros

SMTP del propio colegio (`mail.ceglorca.com:587`, STARTTLS, ver H4), detrás de un puerto
`ports.Mailer` para que sea reemplazable y testeable sin red. Cero dependencias nuevas:
`net/smtp` de la biblioteca estándar basta. Plantillas en español, remitente
`no-reply@ceglorca.com`, con el nombre del colegio. El envío devuelve error de verdad
cuando falla, y `sent_at` se escribe sólo si el servidor aceptó el mensaje.

### C. Pantalla propia para poner la contraseña

`/activar?token=…` en `apps/web`: valida el token contra el gateway, pide contraseña dos
veces, aplica la política, y al terminar deja al usuario en el login (o dentro).
Un token vencido enseña un mensaje que se entiende y un "pídele al director que te la
reenvíe" — no una página en inglés de `firebaseapp.com`.

### D. El botón, donde toca

- "Reenviar invitación" **sólo** si el usuario no está activo. Desaparece al activarse.
- Estado visible en la tabla de Usuarios: *Invitado · vence en Xh* / *Activo* / *Vencida*.
- "¿Olvidaste tu contraseña?" deja de ser `href="#"`: pide el correo y llama a
  `POST /api/v1/auth/password-reset`, con el mismo mensaje pase lo que pase.

## Lo que NO entra

- MFA, login social, SMS.
- Cambiar el modelo de permisos ni la RLS existente.
- Tocar el DNS de `ceglorca.com` — resulta que no hace falta (H4).
- Migrar los usuarios que ya tienen contraseña.

## Riesgos conocidos de entrada

1. **La demo es diaria.** Todo esto toca la puerta por la que entra todo el mundo.
2. **Deriva de configuración detectada de paso:** terraform declara
   `client.permissions.disabled_user_signup = true` pero la config viva del proyecto
   trae `"permissions": {}`. Si de verdad está apagado, cualquiera con la llave pública
   puede crear cuentas en el proyecto. **Hay que verificarlo y cerrarlo.**
3. **Dependemos de una credencial que hoy no tenemos**: el buzón `no-reply@ceglorca.com`.
   Hay que pedírsela al colegio antes de que esto pueda funcionar en dev o en la demo.
4. El SMTP del colegio es alojamiento compartido: tiene topes por hora y su reputación es
   compartida con el resto del hosting. Suficiente para invitaciones (decenas), no para
   un envío masivo a 400 familias sin medirlo antes.

---

# REVISIÓN CEO — Step 0 (modo: SELECTIVE EXPANSION)

Voces: Codex **no disponible** (binario ausente) → `[subagent-only]` en las cuatro fases.

## 0A. Reto a las premisas

| # | Premisa del plan | Veredicto | Evidencia |
|---|---|---|---|
| P1 | "El correo no llega" | **CIERTA y peor de lo que parece** | 4 sondas en vivo: `sendOobCode` responde `200` y no entrega **nada**, ni siquiera a una cuenta real con contraseña y logins previos. Buzón revisado `in:anywhere`. |
| P2 | "Se integró con Firebase, en teoría ya podríamos atacar esto" | **PARCIALMENTE FALSA** | Firebase resolvió *guardar* la contraseña y *verificar* el token. **No** resolvió el correo: su remitente por defecto es exactamente la pieza que falla, y Google lo documenta como no apto para producción. El problema no se ataca *con* Firebase; se ataca **quitándole a Firebase el envío**. |
| P3 | "El botón de olvidé mi contraseña no funciona" | **CIERTA, y es literal** | `Login.tsx:193-197` es `href="#"` con `preventDefault()`. Nunca estuvo conectado a nada. |
| P4 | "Falta el botón de reenviar" | **FALSA — ya existe, mal puesto** | `AdminScreen.tsx:173` lo pinta en **todos** los usuarios, activos incluidos. Lo que falta no es el botón: es el **estado** que decide cuándo mostrarlo. |
| P5 | "La invitación vencerá a las 24 horas" | **NO SE PUEDE con el enlace de Google** | El OOB de Firebase trae su propia vigencia (~1 h) y la config viva del proyecto no expone ningún campo para cambiarla. 24 h exige token propio. |
| P6 (implícita) | "Esto es un arreglo de front" | **FALSA** | Ninguno de los tres síntomas se arregla en el front. Los tres nacen del mismo hueco: **no existe el objeto «invitación»**. |

**¿Es el problema correcto?** Sí, y el dueño lo nombró mejor de lo que el plan original lo tenía: los tres puntos son un solo problema. Reformulación que rinde 10x: no son "tres bugs", es **una pieza de dominio que falta**. Con ella, los tres desaparecen y encima aparece el reset de administrador que el `iam` nunca tuvo (`iam-has-no-admin-password-reset`, lección observada 2026-09-03).

**¿Qué pasa si no hacemos nada?** Dolor real, hoy: dos usuarios del colegio (`pagos@ceglorca.com`, `docentes.cegl@gmail.com`) existen en el proveedor **sin contraseña y sin poder ponérsela**. Están creados el 7-sep y siguen inertes. La demo es diaria; cada alta nueva produce otro usuario fantasma.

## 0B. Lo que ya existe (mapa sub-problema → código)

| Sub-problema | ¿Existe? | Dónde | Qué falta |
|---|---|---|---|
| Crear cuenta sin contraseña | **sí, completo** | `googleid/provisioner.go:CreateAccount` | nada |
| Pedir el envío del enlace | sí, pero inútil | `googleid/invite.go:SendInvitation` | el proveedor no entrega; y el `200` miente |
| RPC + REST de invitar / reenviar | **sí, completo** | `iam.proto:87,98` · `gateway/cmd/main.go:622-623` | sólo cambia el cuerpo del caso de uso |
| Guard de subconjunto de permisos al invitar | **sí** | `gateway/internal/handler/admin.go:213-218` | nada |
| Botón de reenviar | sí, sin condición | `AdminScreen.tsx:173` | condicionarlo al estado |
| Puerto de correo (`Mailer`) | **no** | — | crear puerto + adaptador SMTP |
| Tabla de invitaciones | **no** | — | migración + RLS |
| Estado "invitado" | **no** | `migrations/iam/0001_init.sql:19` | tercer estado o `activated_at` |
| Pantalla para poner contraseña | **no** | sólo `/login` y `/` | ruta `/activar` |
| Enlace "olvidé mi contraseña" | esqueleto muerto | `Login.tsx:193` | conectarlo |

**¿Reconstruye algo?** No. Reutiliza el 70% de la tubería (proto, gateway, guards, adaptador del proveedor) y sustituye **una sola pieza**: quién manda el correo y quién es dueño del plazo.

## 0C. Estado soñado

```
  HOY                              ESTE PLAN                      IDEAL A 12 MESES
  ─────────────────────────        ─────────────────────────      ─────────────────────────
  El director crea un usuario  →   El director crea un usuario →  El director crea un usuario
  Google dice "200"                Nosotros mandamos el correo    y el sistema le enseña
  No llega nada                    por el SMTP del colegio        "entregado 12:04" al lado
  El director no se entera         Falla ruidosa si no sale       del nombre
                                                                   
  No hay estado "invitado"     →   invited → active, con      →   Ciclo de vida completo:
  El botón sale siempre            vencimiento a 24 h y           invitado / activo / suspendido
                                   reenvío que lo renueva         / dado de baja, con bitácora
                                                                   
  Olvidé mi contraseña = "#"   →   Flujo real, sin filtrar    →   Autoservicio total: el docente
  El director no puede ayudar      si el correo existe            se rescata solo, el director
                                                                   sólo mira
                                                                   
  El correo es de Google,      →   El correo es del colegio,  →   Plantillas del colegio para
  en inglés, y no llega            en español, y llega            invitación, recibo y aviso
```

**Delta:** este plan mueve el sistema de "dependemos de un remitente que no entrega y no podemos saberlo" a "el colegio manda sus propios correos y el fallo se ve". Va **hacia** el ideal, y además desbloquea el resto: el mismo `Mailer` sirve luego para recibos de pago y avisos a familias, que hoy sólo tienen push.

## 0C-bis. Alternativas de implementación

```
APPROACH A: Arreglar el remitente en el proveedor (mínimo viable)
  Resumen: configurar CUSTOM_SMTP en Identity Platform con el correo del colegio,
           traducir las plantillas al español, conectar el botón de olvidé mi
           contraseña al SDK de Firebase, y añadir el estado `invited` para
           condicionar el botón de reenvío.
  Esfuerzo: S/M   (humano ~2 d / CC ~1 h)
  Riesgo:   Medio
  Pros:   - No añade tabla, ni puerto, ni pantalla nueva.
          - Se apoya en terraform que ya existe.
          - El envío sigue siendo problema de Google (menos código nuestro).
  Contras:- **NO cumple las 24 h**: la vigencia la sigue fijando el proveedor (~1 h).
          - Seguimos ciegos: con la protección de enumeración encendida, el `200`
            no distingue entregado de perdido. El bug de hoy podría repetirse
            sin que nadie lo note.
          - La página donde el usuario pone la contraseña sigue siendo la de
            `firebaseapp.com`, en inglés y sin el colegio por ningún lado.
  Reusa:  identity_platform.tf, googleid/*, proto y rutas actuales.

APPROACH B: La invitación es un objeto nuestro (arquitectura ideal)  ← RECOMENDADA
  Resumen: `user_invitations` (token con hash, vencimiento, aceptación) + puerto
           `Mailer` con adaptador SMTP + pantalla `/activar` propia + estado
           `invited`. Firebase se queda con lo que hace bien: guardar la
           contraseña y verificar el token.
  Esfuerzo: M/L   (humano ~5 d / CC ~2.5 h)
  Riesgo:   Medio
  Pros:   - Cumple **los tres** requisitos tal como se pidieron, 24 h incluidas.
          - El fallo de envío es un error de verdad: se ve, se registra, se reintenta.
          - El correo sale del colegio, en español, y el usuario aterriza en
            nuestra pantalla, no en una de Google en inglés.
          - El `Mailer` queda disponible para recibos y avisos (hoy sólo hay push).
          - Deja de haber una dependencia de un remitente que ya demostró fallar.
  Contras:- Es la más grande: migración, puerto, adaptador, pantalla y ruta nueva.
          - Un token propio es criptografía de la que somos responsables
            (hash, un solo uso, tiempo constante).
          - Depende de una credencial SMTP que hoy no tenemos en la mano.
  Reusa:  proto y rutas de invite/resend, guard de subconjunto, Provisioner,
          pkg/authctx para la RLS, patrón de migración versionada.

APPROACH C: Enlace de Google, correo nuestro (híbrido)
  Resumen: generar el enlace con el Admin SDK (`GeneratePasswordResetLink`) y
           mandarlo nosotros por SMTP del colegio, con estado `invited`.
  Esfuerzo: M     (humano ~3 d / CC ~1.5 h)
  Riesgo:   Medio
  Pros:   - Arregla la entrega (que es el bug que duele hoy) sin token propio.
          - El correo ya es nuestro: español, remitente del colegio, marca.
  Contras:- **Tampoco cumple las 24 h** — la vigencia sigue siendo la de Google.
          - La pantalla de poner contraseña sigue siendo la de Firebase.
          - Deja la mitad del trabajo hecho: cuando se pidan las 24 h en serio,
            hay que volver a pasar por aquí.
  Reusa:  lo mismo que B menos la tabla y la pantalla.
```

**RECOMENDACIÓN: B.** Es la única que cumple los tres requisitos tal como los pidió el dueño, y la única que hace visible el fallo que hoy es invisible. A y C dejan las 24 h sin cumplir, que es un requisito explícito, no un adorno. (P1 completitud; P5 explícito sobre listo — un token propio con vencimiento se lee en treinta segundos, la vigencia heredada de Google no se lee en ninguna parte.)

## 0D. Análisis del modo (SELECTIVE EXPANSION)

**Complexity check.** El plan toca ~14 archivos y añade 1 tabla, 1 puerto, 1 adaptador, 1 pantalla. Por encima del umbral de 8, así que hay que justificarlo: no es una abstracción prematura, es **la pieza que faltaba**. Cada uno de los tres síntomas del dueño sale del mismo hueco. El mínimo real que cumple lo pedido es exactamente esto; recortar cualquiera de las cuatro piezas incumple un requisito nombrado.

**Mínimo que logra el objetivo.** Tabla + estado + mailer + `/activar`. Diferible sin bloquear: reintentos automáticos de envío, bitácora de entregas, plantillas para otros correos.

**Escaneo de expansión (candidatos, no alcance todavía):**

| # | Oportunidad | Esfuerzo | Riesgo | Decisión auto |
|---|---|---|---|---|
| E1 | Rescatar a los 2 usuarios ya creados sin contraseña (`pagos@`, `docentes.cegl@`) con una invitación nueva | S (CC ~10 min) | Bajo | **DENTRO** (P2: está en el radio, y son usuarios reales bloqueados hoy) |
| E2 | El `Mailer` como pieza compartida, no privada de `iam` | S (CC ~15 min) | Bajo | **DENTRO** (P2 + P4: si nace privado se duplica en cuanto finance mande recibos) |
| E3 | Estado visible en la tabla de Usuarios: *Invitado · vence en 6 h* / *Vencida* | S (CC ~20 min) | Bajo | **DENTRO** (P1: el dueño pidió "que no salga si está activo"; para eso hay que **enseñar** el estado) |
| E4 | Cerrar la deriva de `disabled_user_signup` (terraform lo declara, la config viva no lo trae) | S (CC ~15 min) | Bajo | **DENTRO** (P2: seguridad, en el radio, y lo encontré verificando esto) |
| E5 | Bitácora de envíos (`sent_at`, `sent_count`, último error) visible al director | M | Bajo | **DENTRO** (P1: sin esto volvemos a la ceguera de hoy; es el requisito no dicho detrás de "no llega") |
| E6 | Reintento automático con backoff si el SMTP falla | M | Medio | **FUERA → TODOS** (P3: el reenvío manual ya lo cubre; automatizarlo antes de tener datos es adivinar) |
| E7 | Plantillas de correo para recibos de pago y avisos a familias | L | Medio | **FUERA → TODOS** (P3: otro módulo, otro PR) |
| E8 | MFA para el director | L | Medio | **FUERA → TODOS** (fuera del radio) |

## 0E. Interrogatorio temporal

- **HORA 1 (el director invita a una maestra).** Se crea la cuenta, se emite el token, sale el correo del colegio, el director ve *"Invitación enviada · vence 09-sep 12:40"*. Si el SMTP rechaza, ve **"No se pudo enviar"** con el motivo, y el usuario queda marcado `Invitado · sin enviar`.
- **HORA 2 (la maestra abre el correo).** Aterriza en `/activar?token=…` del colegio, en español. Pone su contraseña dos veces. Al terminar, entra.
- **HORA 6 (otra maestra abre el correo del día anterior).** Token vencido. Pantalla clara: *"Esta invitación venció. Pídele al director que te la reenvíe."* Sin jerga, sin `firebaseapp.com`.
- **HORA 6+ (el director reenvía).** Botón visible **sólo** en ella. Un clic, token nuevo, 24 h nuevas, el anterior muere. Cuando ella entra, el botón desaparece de la lista.
- **DÍA 30.** Alguien olvida su contraseña un domingo. Se rescata sola desde el login. El director no se entera y eso es lo bueno.
- **MES 6.** El colegio quiere mandar recibos por correo. El `Mailer` ya está y sólo hay que escribir la plantilla.

## 0F. Selección de modo

**SELECTIVE EXPANSION**, confirmado (impuesto por /autoplan). Alcance base = lo que pidió el dueño; E1–E5 aceptadas al alcance por las reglas P1/P2/P4; E6–E8 a TODOS.

---

# REVISIÓN CEO — Secciones 1-11

### H5 (hallazgo nuevo, encontrado revisando la Sección 2) — **nadie puede cambiar su contraseña, punto**

`services/iam/internal/usecase/auth.go:245` — con un proveedor configurado, `ChangePassword`
**se niega siempre**:

```go
if s.idTokens != nil {
    return glerrors.New(glerrors.KindInternal,
        "passwords are managed by the identity provider; use its password reset")
}
```

El comentario dice que la rotación vive en el proveedor y que "el enlace de invitación es
también el de reset". Ese enlace es justo el que no llega. Resultado: hoy, en dev, **no
existe ningún camino para cambiar una contraseña** — ni desde dentro (rechazado), ni desde
fuera (no llega el correo). El dueño reportó tres síntomas; son cuatro. `AcceptInvitation`
lo cierra: pasa a ser el único camino, y funciona.

## Sección 1 — Arquitectura

### Grafo de dependencias (antes → después)

```
ANTES                                   DESPUÉS
─────                                   ───────
web ──► gateway ──► iam ──► postgres    web ──► gateway ──► iam ──► postgres
                     │                                       │        │
                     └──► Identity Platform                   │        └─ user_invitations (RLS)
                            ├─ Admin SDK  (crear)             │
                            └─ sendOobCode (MANDAR) ✗ roto    ├──► Identity Platform
                                                              │      ├─ Admin SDK crear
                                                              │      └─ Admin SDK UpdateUser (fijar contraseña)
                                                              └──► SMTP del colegio (mandar)  ← NUEVO
```

El acoplamiento nuevo es `iam → SMTP`. Es justificable: `iam` es el dueño del ciclo de vida
de la cuenta, y el correo de invitación **es** parte de ese ciclo. Se declara como puerto
(`ports.Mailer`) definido en el consumidor, igual que `IdentityProvisioner`, así que el
dominio no se entera y los tests no tocan la red.

Se **elimina** el acoplamiento `iam → identitytoolkit REST` (el `Inviter`): deja de usarse.

### Máquina de estados de la invitación

```
                    InviteUser
                        │
                        ▼
   ┌──────────────► PENDIENTE ─────── expira 24 h ──────► VENCIDA
   │                 │      │                                │
   │                 │      └── AcceptInvitation ──► ACEPTADA│
   │                 │                                  │    │
   └── ResendInvitation (revoca la anterior) ◄───────────┼────┘
                                                         │
                            (usuario pasa a `active`) ◄──┘

  TRANSICIONES IMPOSIBLES, y qué las impide:
  ACEPTADA → PENDIENTE     · `accepted_at` NOT NULL en el WHERE de aceptación
  ACEPTADA → reenvío        · `ResendInvitation` rechaza si el usuario está `active`
  PENDIENTE ×2 a la vez     · índice único parcial: UNIQUE (user_id) WHERE accepted_at IS NULL
                              — reenviar es UPDATE del token, no INSERT de una fila nueva
  VENCIDA → ACEPTADA        · `expires_at > now()` en el mismo WHERE, en una sola sentencia
```

### Los cuatro caminos del flujo nuevo (`AcceptInvitation`)

```
  FELIZ:   token ─► hash ─► SELECT vivo ─► UpdateUser(proveedor) ─► UPDATE accepted_at
                                                                  ─► users.status = active ─► 204

  NULO:    token ausente en la URL  ─► el gateway responde 400 antes de tocar iam
           (la pantalla /activar sin ?token= enseña "enlace incompleto", no un formulario)

  VACÍO:   token = ""               ─► mismo 400. NUNCA llega al SELECT: un hash de ""
                                       es un hash válido y podría empatar con una fila
                                       corrupta. Se rechaza por longitud antes de hashear.

  ERROR:   el proveedor falla en UpdateUser ─► NO se marca accepted_at, NO se activa
           el usuario, se devuelve 503 y el token SIGUE VIVO para reintentar.
           ORDEN OBLIGATORIO: proveedor primero, base después. Al revés dejaría una
           invitación consumida y una cuenta sin contraseña — el peor de los dos mundos.
```

### Escala

400 alumnos, ~20 docentes, ~1.500 personas con las familias. Invitaciones: decenas por
semana. Lo primero que se rompe a 100x no es la base (un índice sobre `token_hash` resuelve
cualquier volumen) sino el **SMTP compartido del colegio**: alojamiento con topes por hora.
Un alta masiva de 400 tutores en una tarde es el escenario que hay que medir antes de
intentarlo, no después.

### Punto único de fallo

El servidor del colegio. Mitigación real y suficiente: **el fallo es ruidoso** (el director
lo ve en el momento) y **el reenvío es un botón**. No se propone reintento automático (E6,
diferido): sin datos de cómo falla, un backoff es adivinar.

### Reversión

Reversible en tres pasos, ninguno destructivo:
1. La migración es aditiva (tabla nueva + un valor más en el CHECK). No se borra nada.
2. `git revert` del código deja la tabla huérfana, inofensiva.
3. Las filas con `status='invited'` sí romperían el código viejo (`ParseStatus` no lo
   conoce) — por eso la migración va **antes** del despliegue y el revert exige un
   `UPDATE users SET status='active' WHERE status='invited'`. **Hay que escribirlo en el
   runbook**, no descubrirlo en caliente.

**Hallazgos S1:**

| # | Hallazgo | Sev | Decisión |
|---|---|---|---|
| A1 | El endpoint de aceptar es **anónimo** y las tablas de `iam` son `FORCE ROW LEVEL SECURITY` filtrando por `app.current_tenant`. Sin sesión no hay tenant. | Alta | **Resuelto sin código nuevo:** `AdminService` ya recibe `tenantID` por configuración (`admin.go:29`, single-tenant). El caso de uso lo enlaza con `authctx.WithTenantContext` igual que hoy. Se documenta explícitamente para que nadie lo "arregle" pasando el tenant por la URL. |
| A2 | El revert deja filas `invited` que el código viejo no sabe leer | Media | Runbook con el `UPDATE` de vuelta. P5 explícito. |
| A3 | SMTP compartido = SPOF con tope por hora | Media | Fallo ruidoso + reenvío manual. Envío masivo, fuera de alcance (E7). |

## Sección 2 — Mapa de errores y rescate

```
  MÉTODO / CAMINO              | QUÉ PUEDE FALLAR                  | ERROR (Kind)
  -----------------------------|-----------------------------------|------------------
  InviteUser                   | correo mal formado                | KindInvalid
                               | la cuenta ya existe en el proveedor| KindConflict
                               | email duplicado en users          | KindConflict
                               | rol inexistente                   | KindInvalid
                               | SMTP no autentica                 | KindInternal
                               | SMTP rechaza el destinatario      | KindInvalid
                               | SMTP agota el tiempo              | KindUnavailable
  AcceptInvitation             | token ausente / vacío             | KindInvalid
                               | token desconocido                 | KindNotFound (genérico)
                               | token vencido                     | KindNotFound (genérico)
                               | token ya usado                    | KindNotFound (genérico)
                               | contraseña demasiado corta        | KindInvalid
                               | el proveedor rechaza UpdateUser   | KindUnavailable
                               | dos aceptaciones simultáneas      | la 2ª: KindNotFound
  ResendInvitation             | el usuario ya está activo         | KindInvalid
                               | usuario inexistente               | KindNotFound
                               | SMTP falla                        | igual que InviteUser
  RequestPasswordReset         | el correo no existe               | (ninguno — 204 igual)
                               | SMTP falla                        | 204 igual, log ERROR

  ERROR                    | RESCATADO | ACCIÓN                        | QUÉ VE EL USUARIO
  -------------------------|-----------|-------------------------------|--------------------------------
  KindInvalid (correo)     | Sí        | rechazo antes de escribir     | "Ese correo no es válido"
  KindConflict             | Sí        | rechazo, nada creado          | "Ese correo ya tiene cuenta"
  SMTP no autentica        | Sí        | usuario creado, invitación    | "Se creó la cuenta pero NO se
                           |           | marcada `sin enviar`, log     |  pudo enviar el correo:
                           |           | ERROR con el código SMTP      |  <motivo>. Usa Reenviar."
  SMTP timeout             | Sí        | idem                          | idem
  token desconocido /      | Sí        | UNA sola respuesta para los   | "Esta invitación ya no es
   vencido / usado         |           | tres (no se distingue)        |  válida. Pídele al director
                           |           |                               |  que te la reenvíe."
  contraseña corta         | Sí        | no toca el proveedor          | "Mínimo N caracteres"
  proveedor caído          | Sí        | token INTACTO, 503            | "No pudimos guardar tu
                           |           |                               |  contraseña. Vuelve a
                           |           |                               |  intentarlo en un minuto."
  2ª aceptación simultánea | Sí        | UPDATE ... WHERE accepted_at  | "Esta invitación ya se usó"
                           |           | IS NULL → 0 filas → NotFound  |
  reset de correo inexistente| Sí      | log INFO, sin correo          | "Si esa dirección tiene
                           |           |                               |  cuenta, te llegará un enlace"
```

**Reglas que este mapa impone al código:**

- Nada de `catch` genérico. El front actual ya peca de eso: `AdminScreen.tsx:111` hace
  `try { … } catch { say("No se pudo enviar el enlace") }`, que convierte una sesión
  caducada en un mensaje sobre el correo. Es exactamente la lección
  `blanket-catch-reports-dead-session-as-domain-error` de este mismo repo. **Se corrige
  en este PR**: 401 → "tu sesión caducó", 4xx/5xx → el motivo del gateway.
- El error de SMTP se registra con contexto (a quién, qué código devolvió el servidor)
  **sin el token y sin la contraseña**, nunca.
- Los tres fallos de token dan **una sola** respuesta hacia fuera: distinguirlos sería un
  oráculo para adivinar tokens.

**Hallazgos S2:** 3 GAPs, los tres cerrados arriba (SMTP sin rescate → mensaje con motivo +
estado `sin enviar`; proveedor caído en aceptación → token intacto; `catch` genérico en el
front → mensajes por clase de error).

## Sección 3 — Seguridad y modelo de amenazas

| # | Amenaza | Prob. | Impacto | ¿Mitigada? |
|---|---|---|---|---|
| T1 | Adivinar un token de invitación → **toma de cuenta con permisos de docente/director** | Baja | **Crítico** | Sí: 32 bytes de `crypto/rand` (256 bits), base64url. Inadivinable por construcción. |
| T2 | Robo del token desde la base (backup, log, dump) | Media | Crítico | Sí: sólo se guarda `sha256(token)`. El claro existe únicamente dentro del correo. |
| T3 | Comparación no constante → filtrado por tiempo | Baja | Alto | Sí: se busca **por el hash** (índice), no se compara byte a byte el claro. |
| T4 | Reutilizar un token ya usado | Media | Crítico | Sí: `UPDATE … WHERE accepted_at IS NULL AND expires_at > now()` en **una** sentencia; 0 filas = rechazo. |
| T5 | Enumeración de correos en `/auth/password-reset` | Alta | Medio | Sí: 204 siempre, mismo texto, y el trabajo se hace igual (sin atajo temprano que se note en el tiempo de respuesta). |
| T6 | Fuerza bruta sobre `/auth/password-reset` como cañón de correo hacia un tercero | Media | Medio | Sí: se reutiliza el `loginLimiter` por IP que ya frontea `/auth/login` (`gateway/cmd/main.go:218`). |
| T7 | El token viaja en la URL → queda en el historial y en el `Referer` | Alta | Alto | Parcial: token de un solo uso y 24 h. **Añadir `Referrer-Policy: no-referrer` en `/activar`** para que no se filtre a terceros. |
| T8 | Contraseña débil al activar | Alta | Alto | Sí: se aplica `minPasswordLen` del dominio antes de tocar al proveedor. |
| T9 | La credencial SMTP en claro (repo, imagen, log) | Media | Alto | Sí: Secret Manager + variable de entorno, nunca en `tfvars` (la llave web sí puede estar ahí, es pública por diseño; **esta no**). |
| T10 | `disabled_user_signup`: terraform lo declara, la config viva trae `"permissions": {}` | Media | **Alto** | **NO mitigada hoy.** Con la llave pública, cualquiera podría registrarse en el proyecto. Entra al alcance (E4): verificar y cerrar. |
| T11 | El correo de invitación reenviado por el usuario a un tercero | Baja | Alto | Aceptado: el correo lo controla su dueño. 24 h acota la ventana. |
| T12 | Inyección en la plantilla del correo (nombre del usuario) | Baja | Medio | Sí: plantilla de texto plano + HTML escapado con `html/template`, nunca concatenación. |

**Superficie nueva:** 3 endpoints sin sesión (`GET /auth/invitations/{token}` para validar,
`POST /auth/invitations/{token}/accept`, `POST /auth/password-reset`) y 1 con sesión
(`POST /users/{id}/invite`, que ya existe). Los tres anónimos van detrás del limitador por IP.

**Bitácora:** invitar, reenviar, aceptar y pedir reset dejan registro con actor, destinatario
y resultado. Hoy `iam` no tiene bitácora de autenticación (lección
`iam-has-no-admin-password-reset`); esto es el primer trozo de ella.

**Dependencias nuevas:** ninguna. `net/smtp` y `crypto/rand` son de la biblioteca estándar.
`govulncheck` en CI no cambia de superficie.

## Sección 4 — Flujo de datos y casos límite de interacción

```
  ENTRADA ──▶ VALIDACIÓN ──▶ HASH ──▶ SELECT+UPDATE ──▶ PROVEEDOR ──▶ SALIDA
     │            │            │            │               │            │
     ▼            ▼            ▼            ▼               ▼            ▼
  [sin token] [longitud]  [sin error]  [0 filas =      [timeout →   [204 sin
  → 400       [no vacío]  posible      vencido/usado    token vivo]  cuerpo]
              [charset]                 /desconocido]  [rechazo →
                                        → 404 genérico  KindInvalid]
```

| Interacción | Caso límite | ¿Cubierto? | Cómo |
|---|---|---|---|
| Formulario `/activar` | doble clic en Guardar | **Sí** | botón deshabilitado en vuelo + el 2º intento choca con `accepted_at IS NULL` |
| | el usuario se va a media petición | Sí | la operación es atómica en el servidor; al volver, el token ya está usado y ve "ya se usó" |
| | token vence mientras teclea | Sí | el rechazo llega al enviar, con el mensaje de reenvío, no un 500 |
| | pega el enlace en otro navegador | Sí | el token no depende de cookie ni de sesión |
| | contraseñas que no coinciden | Sí | validación en cliente **y** en servidor |
| Botón Reenviar | doble clic del director | Sí | el 2º UPDATE gana; sólo el último token vive. Dos correos, un token válido |
| | reenviar mientras el usuario está activando | Sí | el token viejo muere; si el usuario ya envió, ganó él (`accepted_at` se fija primero) |
| | reenviar a alguien ya activo | Sí | el backend rechaza con `KindInvalid`; el botón ni siquiera está |
| Tabla de Usuarios | 0 usuarios | Sí | estado vacío ya existe |
| | el estado cambia mientras miras | Parcial | `reload()` tras cada acción; **no** hay polling. Aceptado: el director actúa, no vigila |
| Login → olvidé | correo inexistente | Sí | mismo mensaje |
| | envío repetido | Sí | limitador por IP |

**Hallazgo S4:** la única laguna real es el token en la URL (T7) → cabecera
`Referrer-Policy: no-referrer` en la ruta `/activar`. Decidido: **dentro** (P2, una línea).

## Sección 5 — Calidad de código

- **DRY.** `InviteUser`, `ResendInvitation` y `RequestPasswordReset` comparten "emite token,
  guarda hash, manda correo". Una función privada `issueAndSend`, no tres copias. El repo ya
  aprendió esto con `gormTxRunner` (6 copias hoistadas a `pkg/authctx/gormtx`).
- **Código muerto.** `googleid/invite.go` (el `Inviter`) queda sin uso. **Se borra en este
  PR**, con su test. Dejarlo sería una trampa para el siguiente que busque "quién manda el
  correo" y encuentre dos respuestas.
- **Nombres.** `SendInvitation` en el puerto `IdentityProvisioner` deja de tener sentido: el
  proveedor ya no manda nada. El puerto queda `CreateAccount` + `SetPassword`. Nombrar por lo
  que hace, no por lo que hacía.
- **Sobre-ingeniería.** El puerto `Mailer` con un solo adaptador podría oler a abstracción
  prematura. No lo es: es la costura que permite testear sin red, y es exactamente el patrón
  que este repo ya usa para el proveedor de identidad. Un método (`Send(ctx, to, subject, html, text)`).
- **Complejidad.** Ninguna función nueva ramifica más de 3 veces si `issueAndSend` se
  extrae. Sin `else`: retorno temprano, como el resto del servicio.
- **Bajo-ingeniería.** El `catch {}` de `AdminScreen.tsx:111` (ya tratado en S2).

## Sección 6 — Revisión de pruebas

```
  FLUJOS DE UX NUEVOS
    · Director invita y ve el resultado real del envío
    · Director reenvía (sólo a no activos)
    · Usuario abre /activar y pone su contraseña
    · Usuario abre /activar con token vencido
    · Cualquiera pide "olvidé mi contraseña" desde el login

  FLUJOS DE DATOS NUEVOS
    · token en claro → sha256 → fila → correo
    · token del correo → fila viva → contraseña en el proveedor → usuario activo

  CAMINOS DE CÓDIGO NUEVOS
    · issueAndSend (invitar / reenviar / reset)
    · AcceptInvitation (6 ramas de rechazo, 1 de éxito)
    · ResendInvitation con guard de usuario activo
    · users.status = 'invited' y su lectura en ListUsers

  INTEGRACIONES NUEVAS
    · SMTP del colegio (STARTTLS + AUTH)
    · Admin SDK UpdateUser (fijar contraseña + emailVerified)

  CAMINOS DE ERROR NUEVOS  → los 10 de la tabla de la Sección 2
```

| Cosa | Tipo | Test feliz | Test de fallo | Test de borde |
|---|---|---|---|---|
| `issueAndSend` | unit (tabla) | emite, hashea, manda | mailer devuelve error → invitación queda `sin enviar` | correo con `+alias`, mayúsculas, 254 chars |
| `AcceptInvitation` | unit (tabla) | activa y marca | vencido / usado / desconocido → **mismo** error | contraseña de `minPasswordLen-1` |
| " | integration (testcontainers) | RLS: el token de otro tenant no se ve | dos aceptaciones concurrentes → sólo una gana | reloj: `expires_at` exactamente ahora |
| `ResendInvitation` | unit | renueva 24 h y mata el anterior | usuario activo → `KindInvalid` | usuario inexistente |
| Adaptador SMTP | integration | servidor SMTP falso en memoria acepta | AUTH rechazado, timeout, 5xx | asunto/acentos en UTF-8 (quoted-printable) |
| `/activar` | componente (Vitest/RTL) | pinta el formulario, envía | token inválido → mensaje de reenvío | sin `?token=` |
| `/activar` | **E2E navegador real** | ciclo completo | — | **obligatorio**: la lección `cmdk-ships-no-css` de este repo dice que jsdom no aplica CSS y 141 tests verdes escondían una pantalla invisible. Una pantalla nueva no se da por buena sin navegador. |
| Botón reenviar | componente | visible sólo si no activo | — | usuario activo → ausente (aserción negativa) |
| Contrato REST | contract test | los 3 endpoints contra `docs/api/openapi.yaml` | — | — |

**Test para dormir tranquilo un viernes a las 2 am:** el de integración concurrente — dos
aceptaciones del mismo token a la vez, una gana, la otra recibe el mismo error genérico que
un token inventado.

**Test que escribiría un QA hostil:** pedir 200 resets seguidos para el mismo correo y
comprobar que el limitador corta y que los tiempos de respuesta de "existe" y "no existe" no
se distinguen.

**Riesgo de flakiness:** el vencimiento depende del reloj. Se inyecta `now func() time.Time`
como ya se hace con `newID` en `AdminService` (`admin.go:32`). Nada de `time.Now()` suelto.

**Pirámide:** ~25 unit, ~5 integration, 1 E2E. Correcta.

## Sección 7 — Rendimiento

- **Índices:** `UNIQUE (token_hash)` (la búsqueda de aceptación) y el parcial
  `UNIQUE (user_id) WHERE accepted_at IS NULL`. Con eso, la aceptación es una sola lectura
  por índice.
- **N+1, el riesgo real:** `ListUsers` tiene que devolver el estado de invitación de cada
  usuario. Hecho ingenuamente son 1+N consultas. **Debe ser un `LEFT JOIN` a
  `user_invitations` en la misma consulta.** El repo ya tiene la regla ("Preload explícito,
  nada de N+1") y ya la ha roto antes.
- **Memoria:** una invitación por usuario. ~1.500 filas como techo. Irrelevante.
- **Latencia:** el SMTP síncrono mete 200-800 ms en la respuesta de "invitar". Aceptable
  para una acción explícita del director, y **preferible a asíncrono**: el director tiene
  que enterarse en el momento de si salió o no. Ese es todo el punto.
- **Pool de conexiones:** sin cambios (una conexión SMTP por envío, cerrada al terminar).

## Sección 8 — Observabilidad

| Qué | Cómo |
|---|---|
| Log | `iam.invitation.issued` / `.sent` / `.send_failed` / `.accepted` / `.expired_attempt`, estructurado, con `user_id` y **jamás** el token ni la contraseña |
| Métrica de que funciona | invitaciones aceptadas / invitaciones enviadas (por semana) |
| Métrica de que está roto | `send_failed > 0` en 15 min; aceptaciones = 0 con envíos > 0 durante 48 h |
| Visible para el director | columna de estado en Usuarios: *Invitado · vence 12:40* / *Sin enviar ⚠* / *Activo*. **Ésta es la observabilidad que importa**: el director es el operador |
| Depurar 3 semanas después | `user_invitations` guarda `sent_at`, `sent_count`, `last_error`, `accepted_at`. La fila cuenta la historia sola |
| Runbook | "no le llegó el correo": 1) mirar `last_error` en la fila, 2) probar el SMTP con `openssl s_client -starttls smtp`, 3) reenviar, 4) si el servidor del colegio está caído, avisar y esperar |

**Hallazgo S8:** hoy no hay ninguna de estas señales; por eso el bug lleva desde el 7-sep sin
detectarse. La observabilidad **no** es un extra en este plan: es la mitad del arreglo.

## Sección 9 — Despliegue

- **Migración:** aditiva (tabla nueva + `CHECK` ampliado). Sin bloqueos: `CREATE TABLE` y un
  `ALTER … DROP CONSTRAINT / ADD CONSTRAINT` sobre `users`, que en Postgres 13+ con
  `NOT VALID` + `VALIDATE` no toma bloqueo largo. Con 4 filas, irrelevante de todos modos.
- **Orden:** migrar → desplegar. Ya lo hace el pipeline desde `96670e26`.
- **Ventana de convivencia:** código viejo + esquema nuevo = inofensivo (nadie escribe
  `invited` hasta que el código nuevo corre). Código nuevo + esquema viejo = falla al
  insertar. Por eso el orden.
- **Secreto:** `SMTP_PASSWORD` en Secret Manager, inyectado como variable de entorno igual
  que `IAM_JWT_SECRET`. **Sin él, el servicio arranca igual** y las invitaciones se niegan
  con un mensaje que nombra la configuración que falta — el mismo patrón que ya usa
  `provisioner == nil`.
- **Flag:** **no** hace falta uno nuevo. Esto arregla un camino roto; no hay nada que
  esconder en una demo. El interruptor real es la presencia de la credencial SMTP.
- **Verificación post-despliegue (5 min):** invitar a una dirección propia, recibir el
  correo, activar, entrar. Cuatro pasos, manual, una vez.
- **Rollback:** revert + `UPDATE users SET status='active' WHERE status='invited'` (S1/A2).

## Sección 10 — Trayectoria

- **Deuda que se paga:** desaparece el `Inviter` roto, desaparece el `ChangePassword` que se
  niega siempre (H5), y aparece la bitácora de auth que faltaba.
- **Deuda que se crea:** una plantilla de correo en el código (no en un sistema de
  plantillas) y un adaptador SMTP síncrono sin reintentos. Ambas conscientes y anotadas.
- **Reversibilidad: 4/5.** Todo aditivo salvo el estado nuevo, que tiene su `UPDATE` de
  vuelta escrito.
- **Dependencia de camino:** ninguna mala. Si mañana el colegio quiere Resend o SES, se
  cambia el adaptador detrás del puerto `Mailer` y nada más se entera.
- **Potencial de plataforma: alto.** Hoy `comms` sólo sabe mandar push. Con `Mailer` en
  `pkg/`, los recibos de pago de `finance` y los avisos a familias tienen por dónde salir.
  Ésa es la razón de E2.
- **La pregunta del año:** un ingeniero nuevo lee `user_invitations` y entiende el ciclo
  completo en una tabla. Hoy tendría que deducirlo de tres servicios y un proveedor externo.

## Sección 11 — Diseño y UX

```
  ┌────────────┐  invita   ┌──────────────────────┐
  │ Usuarios   │──────────►│ Cajón: Invitar       │
  │ (director) │           │ correo · nombre · rol│
  └─────┬──────┘           └──────────┬───────────┘
        │                             │ éxito: "Invitación enviada · vence mañana 12:40"
        │                             │ fallo: "Se creó la cuenta pero el correo no salió: <motivo>"
        │◄────────────────────────────┘
        │
        │  fila del usuario:
        │  ┌──────────────────────────────────────────────────────┐
        │  │ Tania Cortés   docentes.cegl@…  [Invitado · 6 h]     │
        │  │                        [Reenviar invitación] [Suspender] │
        │  ├──────────────────────────────────────────────────────┤
        │  │ Celia Quiñones pagos@ceglorca…  [Activo]             │
        │  │                                        [Suspender]   │  ← sin Reenviar
        │  └──────────────────────────────────────────────────────┘

  correo ──► /activar?token=…
              ├─ token vivo    → [contraseña] [repetir] [Activar mi cuenta] → login
              ├─ token vencido → "Esta invitación venció. Pídele al director que te la
              │                   reenvíe." (sin formulario, sin jerga)
              └─ sin token     → "El enlace está incompleto. Ábrelo desde el correo."

  /login ──► "¿Olvidaste tu contraseña?" ──► [correo] [Enviar]
              → SIEMPRE: "Si esa dirección tiene cuenta, te llegará un enlace."
```

| Pantalla | Cargando | Vacío | Error | Éxito | Parcial |
|---|---|---|---|---|---|
| Cajón Invitar | botón "Enviando…" | — | motivo real del SMTP | toast con la fecha de vencimiento | cuenta creada, correo no → estado `Sin enviar ⚠` |
| Fila de usuario | esqueleto | ya existe | — | píldora de estado | *Vence en 2 h* en ámbar |
| `/activar` | "Guardando…" | — | 3 mensajes distintos, ninguno técnico | entra directo | — |
| Olvidé mi contraseña | "Enviando…" | — | mismo mensaje que el éxito | mensaje neutro | — |

**Jerarquía (qué ve primero el director):** el estado, no el botón. La píldora *Invitado ·
6 h* responde la pregunta que trae ("¿ya entró?"); el botón es la consecuencia.

**Riesgo de UI genérica:** el mayor es `/activar`. Una pantalla de "pon tu contraseña" es
donde más fácil se cuela algo que parece de plantilla. Debe usar el sistema de diseño del
proyecto (`.claude/docs/design/DESIGN-SYSTEM.md`, mandato vigente en memoria del proyecto):
tokens, `Icon`, `Btn`, la marca del colegio. **Es la primera pantalla que ve un docente
nuevo del sistema entero.** Merece cuidado, no un formulario suelto.

**Accesibilidad:** `<label>` reales en los dos campos, `autocomplete="new-password"`,
`aria-live` para el mensaje de error, contraste según los tokens, objetivos táctiles ≥44 px.
La píldora de estado no puede comunicar sólo por color (añade texto).

**Móvil:** el docente abre el correo en el teléfono. `/activar` **nace responsive**, no se
adapta después. Es el caso principal, no el secundario.

**Recomendación:** este plan tiene alcance de UI real (una pantalla nueva y dos cambios de
estado visible). La fase 2 de /autoplan lo revisa a fondo.

## Registro de modos de fallo (CEO)

| # | Modo de fallo | Prob. | Impacto | ¿Visible? | Respuesta |
|---|---|---|---|---|---|
| F1 | El SMTP del colegio rechaza la autenticación (contraseña rotada) | Media | Alto | **Sí** — mensaje al director + `last_error` en la fila | Renovar la credencial en Secret Manager, reenviar |
| F2 | El SMTP acepta el mensaje y el destinatario nunca lo ve (spam) | Media | Alto | **Parcial** — sabemos que salió, no que llegó | Reenviar; a medio plazo, DKIM propio si el colegio da acceso |
| F3 | Identity Platform rechaza `UpdateUser` al activar | Baja | Alto | Sí — 503 y el token sigue vivo | Reintentar; el usuario no pierde la invitación |
| F4 | Alguien acepta con el token dos veces a la vez | Baja | Bajo | Sí | Una gana, la otra ve "ya se usó" |
| F5 | Se despliega el código sin la credencial SMTP | Media | Alto | **Sí** — invitar se niega nombrando la variable que falta | Cargar el secreto; nada queda a medias |
| F6 | Filas `invited` tras un revert | Baja | Alto | No (silencioso) | `UPDATE users SET status='active' WHERE status='invited'` — **en el runbook** |
| F7 | Un alta masiva pasa el tope por hora del hosting | Baja hoy | Medio | Sí — errores por lote | No hacer altas masivas por ahora (E7 diferido) |
| F8 | `ListUsers` en N+1 al pintar el estado | Media si se hace mal | Medio | No | `LEFT JOIN` obligatorio, con test |
| F9 | El token queda en el historial del navegador / `Referer` | Alta | Medio | No | Un solo uso + 24 h + `Referrer-Policy: no-referrer` |
| F10 | La deriva de `disabled_user_signup` sigue abierta | — | Alto | No | E4: verificar y cerrar en este PR |

## NO entra en este plan (y por qué)

| Item | Por qué fuera | Dónde va |
|---|---|---|
| Reintento automático del envío con backoff (E6) | El reenvío manual ya cubre el caso; automatizar sin datos de cómo falla es adivinar | TODOS |
| Plantillas de correo para recibos y avisos (E7) | Otro módulo, otro PR. El puerto `Mailer` queda listo para ello | TODOS |
| MFA para el director (E8) | Fuera del radio de este cambio | TODOS |
| Envío masivo a las ~400 familias | El SMTP compartido tiene topes que hay que medir antes | TODOS |
| DKIM/dominio de remitente propio | El SPF del colegio ya alinea (H4); esto sólo mejora la reputación | TODOS |
| Login social, SMS | Aditivos, sin petición | — |
| Cambiar el modelo de permisos o la RLS | No se toca | — |
| Migrar a los usuarios que ya tienen contraseña | Siguen entrando igual | — |

---

# CEO — Voces independientes (Step 0.5)

**CODEX DICE (CEO — reto estratégico):** `[codex-unavailable: binary not found]`. Sin voz externa.

**CLAUDE SUBAGENT (CEO — independencia estratégica):** 10 hallazgos, sin contexto previo.
Verificó por su cuenta el DNS, `admin.go:99`, `AdminScreen.tsx:173` y la ausencia de
pantalla de activación. Resumen y qué se hizo con cada uno:

| # | Hallazgo de la voz | Sev | Veredicto tras verificarlo | Acción |
|---|---|---|---|---|
| V1 | `AcceptInvitation` necesita `firebaseauth.users.update` y el rol de runtime **no lo tiene** | Crítico | **CIERTO Y VERIFICADO.** `gcloud iam roles describe garciaLorcaIdentityRuntime` → `firebaseauth.users.create;firebaseauth.users.get`. B habría fallado en tiempo de ejecución, en dev, después de escribirlo todo. | **ACEPTADO.** Tarea de terraform explícita + decisión sobre la cuenta de servicio (abajo, D-A) |
| V2 | Existe una ruta de alta por contraseña temporal que desbloquea hoy | Crítico | **PARCIALMENTE FALSO.** Con `NEXT_PUBLIC_AUTH_PROVIDER=identity-platform` el login va al proveedor: un hash local no abre nada, y `CreateUser` ni siquiera crea cuenta en el proveedor. Esa ruta sólo funciona apagando la migración entera. **Pero la necesidad que señala es real** | **ACEPTADO EN SU FONDO, con mejor respuesta:** ver H6 |
| V3 | Las 24 h nunca se interrogaron, y los contras de C están inflados: Identity Platform **sí** permite URL de acción propia | Crítico | **CIERTO en lo de la URL** — el campo `notification.sendEmail.callbackUri` está en la config viva y es editable. **El de la vigencia queda intacto:** documentado, 1 h, no configurable | **VA AL GATE COMO USER CHALLENGE.** El delta B-sobre-C queda reducido a un número: 24 h contra 1 h |
| V4 | Contradicción: el `Mailer` se vende para "recibos a 400 familias" y el mismo plan dice que ese transporte no aguanta un envío masivo | Alto | **CIERTO.** | **ACEPTADO.** Se deja de justificar el `Mailer` con envío masivo |
| V5 | Token en la query string + **inyección CRLF de cabeceras** al componer MIME a mano con el `displayName` | Alto | **CIERTO, y el CRLF es un hallazgo nuevo** que no estaba en la Sección 3 | **ACEPTADO.** Ambos entran al alcance |
| V6 | La deriva de `disabled_user_signup` no debe esperar cinco días | Alto | **CIERTO.** | **ACEPTADO.** Sale de E4 y pasa a hotfix del día 1 |
| V7 | Ni flag, ni semántica de login para `invited`, ni ventana de despliegue | Alto | **Mitad y mitad.** La semántica de login para `invited` es un hueco real que no cubrí. El flag **se rechaza** con razón: esto no es un módulo nuevo, es la reparación de la puerta de entrada; un flag que esconde la reparación deja el sistema roto | **ACEPTADO** lo del login. **RECHAZADO** el flag, con motivo escrito |
| V8 | SMTP síncrono dentro del request contra hosting compartido; el outbox de `finance` ya existe | Medio-alto | **Cierto el riesgo, discutible el remedio.** Asíncrono le quita al director la única señal que hoy le falta: si salió o no | **PARCIAL:** se queda síncrono **con timeout duro de 10 s** + estado `sin enviar` |
| V9 | IP de salida dinámica de Cloud Run contra cPanel/CSF; y el limitador por IP no se nombra | Medio | **La IP dinámica es un riesgo nuevo y real.** El limitador sí estaba (T6), pero como amenaza, no como tarea | **ACEPTADO.** Riesgo documentado + el limitador pasa a ser tarea explícita |
| V10 | H2 sobre-argumentada; la causa raíz de H1 nunca se diagnosticó | Medio | **CIERTO en ambas.** `sendOobCode` sí devuelve no-200 por llave inválida o cuota; sólo el caso "destinatario inexistente" es indistinguible. Y cuatro sondas prueban el síntoma, no la causa | **ACEPTADO.** H2 corregida abajo; diagnóstico hecho → H6 |

### H2 — corregida

`sendOobCode` **sí** devuelve no-200 por llave inválida, cuota agotada o proveedor
deshabilitado. Lo que **no** puede distinguir, con la protección de enumeración encendida,
es *"lo mandé"* de *"esa dirección no existe"* y de *"lo acepté y no lo entregué"*. El
mensaje de `admin.go:120` no es inalcanzable; es **inalcanzable justo en el fallo que
estamos sufriendo**. Sigue siendo el problema, con la afirmación del tamaño correcto.

### H6 (nuevo, del diagnóstico que pidió la voz) — la maquinaria funciona; **sólo la entrega está rota**

Sonda con credenciales de administrador y `returnOobLink: true` sobre uno de los dos
usuarios muertos:

```json
{ "oobCode": "XZ-PHe8XQa0auLAwRTLthhb4m4MoXmUekO3wkEEVEUsAAAGgglWYzA",
  "email": "docentes.cegl@gmail.com",
  "oobLink": "https://garcia-lorca.firebaseapp.com/__/auth/action?mode=resetPassword&oobCode=…&apiKey=…&lang=en" }
```

Tres consecuencias, todas útiles:

1. **La causa queda acotada.** Generar el código, la cuenta y el enlace funciona. Lo único
   que falla es que Google **entregue** el mensaje. Ya no elegimos arquitectura desde un
   fallo sin diagnosticar: elegimos desde "el remitente por defecto no entrega", que es
   exactamente lo que Google documenta como no apto para producción.
2. **Desbloqueo hoy, sin depender de nadie.** Los dos usuarios muertos
   (`docentes.cegl@gmail.com`, `pagos@ceglorca.com`) se rescatan generando su enlace y
   pasándoselo al director por WhatsApp. Minutos, cero código, cero credenciales nuevas.
   Ésta es la respuesta a V2, y es mejor que la contraseña temporal: no inventa
   credenciales y no exige apagar la migración.
3. **`lang=en`.** Hasta el enlace viene en inglés. La plantilla y el idioma son nuestros
   en cuanto el correo lo mandemos nosotros (`defaultLocale: "en"` en la config viva).

### H3 — ahora con evidencia documental

Los enlaces de restablecimiento de Firebase **caducan a la hora** (los de verificación de
correo, a los tres días) y el código es de un solo uso. No hay campo en la config de
Identity Platform para cambiarlo. Confirmado en la documentación, no deducido.
El delta entre B y C es, por tanto, exactamente **24 h contra 1 h**.

## CEO — Tabla de consenso

```
CEO DUAL VOICES — CONSENSUS TABLE:
═══════════════════════════════════════════════════════════════
  Dimensión                              Claude   Codex   Consenso
  ────────────────────────────────────── ──────── ─────── ──────────
  1. ¿Premisas válidas?                  CONCERN   N/A     CONCERN  → H2/H3/H6 corregidas
  2. ¿Es el problema correcto?           CONFIRMED N/A     CONFIRMED
  3. ¿Alcance bien calibrado?            DISAGREE  N/A     DISAGREE → USER CHALLENGE (24 h)
  4. ¿Alternativas exploradas?           DISAGREE  N/A     DISAGREE → C reevaluada, V2 refutada
  5. ¿Riesgos operativos cubiertos?      DISAGREE  N/A     DISAGREE → V1/V5/V6/V9 al alcance
  6. ¿Trayectoria a 6 meses?             CONCERN   N/A     CONCERN  → hito de handover
═══════════════════════════════════════════════════════════════
CONFIRMED = ambas coinciden. Codex no disponible → N/A (nunca CONFIRMED).
1 de 6 CONFIRMED · 3 DISAGREE · 2 CONCERN. Modo: [subagent-only].
```

**Lectura honesta de esta tabla:** una sola voz encontró un bloqueante crítico verificable
(V1) que la revisión principal no vio. Sin la segunda voz, este plan se habría implementado
y habría fallado en dev con un error de permisos. Ése es todo el argumento a favor de correr
las voces.

## Enmiendas al plan tras las voces

- **A. `firebaseauth.users.update`** entra al rol `garciaLorcaIdentityRuntime` en terraform.
  Con una condición: hoy **todas** las cuentas de servicio comparten
  `google_service_account.runtime`, así que el permiso de fijar la contraseña de cualquiera
  llegaría también a `students`, `finance` y el resto. El propio terraform ya lo anota como
  deuda ("la solución limpia es una cuenta de servicio por servicio"). Este cambio es
  justo el disparador de esa deuda → **decisión al gate (D-A)**.
- **B. Semántica de login para `invited`:** `CanAuthenticate()` devuelve `false` para
  cualquier estado que no sea `active`, así que un `invited` no entra — correcto. El hueco
  es el mensaje: si el proveedor ya aceptó la contraseña y el `UPDATE` local falló, la
  persona tiene credencial válida y la app le dice "no tienes acceso". Debe decir
  **"Tu cuenta aún no está activada; vuelve a abrir el enlace del correo"**, y el token
  sigue vivo para que ese segundo intento funcione.
- **C. Sin flag de módulo.** Regla 3 del repo pide `module_<name>` en OFF para módulos
  nuevos. Esto no es un módulo: es la reparación del camino por el que entra todo el mundo,
  y hoy ese camino está roto. Un flag apagado sería dejar el sistema roto a propósito. El
  interruptor real y suficiente es la presencia de la credencial SMTP: sin ella el servicio
  arranca igual y las invitaciones se niegan nombrando lo que falta.
- **D. Alcance añadido por las voces:** token fuera de la query string donde se pueda
  + `Referrer-Policy: no-referrer`; compositor MIME que codifique cabeceras (RFC 2047) en
  vez de concatenar el `displayName`; limitador por IP como **tarea**, no como intención;
  timeout SMTP de 10 s; runbook de rotación de la credencial.
- **E. El `Mailer` deja de venderse como base del envío masivo.** Sirve para invitaciones y
  restablecimientos. Los recibos a 400 familias necesitan medir el transporte primero.
- **F. Hito de handover (noviembre), no dependencia de tarea:** buzón `no-reply@`, acceso
  DNS, DMARC (`p=none` hoy — el dominio del colegio es suplantable por cualquiera) y
  quién será dueño de las credenciales tras la entrega. Se pide entero, de una vez.

## Registro de errores y rescate — versión CEO final

Ver la tabla completa en la Sección 2. Tres GAPs identificados, tres cerrados:
fallo de SMTP sin rescate → mensaje con motivo + estado `sin enviar`; proveedor caído a
media aceptación → token intacto y 503; `catch` genérico en el front → mensajes por clase
de error. Añadidos tras las voces: inyección CRLF en cabeceras del correo, y el mensaje de
login para un usuario `invited`.

## Resumen de cierre — CEO

| Dimensión | Estado |
|---|---|
| Modo | SELECTIVE EXPANSION |
| Premisas retadas | 6 (2 falsas, 1 parcialmente falsa, 1 imposible como estaba escrita) |
| Hallazgos nuevos propios | H1–H4 (sondas en vivo), H5 (`ChangePassword` se niega siempre), H6 (la entrega es lo único roto) |
| Hallazgos de la voz independiente | 10, de los cuales **1 bloqueante verificado** (permiso IAM ausente) |
| Secciones con hallazgos | 1, 2, 3, 4, 5, 6, 7, 8, 9, 11 |
| Secciones sin hallazgos | 10 (trayectoria): revisada, deuda anotada, reversibilidad 4/5, sin objeción |
| Expansiones aceptadas | E1, E2, E3, E4→hotfix, E5 |
| Expansiones diferidas | E6, E7, E8 + envío masivo + DKIM propio |
| Al gate como USER CHALLENGE | 24 h contra 1 h (B contra C) |
| Al gate como decisión de gusto | Cuenta de servicio por servicio (D-A) |
| Bloqueante externo | Credencial SMTP del colegio |
| Desbloqueo disponible hoy | Enlace generado a mano para los 2 usuarios muertos (H6) |

---

# REVISIÓN DE DISEÑO (Fase 2)

## Step 0 — Alcance de diseño

**0A. Nota de partida: 5.4/10.** El plan decide tablas, hashes, órdenes de escritura y modos
de fallo con precisión poco común, y dedica 40 líneas a lo único que la gente va a ver. Un
10 para ESTE plan sería: la pantalla de activación especificada componente a componente
(no "usa el sistema de diseño"), los cinco estados de la píldora escritos con su tono y su
regla de tiempo, el correo tratado como superficie de diseño, y un breakpoint móvil real
para las dos superficies que toca.

**0B. Sistema de diseño:** existe y es **mandato** (`.claude/docs/design/DESIGN-SYSTEM.md`,
fuente Claude Design `7162d625…`). Todo lo de abajo se calibra contra él. La regla de oro es
"menor cantidad de clics posible".

**0C. Qué reutilizar (y el error que corrijo):** el inventario de este plan decía que no
existe pantalla para poner la contraseña. **Es falso.**
`apps/web/app/_screens/ChangePassword.tsx` existe, se titula **"Crea tu contraseña"** —el
mismo texto que necesita `/activar`— y ya trae la validación en vivo de longitud y
coincidencia que este plan proponía escribir otra vez. También es la pantalla más pobre del
producto: `placeItems:center`, tres inputs, un botón, sin `LogoBadge`, sin la marca del
colegio. Es literalmente el "formulario suelto" contra el que el propio plan advertía.
Además está muerta: llama a `changePassword`, que el backend rechaza siempre con un
proveedor configurado (H5).

Otros patrones ya montados y desaprovechados por el plan: tabs de filtro **con contador**,
stat cards, drawer, toast, `Avatar`, y el `IconBtn` circular que el sistema declara
obligatorio para acciones de fila.

**0D. Áreas de foco:** las siete, con peso en cobertura de estados, responsive y sistema de
diseño (las tres notas más bajas). Generador de mockups disponible (`DESIGN_READY`); no se
generan: la superficie ya tiene lenguaje visual definido y canónico, así que la decisión útil
es *qué patrón existente usar*, no *qué aspecto inventar*. Decisión registrada (P3).

## Step 0.5 — Voces independientes (diseño)

**CODEX DICE (diseño — reto de UX):** `[codex-unavailable]`.

**CLAUDE SUBAGENT (diseño — revisión independiente):** 10 hallazgos. Cuatro afirmaciones
suyas contradicen este plan; **verifiqué las cuatro y las cuatro son ciertas**:

| Verificación | Resultado |
|---|---|
| `ChangePassword.tsx` existe, se titula "Crea tu contraseña", monta en `AppShell.tsx:362` | **CIERTO** — el inventario del plan estaba mal, ya corregido arriba |
| `Login.tsx:64` es `gridTemplateColumns: "1.05fr .95fr"` inline y `globals.css` no tiene ningún breakpoint para el login (sólo 4 `@media` en todo el archivo: `topbar` y `cmdk`) | **CIERTO** — en un teléfono de 390px son dos columnas de ~200px |
| `StatusPill({active: boolean})` es binaria y hacen falta cinco estados | **CIERTO** (`AdminScreen.tsx:35`) |
| No hay estado vacío en la tabla: `usersF.map(...)` sin alternativa | **CIERTO** (`AdminScreen.tsx:186`) — la Sección 4 decía "ya existe" y se equivocaba |

## Litmus de diseño — tabla de consenso

```
DESIGN DUAL VOICES — LITMUS SCORECARD:
═══════════════════════════════════════════════════════════════════
  Dimensión                          Claude  Codex  Consenso
  ─────────────────────────────────── ─────── ─────── ──────────
  1. Arquitectura de información      6/10    N/A    CONCERN
  2. Cobertura de estados             5/10    N/A    CONCERN
  3. Coherencia del recorrido         5/10    N/A    CONCERN
  4. Especificidad (anti-slop)        6/10    N/A    CONCERN
  5. Alineación con el sistema        5/10    N/A    CONCERN
  6. Intención responsive             4/10    N/A    **CRÍTICO**
  7. Accesibilidad                    7/10    N/A    OK con huecos
═══════════════════════════════════════════════════════════════════
Media 5.4/10 · Codex no disponible → N/A · Modo [subagent-only]
Reglas duras: clasificador = APP UI. Ningún patrón de la lista negra de
slop presente (sin rejilla de 3 columnas, sin degradados morados, sin
iconos en círculos decorativos). El riesgo aquí es el contrario: ausencia
de decisión, no exceso de adorno.
```

## Pases 1-7

### Pase 1 — Arquitectura de información · 6/10 → 9/10

Acierta la jerarquía del director (**estado primero, botón después**) y nunca decide la del
docente. Y mete cinco significados en una píldora de 90 px.

**Corrección — jerarquía del docente en `/activar`,** en este orden y sin nada más:
1. La marca del colegio (está entrando a un sistema del que nunca oyó hablar; primero tiene
   que saber que es el suyo).
2. **"Crea tu contraseña"** + su propio correo, para que confirme que el enlace es para él.
3. El campo, con los requisitos **visibles antes de teclear**.
4. Un solo botón.
Nada de "bienvenido", nada de explicar qué es el sistema. Si borrar el 30% mejora, se borra.

**Corrección — la pregunta real del director.** Lo que trae a esta pantalla no es "¿quién
existe?" sino **"¿quién falta por entrar?"**. Se responde con dos piezas que el sistema de
diseño ya declara obligatorias (§4) y la pantalla ya monta:
- stat card **"Pendientes de activar · N"** (hoy hay Usuarios / Roles / Activos /
  Suspendidos: falta justo ésa)
- tab de filtro **`Invitados (7)`** junto a `Usuarios` y `Roles`.

### Pase 2 — Cobertura de estados · 5/10 → 9/10

| Superficie | Cargando | Vacío | Error | Éxito | Parcial |
|---|---|---|---|---|---|
| Tabla de Usuarios | esqueleto (ya existe) | **NUEVO:** "Todavía no hay usuarios" + botón *Invitar al primero* | ya existe + Reintentar | — | — |
| Búsqueda sin resultados | — | **NUEVO:** "Nadie coincide con «X»" + *Limpiar búsqueda* | — | — | — |
| Cajón Invitar | botón *Enviando…* | — | motivo humano en el cajón, sin cerrarlo | toast + fila nueva ya con la píldora | **NUEVO:** el cajón **no se cierra**; su cuerpo pasa a "La cuenta se creó, pero el correo no salió" + causa + **[Reintentar envío]** / **[Cerrar]** |
| Fila de usuario | esqueleto | — | — | píldora | píldora **Sin enviar ⚠ pulsable** → ficha con `sent_at`, `sent_count`, `last_error` |
| `/activar` — validar token | **NUEVO:** pantalla completa de marca mientras responde el `GET` (en datos móviles no es instantáneo) | — | 2 mensajes, no 3 | — | — |
| `/activar` — guardar | botón *Guardando…* | — | mensaje humano, foco movido al error | **entra directo** | proveedor caído → "Vuelve a intentarlo", token intacto |
| Olvidé mi contraseña | *Enviando…* | — | mismo mensaje que el éxito | mensaje neutro + "revisa spam" | — |

**El error que más importa no puede vivir en un toast de 2.6 s.** La Sección 8 guarda
`last_error` y ninguna pantalla lo enseñaba: la mitad del arreglo (que el fallo se vea) no
llegaba a los píxeles. Corregido arriba.

**Regla única de tiempo para la píldora** (el plan escribía el mismo dato de tres formas:
`vence en Xh`, `[Invitado · 6 h]`, `vence mañana 12:40`):
- quedan **más de 12 h** → absoluto: **"vence el 9 de sep, 12:40"**
- quedan **menos de 12 h** → relativo: **"vence en 6 h"**
- pasada la hora → **"Vencida"**, sin número.
Se recalcula **sólo en `reload()`**. Escrito aquí explícitamente porque, si no, alguien mete
un `setInterval` en una pantalla que la Sección 4 decidió que no vigila.

### Pase 3 — Recorrido y arco emocional · 5/10 → 9/10

| # | El docente hace | Siente | Qué lo sostiene |
|---|---|---|---|
| 1 | Recibe un correo de una dirección que no conoce | Desconfianza | **De: `Colegio García Lorca`** (el nombre visible pesa más que la dirección) + quién le da acceso y con qué rol |
| 2 | Toca el botón en el teléfono | Duda: "¿esto es de verdad?" | Aterriza en una pantalla con la marca del colegio, no en `firebaseapp.com` en inglés |
| 3 | Espera a que valide el token | Impaciencia | Pantalla de marca, no un blanco |
| 4 | Teclea la contraseña | Fricción | Requisitos visibles antes, toggle de ver contraseña, error en `blur` |
| 5 | Guarda | Alivio o rabia | **Entra directo.** No al login |

**El destino tras activar aparecía tres veces con tres respuestas** (S-C: "login (o dentro)";
wireframe: "→ login"; HORA 2: "entra"). **Decisión: entra.** Acaba de teclear su contraseña
dos veces; mandarlo a un formulario a teclearla una tercera es el minuto exacto en que le
echa la culpa al sistema.

**Y la ruptura grande:** el arco del docente termina, un paso después, en un login que en su
teléfono es una rejilla fija de dos columnas de ~200 px cada una. El plan prometía
"`/activar` nace responsive" y entregaba a una pantalla inservible. **Entra al alcance el
breakpoint del login** — es la misma superficie que este PR ya toca.

### Pase 4 — Riesgo de slop · 6/10 → 8/10

Clasificador: **APP UI**. Ninguno de los 11 patrones de la lista negra aparece. El riesgo
aquí es el contrario: **ausencia de decisión**. *"Debe usar el sistema de diseño"* es pedirle
buen gusto al implementador en lugar de decirle qué construir. Sustituido, arriba y abajo,
por nombres de componentes concretos: `LogoBadge`, panel forest del `Login`, `StatusPill` de
cinco variantes, `IconBtn`, tabs con contador, stat card.

### Pase 5 — Alineación con el sistema · 5/10 → 9/10

**`StatusPill` de binaria a cinco estados**, con los tonos que el sistema ya define
(`--ok/--warn/--bad`):

| Estado | Tono | Etiqueta | Sub |
|---|---|---|---|
| Activo | `ok` (forest) | Activo | — |
| Invitado vigente | `warn` (yellow) | Invitado | vence el 9 de sep, 12:40 |
| Invitación vencida | `bad` suave | Vencida | — |
| Creado sin enviar | `bad` (red) + ícono `warning` | Sin enviar | pulsable → detalle |
| Suspendido | `bad` | Suspendido | — |

**Y un `stats` que se rompe hoy mismo:** `AdminScreen.tsx:101` calcula
`suspended: users.filter(u => u.status !== "active")`. Con un tercer estado, **los invitados
se contarían como suspendidos**. Hay que tocarlo en el mismo cambio o el número miente desde
el primer usuario invitado.

**El renglón no aguanta lo que le metemos.** La fila es `2.4fr 1.6fr 1fr auto` y ya lleva
tres píldoras de texto (*Roles* · *Reenviar acceso* · *Suspender*). Con la píldora de estado
más ancha y el botón renombrado, a 1320 px se aprieta y por debajo de ~1100 se rompe.
**Decisión: acciones secundarias a `IconBtn` circulares con `aria-label`**, que es el patrón
canónico de tabla del propio sistema de diseño (§7) y el componente que ya está en la lista
de "construir en `_design/`".

**Un solo componente `SetPassword`, dos modos.** `token` (activación) y `current` (rotación
forzada). `ChangePassword.tsx` se reescribe encima de él con el tratamiento visual del
`Login` (panel de marca + `LogoBadge`). Si no, quedan dos pantallas con el mismo título y
distinta calidad, divergiendo para siempre.

### Pase 6 — Responsive y accesibilidad · 4/10 y 7/10 → 9/10 ambas

**Responsive (lo más grave del plan):** una frase verdadera, cero breakpoints.
- `@media (max-width: 860px)` para `Login` y `SetPassword`: **una columna**; el panel de marca
  pasa a banda superior de ~96 px con `LogoBadge` y el degradado; el formulario ocupa el resto.
- Tabla de Usuarios por debajo de ~1100 px: la fila se apila en dos líneas
  (identidad + estado arriba, roles + acciones abajo).
- `/activar` se diseña **primero para 390 px** y luego se ensancha. Es el caso principal: el
  docente abre el correo en el teléfono.

**Accesibilidad — lo que faltaba:** al aparecer el error en el cajón, **mover el foco** al
mensaje (`role="alert"` + `tabIndex={-1}`); la píldora de cuenta atrás lleva la fecha
absoluta en `title`/`aria-label` para quien no puede calcular "en 6 h"; la píldora
*Sin enviar* comunica con **ícono + texto**, nunca sólo color; `autocomplete="new-password"`;
objetivos táctiles ≥44 px también en los `IconBtn` nuevos.

### Pase 7 — Decisiones sin resolver (y ya resueltas aquí)

| Decisión | Si se difiere, qué pasa | Resuelta como |
|---|---|---|
| ¿`/activar` reutiliza `ChangePassword`? | Dos pantallas con el mismo título y distinta calidad | Un `SetPassword`, dos modos |
| ¿Cómo se escribe el tiempo de la píldora? | Tres formatos en el producto, y un `setInterval` que nadie pidió | Regla >12 h / <12 h, recalculada en `reload()` |
| ¿Dónde termina el docente al activar? | Un login más, a teclear otra vez | Entra directo |
| ¿Qué ve el director cuando el correo no sale? | Un toast de 2.6 s y luego nada | Cajón que no se cierra + píldora pulsable |
| ¿Caben tres botones de texto en la fila? | Layout roto por debajo de 1100 px | `IconBtn` circulares |
| ¿Cómo se ve el correo? | Un enlace pelado que el spam y las personas descartan igual | Especificado abajo |
| ¿Uno o dos campos de contraseña? | Se elige por inercia | **AL GATE (D-C)** — es una decisión de gusto real |

### El correo, como superficie de diseño

Es lo primero que el docente ve del sistema, antes que `/activar`. Especificado, no delegado:

- **De:** `Colegio García Lorca <no-reply@ceglorca.com>`
- **Asunto:** *Te damos acceso al sistema del Colegio García Lorca*
- Una línea con quién invita y el rol: *"Andrés Romo te dio acceso como Maestro."*
- Vigencia en palabras: *"El enlace funciona hasta el 9 de sep a las 12:40."*
- Botón **Crear mi contraseña** + la URL en texto debajo (los clientes corporativos rompen botones)
- Cierre con `soporte@garcialorca.edu.mx`
- **Versión en texto plano redactada**, no autogenerada del HTML.

### Copia en español — reescrituras aceptadas

| Antes | Ahora | Por qué |
|---|---|---|
| "Esta invitación venció. Pídele al director que te la reenvíe." | **"Esta invitación ya venció. Pide una nueva en la dirección del colegio o escribe a soporte@garcialorca.edu.mx."** | No siempre invita el director, y así el docente tiene algo que hacer solo |
| "El enlace está incompleto. Ábrelo desde el correo." | **"Este enlace no funciona. Ábrelo directamente desde el correo que te enviamos."** | "Incompleto" es lenguaje de programador |
| "…NO se pudo enviar el correo: `<motivo>`" | Tres causas humanas — **"el servidor de correo rechazó nuestra contraseña"** / **"no respondió"** / **"rechazó esa dirección"** — con el código SMTP detrás de **Ver detalle** | Enseñarle `535 5.7.8` a la directora es el mismo pecado que la página en inglés |
| "Si esa dirección tiene cuenta, te llegará un enlace." | **"Si esa dirección tiene una cuenta, te llegará un correo en los próximos minutos. Revisa también tu carpeta de correo no deseado."** | Sin filtrar nada, pero dejando saber qué esperar |
| "Tu cuenta aún no está activada; vuelve a abrir el enlace del correo." | **"Tu cuenta todavía no está activada. Abre el enlace que te enviamos por correo para terminar."** | Sin punto y coma en interfaz |
| "Invitación enviada · vence mañana 12:40" | **"Invitación enviada · vence el 9 de sep, 12:40"** | "Mañana" es ambiguo si se invita a las 00:30 |

**Contradicción corregida:** la Sección 2 exige **un solo** mensaje para token desconocido /
vencido / usado (por seguridad); la Sección 11 decía "3 mensajes distintos" en `/activar`.
Son **dos**: *sin token* y *token no válido*.

## Alcance añadido por la fase de diseño

| # | Item | Esfuerzo | Decisión |
|---|---|---|---|
| G1 | `SetPassword` único con dos modos; `ChangePassword.tsx` reescrito encima | S (CC ~25 min) | **DENTRO** (P4 DRY, P1) |
| G2 | Breakpoint del login/SetPassword a una columna | S (CC ~15 min) | **DENTRO** (P2 — misma superficie) |
| G3 | `StatusPill` de 5 variantes + regla de tiempo + arreglo de `stats.suspended` | S (CC ~20 min) | **DENTRO** (P1 — sin esto el requisito del dueño no se ve) |
| G4 | Acciones de fila a `IconBtn` circulares | S (CC ~15 min) | **DENTRO** (mandato del sistema de diseño) |
| G5 | Éxito parcial: cajón que no se cierra + píldora pulsable con `last_error` | S (CC ~20 min) | **DENTRO** (P1 — es la mitad del arreglo) |
| G6 | Estados vacíos (tabla, búsqueda) + estado de validación de token | S (CC ~15 min) | **DENTRO** |
| G7 | Stat card *Pendientes de activar* + tab `Invitados (N)` | S (CC ~15 min) | **DENTRO** (la pregunta real del director) |
| G8 | Correo especificado (remitente, asunto, cuerpo, texto plano) | S (CC ~15 min) | **DENTRO** |
| G9 | Copia en español reescrita | S (CC ~10 min) | **DENTRO** |
| G10 | Un campo de contraseña con toggle, o dos con confirmación | — | **AL GATE (D-C)** |

---

# REVISIÓN DE INGENIERÍA (Fase 3 — la última, sobre el plan ya enmendado)

## Step 0 — Reto de alcance (leyendo el código, no el plan)

El plan reutiliza proto, rutas del gateway, guard de subconjunto de permisos, `Provisioner`,
`pkg/authctx` y el patrón de migración versionada. Lo genuinamente nuevo es una tabla, un
puerto, un adaptador y una pantalla. Toca ~20 archivos contando la reescritura de tests. Por
encima del umbral de 8, y justificado: cada síntoma del dueño sale del mismo hueco. Recortar
cualquiera de las cuatro piezas incumple un requisito nombrado.

## Step 0.5 — Voces independientes (ingeniería)

**CODEX DICE (ing — reto de arquitectura):** `[codex-unavailable]`.

**CLAUDE SUBAGENT (ing — revisión independiente):** 12 hallazgos, dos de ellos críticos y
ninguno cosmético. Los dos que son afirmaciones sobre este repo los verifiqué:

| Verificación | Resultado |
|---|---|
| El `loginLimiter` **no** es por IP en producción: `trustProxy: os.Getenv("TRUST_PROXY") != ""` (`ratelimit.go:99`) y `deploy/README.md:165` dice **"TRUST_PROXY stays OFF"**. Detrás de Cloud Run todo llega del front end de Google → **un solo bucket global** de 10 en ráfaga / 30 por minuto **para todo el colegio** | **CIERTO.** Y no es sólo un problema de este plan: **hoy mismo**, un cliente cualquiera puede gastar el bucket y dejar al colegio entero sin poder entrar |
| `userStatusName` es una lectura de mapa pelada que devuelve `""` al fallar (`client/iamadmin.go:208`) | **CIERTO.** Un estado nuevo sin entrada viaja como `{"status":""}` y la píldora pinta **"Suspendido"** para todo invitado, sin un solo error |

### Hallazgos y qué se hizo con cada uno

| # | Hallazgo | Sev | Acción |
|---|---|---|---|
| E1 | **Una sola tabla para invitación y reset se rompe sola.** El índice `UNIQUE (user_id) WHERE accepted_at IS NULL` colisiona al segundo reset; y si el reset reusa la fila, **borra la prueba de que el usuario se activó** (vuelve a *Invitado* y reaparece el botón que el dueño quiere que desaparezca). Peor: `AcceptInvitation → status='active'` sin condición es un **salto de suspensión** — un suspendido pide reset, abre el enlace y vuelve a estar activo, por la puerta principal y sin sesión | **Crítico** | **ACEPTADO ENTERO.** Columna `purpose CHECK IN ('invitation','reset')`; índice `UNIQUE (user_id, purpose) WHERE consumed_at IS NULL`; `accepted_at` se renombra a **`consumed_at`** (significa "este token se gastó", no "esta persona se activó"); la activación vive en `users`, nunca se deduce del token; el accept transiciona **sólo** `WHERE status='invited'`; **el reset no toca el estado jamás** |
| E2 | **"204 siempre, sin diferencia de tiempo" es imposible con envío síncrono.** Sin cuenta: ~5 ms. Con cuenta: ~500 ms. Es un oráculo binario limpio sobre todo el directorio del colegio, desde un endpoint sin sesión | **Crítico** | **ACEPTADO.** Se rompe el DRY justo en este eje: **invitar/reenviar = síncrono** (hay un director mirando que necesita la verdad); **reset = responder 204 y mandar después** (no hay nadie mirando). `issueAndSend` comparte la emisión del token, no la política de envío |
| E3 | El limitador reutilizado no es por IP; colgar de él tres endpoints anónimos convierte el login en blanco de DoS | **Alto** | **ACEPTADO.** (a) instancia de limitador **propia** para invitación/reset, nunca la del login; (b) tope **por correo** en la base (`sent_count` + `last_sent_at`, 3 por hora), que es el único que ata cuando todo parece venir de una IP; (c) escrito en claro que el límite por IP **hoy no opera**. Y el DoS de login preexistente sale a `TODOS` como item propio |
| E4 | `AcceptInvitation` sin proveedor no está definido, y `InviteUser` escribe `CreateUser(ctx, user, "")` — **un hash vacío** | **Alto** | **ACEPTADO.** Regla escrita: la aceptación escribe la credencial en el almacén que mande (`SetPassword` si hay proveedor, `UpdatePassword` si no) y **se niega si no hay ninguno**. Test de regresión: un usuario con hash vacío no autentica con **ninguna** contraseña, `""` incluida |
| E5 | "Entra directo" está decidido pero no diseñado: con el proveedor encendido la sesión sólo existe tras `firebaseIdToken()` → `/auth/google` | **Alto** | **ACEPTADO.** `/activar` importa el SDK **en diferido** (misma técnica que `Login`; es una ruta aparte, así que los 468 KB no tocan el resto). Fallback definido: si el intercambio falla tras fijar la contraseña, cae a `/login` **con banner de éxito, nunca con error** — la contraseña sí quedó puesta |
| E6 | El tercer estado cruza cinco capas y **una falla en silencio** (`userStatusName`) | **Alto** | **ACEPTADO.** `userStatusName` devuelve `"unknown"` o error, no `""`; `SetUserStatus` **rechaza** `invited` (sólo aceptar/reenviar lo escriben, o un director dejaría una cuenta buena en un estado del que no hay salida por UI); un test de tabla pasea un estado por las cinco capas |
| E7 | `net/smtp` **no acepta `context.Context`** ni pone deadlines: el timeout de 10 s prometido no existe, y si el envío queda dentro de `withTenant` retiene una **transacción de Postgres abierta** | **Alto** | **ACEPTADO.** `net.Dialer{Timeout}` → `smtp.NewClient` → `SetDeadline` → goroutine que cierra al `ctx.Done()`. Y regla explícita y testeada: **el envío ocurre FUERA de la transacción** (emitir y commitear → enviar → segunda transacción corta que anota `sent_at`/`last_error`) |
| E8 | La concurrencia analizada era accept-vs-accept; falta **accept-vs-resend**, y ahí el accept consume un token que ya no es el suyo | **Medio-alto** | **ACEPTADO.** El `UPDATE` de aceptación **reafirma el token**: `WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > now()`. Test de integración de accept-vs-resend, que es el difícil |
| E9 | La respuesta de RLS para el endpoint anónimo es correcta **hoy** y nada la sostiene mañana | **Medio** | **ACEPTADO.** Test de integración: el repo devuelve cero filas con `app.current_tenant` sin enlazar. Y guard de compilación: **el puerto no lleva parámetro de tenant** |
| E10 | Mecánica de migración: el CHECK inline se llama `users_status_check` y hay que soltarlo por ese nombre; el `NOT VALID` es cargo cult sobre una restricción que **amplía**; y **el runbook de revert es un bug de seguridad** | **Medio** | **ACEPTADO, y el revert corregido:** `UPDATE users SET status='suspended' WHERE status='invited'` — nunca `'active'`. Poner en activo a quien jamás probó que lee ese correo es exactamente la cuenta fantasma que `CreateAccount` existe para impedir |
| E11 | La suite actual codifica lo que este plan borra: ~10 tests a reescribir, no a añadir | **Medio** | **ACEPTADO.** Entran al presupuesto y, por la regla TDD del repo, se reescriben **en rojo primero** |
| E12 | Cloud Run no tiene IP de salida estable y cPanel/CSF suele bloquear autenticaciones desde IPs desconocidas; el arreglo real (VPC connector + Cloud NAT con IP reservada) es **coste mensual recurrente** contra la decisión de "~7 MXN/mes en reposo" | **Medio** | **ACEPTADO Y ASCENDIDO.** Deja de ser nota al pie: pasa a **tarea de verificación del día 0** — probar `mail.ceglorca.com` autenticado desde dos redes distintas **antes** de escribir el adaptador. Si falla, es un go/no-go con precio |

## ENG — Tabla de consenso

```
ENG DUAL VOICES — CONSENSUS TABLE:
═══════════════════════════════════════════════════════════════
  Dimensión                              Claude    Codex  Consenso
  ────────────────────────────────────── ───────── ────── ──────────
  1. ¿Arquitectura sólida?               CONCERN    N/A   CONCERN  → E1 rehace el modelo
  2. ¿Cobertura de tests suficiente?     DISAGREE   N/A   DISAGREE → +4 tests, +10 reescrituras
  3. ¿Riesgos de rendimiento tratados?   CONCERN    N/A   CONCERN  → E2/E7
  4. ¿Amenazas de seguridad cubiertas?   DISAGREE   N/A   DISAGREE → 4 huecos, los 4 cerrados
  5. ¿Caminos de error tratados?         CONFIRMED  N/A   CONFIRMED
  6. ¿Riesgo de despliegue manejable?    CONCERN    N/A   CONCERN  → credencial + IP + revert
═══════════════════════════════════════════════════════════════
1 de 6 CONFIRMED · 2 DISAGREE · 3 CONCERN. Modo [subagent-only].
```

**Los cuatro huecos de seguridad que encontró la voz, y ninguno lo había visto la revisión
principal:** enumeración por tiempo en el reset, **salto de suspensión** por reset→activo,
limitador global en vez de por IP, y un runbook de revert que activaba cuentas que nunca
probaron leer su correo. Los cuatro están cerrados arriba.

## Sección 1 — Arquitectura (diagrama final, ya enmendado)

```
  apps/web
   ├─ /login ──────────────► POST /api/v1/auth/password-reset   (anónimo, limitador propio)
   ├─ /activar?token= ─────► GET  /api/v1/auth/invitations/{t}  (anónimo)
   │                        └► POST /api/v1/auth/invitations/{t}/accept
   │                             └─ éxito ─► firebaseIdToken (import diferido)
   │                                          └► POST /auth/google ─► cookies HttpOnly
   │                             └─ fallo del intercambio ─► /login + banner de éxito
   └─ /admin ──────────────► POST /api/v1/users/invite
                             POST /api/v1/users/{id}/invite

  gateway  ── Connect ──►  iam
                            │
                            ├─ usecase.AdminService
                            │    ├─ issueToken()  ──► InvitationRepo (Postgres + RLS)
                            │    ├─ invitar/reenviar: enviar SÍNCRONO   ─┐
                            │    └─ reset:            responder y enviar ─┤
                            │                                            ▼
                            │                              ports.Mailer ──► SMTP colegio
                            │                              (FUERA de la transacción,
                            │                               deadline propio de 10 s)
                            └─ ports.IdentityProvisioner
                                 ├─ CreateAccount   (Admin SDK)
                                 └─ SetPassword     (Admin SDK — NECESITA
                                                     firebaseauth.users.update)

  Postgres
    users             status: active | suspended | invited      ← activación vive AQUÍ
    user_invitations  user_id, purpose, token_hash, expires_at,
                      consumed_at, sent_at, sent_count, last_sent_at, last_error
                      UNIQUE (token_hash)
                      UNIQUE (user_id, purpose) WHERE consumed_at IS NULL
                      ENABLE + FORCE ROW LEVEL SECURITY
```

**Lo que cambió respecto al diagrama del CEO:** el reset dejó de compartir camino de envío con
la invitación (E2), el envío salió de la transacción (E7), `accepted_at` se convirtió en
`consumed_at` con `purpose` (E1), y `/activar` ganó el intercambio de sesión con su fallback (E5).

## Sección 2 — Calidad de código (ing)

- `issueAndSend` **se parte en dos**: `issueToken` (compartido) y la política de envío
  (síncrona para invitar/reenviar, diferida para reset). DRY sobre lo que de verdad es igual.
- `userStatusName` deja de tragarse el fallo.
- Se borra `googleid/invite.go` **con su test**.
- El puerto queda `CreateAccount` + `SetPassword`; **sin parámetro de tenant** en el puerto de
  invitaciones (guard de compilación para E9).
- Nombres: `consumed_at`, no `accepted_at`. La diferencia entre "el token se gastó" y "la
  persona se activó" es justo la que produjo E1.

## Sección 3 — Diagrama de tests y artefacto

**Artefacto en disco:**
`~/.gstack/projects/romodevx-garcia-lorca/andresromo-dev-test-plan-20260908-124500.md`

Cobertura: ~28 unit · ~7 integración · 2 E2E · **+10 tests existentes reescritos en rojo**.
Los cuatro tests nuevos que exigió la voz y no estaban: **accept-vs-resend**, **hash vacío no
autentica jamás**, **RLS sin tenant enlazado devuelve cero filas**, y **el estado nuevo
paseado por las cinco capas**.

`AdminScreen.test.tsx:98` merece su renglón: hoy afirma que el botón de reenvío aparece en un
usuario **activo**. **La suite codifica el bug que el dueño reportó.** Arreglarlo es reescribir
aserciones, no añadir un caso — igual que pasó con el registro del command palette.

## Sección 4 — Rendimiento (ing)

- `ListUsers` ya hace 2 consultas (`repository.go:189-221`), así que añadir el LEFT JOIN del
  estado de invitación es un cambio modesto, no una reescritura. Se mantiene la exigencia:
  **una consulta, no 1+N**, con aserción sobre el número de queries.
- El envío síncrono sale de la transacción (E7): deja de retener una conexión de Postgres
  durante cientos de milisegundos.
- El reset deja de bloquear el request (E2): además de cerrar el oráculo de tiempo, quita el
  peor caso de latencia del endpoint público.

## Registro de modos de fallo — versión final (huecos críticos marcados)

| # | Modo de fallo | ¿Cubierto? | Cómo |
|---|---|---|---|
| F1-F10 | (ver registro CEO) | Sí | sin cambios, salvo F6 |
| **F6 (corregido)** | Filas `invited` tras un revert | **Sí** | `UPDATE … SET status='suspended'` — **nunca `'active'`**. Activar a quien no probó su correo era el bug que el revert introducía |
| F11 | **Salto de suspensión** vía reset → `status='active'` | **Sí** | el reset no toca el estado; el accept sólo transiciona `WHERE status='invited'` |
| F12 | **Oráculo de tiempo** en `/auth/password-reset` | **Sí** | responder 204 y enviar después; tiempo constante exista o no la cuenta |
| F13 | Un cliente gasta el bucket del login pidiendo resets y **deja al colegio fuera** | **Sí** | limitador propio para invitación/reset + tope por correo en la base |
| F14 | Accept y resend a la vez: el accept consume un token que ya no es el suyo | **Sí** | el `UPDATE` reafirma `token_hash` |
| F15 | Usuario con `password_hash = ''` autentica con cualquier cosa | **Sí** | test de regresión explícito |
| F16 | `net/smtp` cuelga y retiene una transacción de Postgres | **Sí** | deadlines propios + envío fuera de la transacción |
| F17 | `userStatusName` devuelve `""` y todo invitado se pinta *Suspendido* | **Sí** | falla ruidoso + test de las cinco capas |
| F18 | cPanel/CSF bloquea la IP de salida variable de Cloud Run | **Parcial** | **verificación del día 0** desde dos redes. Si falla: VPC connector + Cloud NAT, con coste mensual → go/no-go |
| F19 | El límite por IP **hoy no opera** en producción (`TRUST_PROXY` OFF tras Cloud Run) | **No, y es preexistente** | Sale a `TODOS` como item propio: afecta a `/auth/login` desde antes de este plan |

## A TODOS.md (diferido de todas las fases)

| Item | Origen | Por qué fuera |
|---|---|---|
| Reintento automático de envío con backoff | E6 (CEO) | el reenvío manual cubre el caso; sin datos de cómo falla, un backoff es adivinar |
| Plantillas de correo para recibos y avisos | E7 (CEO) | otro módulo; el puerto `Mailer` queda listo |
| MFA para el director | E8 (CEO) | fuera del radio |
| Envío masivo a ~400 familias | CEO S1 | el transporte compartido tiene topes por medir |
| DKIM/DMARC propios (`p=none` hoy: el dominio del colegio es suplantable) | voz CEO | mejora de reputación; hito de handover |
| **`TRUST_PROXY` + XFF: hoy el límite por IP no opera y `/auth/login` es DoS-able** | voz ING (E3) | **preexistente, no lo introduce este plan** — pero hay que arreglarlo |
| Una cuenta de servicio por servicio (hoy todas comparten `google_service_account.runtime`) | voz CEO (V1) | **al gate como D-A**: este plan es justo su disparador |
| Bitácora de autenticación completa | CEO S3 | este PR pone el primer trozo |

## Resumen de cierre — Ingeniería

| Dimensión | Estado |
|---|---|
| Archivos tocados | ~20 (backend 9 · web 7 · terraform 1 · migraciones 1 · contrato 1 · docs 1) |
| Piezas nuevas | 1 tabla, 1 puerto, 1 adaptador, 1 pantalla, 3 endpoints |
| Hallazgos de la voz independiente | 12 · **2 críticos** · 4 huecos de seguridad, los 4 cerrados |
| Afirmaciones de la voz verificadas contra el código | 2 de 2 ciertas (limitador global, `userStatusName` mudo) |
| Tests | ~28 unit · ~7 integración · 2 E2E · **+10 reescritos en rojo** |
| Artefacto de plan de pruebas | escrito en disco |
| Contrato OpenAPI | **hueco encontrado**: ni `/users/invite` ni `/users/{id}/invite` están en `docs/api/openapi.yaml` (sólo hay 24 rutas, ninguna de auth). Los 3 nuevos **y** los 2 existentes entran al contrato — la regla del repo es contrato primero |
| Reversibilidad | 4/5 (revert corregido a `suspended`) |
| Bloqueantes externos | credencial SMTP del colegio · verificación de IP de salida |

---

<!-- AUTONOMOUS DECISION LOG -->
## Decision Audit Trail

| # | Fase | Decisión | Clasificación | Principio | Razón | Rechazado |
|---|---|---|---|---|---|---|
| 1 | 0 | Saltar `/office-hours` (no hay design doc) | Mecánica | P6 | El dueño trajo un problema preciso con requisitos nombrados; el reto de premisas lo hace la fase CEO | Correr /office-hours primero |
| 2 | 0 | Alcance DX = NO | Mecánica | P3 | No hay API pública, SDK, CLI ni integrador externo; el REST sirve sólo a nuestra web y a nuestra app Expo | Correr la fase DX |
| 3 | 0.5 | Voces sólo con subagente Claude | Mecánica | — | `codex` no está instalado | — |
| 4 | 1 | Enfoque **B** (invitación propia) sobre A y C | **Taste** | P1, P5 | Único que cumple las 24 h pedidas; A y C heredan 1 h de Google | A (config del proveedor), C (híbrido) |
| 5 | 1 | E1 rescatar a los 2 usuarios muertos | Mecánica | P2 | En el radio, usuarios reales bloqueados hoy | Diferir |
| 6 | 1 | E2 `Mailer` compartido en vez de privado de iam | Mecánica | P2, P4 | Si nace privado se duplica en cuanto finance mande recibos | Privado de iam |
| 7 | 1 | E3 estado visible en la tabla | Mecánica | P1 | Para que el botón "no salga si está activo" hay que **enseñar** el estado | Sólo ocultar el botón |
| 8 | 1 | E4 deriva `disabled_user_signup` **al día 1** | Mecánica | P2 | Seguridad viva en un proyecto con datos de menores | Dejarlo como expansión |
| 9 | 1 | E5 bitácora de envíos | Mecánica | P1 | Sin ella volvemos a la ceguera de hoy | Diferir |
| 10 | 1 | E6/E7/E8 a TODOS | Mecánica | P3 | Fuera del radio o sin datos para decidir | Meterlos |
| 11 | 1 | **Sin flag de módulo** | **Taste** | P5 | Esto repara la puerta de entrada; un flag apagado deja el sistema roto a propósito | Flag `module_invitations` OFF |
| 12 | 1 | V1: añadir `firebaseauth.users.update` | Mecánica | P1 | Verificado: sin él `SetPassword` falla en ejecución | Ignorar |
| 13 | 1 | V2 refutado con evidencia; se adopta H6 en su lugar | Mecánica | P5 | La ruta de contraseña temporal no funciona con el proveedor encendido; el enlace generado sí | Contraseña temporal |
| 14 | 1 | V3 (24 h) **al gate** | **User Challenge** | — | Cuestiona un requisito que el dueño nombró; no se auto-decide | Auto-decidir por 1 h |
| 15 | 1 | Cuenta de servicio por servicio **al gate** | **Taste** | P2 borderline | Toca terraform de 10 servicios: fuera del radio estricto | Auto-aprobar |
| 16 | 2 | No generar mockups pese a `DESIGN_READY` | Mecánica | P3 | La superficie ya tiene lenguaje canónico; la decisión útil es qué patrón usar, no qué inventar | Generar mockups |
| 17 | 2 | G1–G9 al alcance | Mecánica | P1, P2, P4 | Todas en el radio, todas < 30 min CC | Diferir |
| 18 | 2 | Destino tras activar = **entra directo** | Mecánica | P5 | Tres secciones decían tres cosas; mandarlo al login es pedirle la contraseña por tercera vez | Ir a /login |
| 19 | 2 | Un campo de contraseña con toggle **al gate** | **Taste** | — | Seguridad percibida contra fricción en el teclado del teléfono | Auto-decidir |
| 20 | 3 | E1: `purpose` + `consumed_at`, activación en `users` | Mecánica | P1 | Una tabla para ambos usos rompía su índice y permitía saltar la suspensión | Mantener una fila por usuario |
| 21 | 3 | E2: reset asíncrono, invitar/reenviar síncrono | Mecánica | P1, P5 | El envío síncrono era un oráculo de tiempo; el director sí necesita la verdad al invitar | DRY total (lo que había) |
| 22 | 3 | E3: limitador propio + tope por correo | Mecánica | P1 | Verificado: el limitador es global tras Cloud Run; colgar el reset de él era DoS al login | Reutilizar el del login |
| 23 | 3 | E4–E11 al alcance | Mecánica | P1, P5 | Cierran los 4 huecos de seguridad y los 2 críticos | Diferir |
| 24 | 3 | Revert a `'suspended'`, no `'active'` | Mecánica | P1 | Activar a quien no probó su correo es la cuenta fantasma que el diseño existe para impedir | `'active'` |
| 25 | 3 | `TRUST_PROXY`/DoS de login a TODOS | Mecánica | P3 | Preexistente, no lo introduce este plan | Meterlo aquí |

## Tareas de implementación (agregadas de las tres fases)

- [ ] **C3 (P1, human: 0.1 d / CC: 5 min) — operación** — Desbloquear hoy a docentes.cegl@gmail.com y pagos@ceglorca.com generando su enlace con returnOobLink y pasándoselo al director
  - Surfaced by: ceo-review — H6 — la maquinaria funciona, sólo la entrega está rota
  - Files: (sin archivos: llamada a la API)
- [ ] **C1 (P1, human: 0.5 d / CC: 10 min) — terraform** — Añadir firebaseauth.users.update al rol garciaLorcaIdentityRuntime
  - Surfaced by: ceo-review — voz CEO V1 — verificado: el rol sólo tiene create+get; SetPassword habría fallado en dev
  - Files: deploy/terraform/identity_platform.tf
- [ ] **C2 (P1, human: 0.25 d / CC: 10 min) — terraform** — Cerrar la deriva de disabled_user_signup (terraform dice true, la config viva trae permissions:{})
  - Surfaced by: ceo-review — voz CEO V6 — hotfix del día 1, no esperar al PR
  - Files: deploy/terraform/identity_platform.tf
- [ ] **G1 (P1, human: 1 d / CC: 25 min) — web** — Un solo componente SetPassword con modos token/current; ChangePassword.tsx reescrito encima con el panel de marca del Login
  - Surfaced by: design-review — diseño #1 — hoy son dos pantallas con el mismo título y distinta calidad
  - Files: apps/web/app/_screens/ChangePassword.tsx, apps/web/app/_screens/SetPassword.tsx, apps/web/app/AppShell.tsx
- [ ] **G2 (P1, human: 0.5 d / CC: 15 min) — web** — Breakpoint @media(max-width:860px): login y SetPassword a una columna, panel de marca como banda superior
  - Surfaced by: design-review — diseño #2 — el login es una rejilla fija de 2 columnas y el docente llega desde el teléfono
  - Files: apps/web/app/_screens/Login.tsx, apps/web/app/globals.css
- [ ] **G3 (P1, human: 0.5 d / CC: 20 min) — web** — StatusPill de 5 variantes + regla única de tiempo (>12h absoluto, <12h relativo) + arreglar stats.suspended
  - Surfaced by: design-review — diseño #3/#5 — la píldora es binaria y los invitados se contarían como suspendidos
  - Files: apps/web/app/_screens/admin/AdminScreen.tsx
- [ ] **G5 (P1, human: 0.5 d / CC: 20 min) — web** — Éxito parcial: el cajón no se cierra, muestra causa humana y [Reintentar envío]; píldora 'Sin enviar' pulsable con last_error
  - Surfaced by: design-review — diseño #5 — el único error que el director debe atender vivía en un toast de 2.6 s
  - Files: apps/web/app/_screens/admin/AdminScreen.tsx
- [ ] **E6 (P1, human: 1 d / CC: 30 min) — gateway** — 3 rutas anónimas (GET invitations/{t}, POST accept, POST password-reset) con instancia PROPIA de limitador + tope por correo en la base; nunca el limitador del login
  - Surfaced by: eng-review — voz ING E3 — verificado: el limitador es global, no por IP, tras Cloud Run
  - Files: services/gateway/cmd/main.go, services/gateway/internal/handler/auth.go
- [ ] **E5 (P1, human: 0.5 d / CC: 20 min) — iam/adapters** — Provisioner.SetPassword (Admin SDK UpdateUser + emailVerified) y borrar googleid/invite.go con su test
  - Surfaced by: eng-review — voz CEO V1 — requiere la tarea C1 de terraform
  - Files: services/iam/internal/adapters/googleid/provisioner.go, services/iam/internal/adapters/googleid/invite.go
- [ ] **E4 (P1, human: 1 d / CC: 30 min) — iam/adapters** — Adaptador SMTP con deadlines propios (net.Dialer+SetDeadline+goroutine en ctx.Done), cabeceras codificadas RFC 2047 contra inyección CRLF, y envío SIEMPRE fuera de la transacción
  - Surfaced by: eng-review — voz ING E7 y voz CEO V5 — net/smtp no acepta context y el timeout prometido no existía
  - Files: services/iam/internal/adapters/smtp/mailer.go, services/iam/internal/adapters/postgres/repository.go
- [ ] **E2 (P1, human: 0.5 d / CC: 20 min) — iam/domain** — StatusInvited en domain.Status + mapper statusToDomain/statusToDB + proto UserStatus + userStatusName que falla ruidoso + union TS
  - Surfaced by: eng-review — voz ING E6 — el estado cruza 5 capas y una fallaba en silencio
  - Files: services/iam/internal/domain/user.go, services/iam/internal/adapters/postgres/mapper.go, proto/garcialorca/iam/v1/iam.proto, services/gateway/internal/client/iamadmin.go, apps/web/app/admin/api.ts
- [ ] **E3 (P1, human: 1.5 d / CC: 45 min) — iam/usecase** — issueToken compartido; InviteUser/ResendInvitation síncronos; AcceptInvitation (accept reafirma token_hash, transiciona sólo WHERE status='invited', proveedor primero y base después); RequestPasswordReset en tiempo constante que nunca toca el estado; SetUserStatus rechaza 'invited'
  - Surfaced by: eng-review — voz ING E1/E2/E4/E8 — dos críticos y el salto de suspensión
  - Files: services/iam/internal/usecase/admin.go, services/iam/internal/ports/ports.go
- [ ] **E1 (P1, human: 1 d / CC: 30 min) — migraciones** — Migración 0016: user_invitations con purpose, consumed_at, UNIQUE(token_hash), UNIQUE(user_id,purpose) WHERE consumed_at IS NULL, RLS ENABLE+FORCE; y ampliar users_status_check (por su nombre generado) con 'invited'
  - Surfaced by: eng-review — voz ING E1/E10 — una tabla para ambos usos se rompía sola y permitía saltar la suspensión
  - Files: migrations/iam/0016_user_invitations.sql
- [ ] **E10 (P1, human: 0.25 d / CC: 15 min) — operación** — Verificación del día 0: autenticar contra mail.ceglorca.com:587 desde dos redes distintas antes de escribir el adaptador; si CSF bloquea, precio del VPC connector + Cloud NAT y decisión go/no-go
  - Surfaced by: eng-review — voz ING E12 — Cloud Run no tiene IP de salida estable
  - Files: (sin archivos: verificación)
- [ ] **E9 (P1, human: 1.5 d / CC: 45 min) — tests** — Reescribir en rojo los ~10 tests que codifican el comportamiento actual (admin_invite_test, admin_resend_test, googleid/invite_test, iam_handler_invite_test, AdminScreen.test.tsx:98) + los 4 nuevos: accept-vs-resend, hash vacío no autentica, RLS sin tenant, estado por las 5 capas
  - Surfaced by: eng-review — voz ING E11 — la suite afirma hoy que el botón sale en un usuario activo
  - Files: services/iam/internal/usecase/admin_invite_test.go, services/iam/internal/usecase/admin_resend_test.go, apps/web/app/_screens/admin/AdminScreen.test.tsx
- [ ] **E8 (P1, human: 1.5 d / CC: 40 min) — web** — Ruta /activar: validar token, SetPassword modo token, intercambio de sesión con import diferido del SDK, y fallback a /login con banner de ÉXITO si el intercambio falla
  - Surfaced by: eng-review — voz ING E5 — 'entra directo' estaba decidido pero no diseñado
  - Files: apps/web/app/activar/page.tsx, apps/web/app/auth/provider.ts
- [ ] **C4 (P2, human: 0.5 d / CC: 20 min) — docs** — Runbook de rotación de la credencial SMTP + hito de handover (buzón, DNS, DMARC, dueño de credenciales tras noviembre)
  - Surfaced by: ceo-review — voz CEO V4/F — la conversación con el colegio es obligatoria de todos modos
  - Files: .claude/docs/PLAN-invitaciones-y-reset.md, deploy/README.md
- [ ] **G8 (P2, human: 0.5 d / CC: 15 min) — iam** — Especificar el correo: remitente con nombre visible, asunto, quién invita y con qué rol, vigencia en palabras, botón + URL en texto, versión texto plano
  - Surfaced by: design-review — diseño #10 — es lo primero que ve un docente del sistema
  - Files: services/iam/internal/adapters/smtp/templates.go
- [ ] **G4 (P2, human: 0.5 d / CC: 15 min) — web** — Acciones de fila a IconBtn circulares con aria-label (patrón canónico del sistema de diseño)
  - Surfaced by: design-review — diseño #4 — la fila se rompe por debajo de ~1100px con el botón renombrado
  - Files: apps/web/app/_screens/admin/AdminScreen.tsx, apps/web/app/_design/Controls.tsx
- [ ] **G9 (P2, human: 0.25 d / CC: 10 min) — web** — Reescribir las 6 cadenas en español (vencida, enlace roto, motivo SMTP humano, reset neutro, cuenta no activada, toast con fecha absoluta)
  - Surfaced by: design-review — diseño #7 — copia de programador en pantallas de usuario
  - Files: apps/web/app/_screens/SetPassword.tsx, apps/web/app/_screens/admin/AdminScreen.tsx, apps/web/app/_screens/Login.tsx
- [ ] **G7 (P2, human: 0.5 d / CC: 15 min) — web** — Stat card 'Pendientes de activar' + tab de filtro Invitados (N)
  - Surfaced by: design-review — diseño #9 — responde la única pregunta que el director trae a esta pantalla
  - Files: apps/web/app/_screens/admin/AdminScreen.tsx
- [ ] **G6 (P2, human: 0.5 d / CC: 15 min) — web** — Estados vacíos: tabla sin usuarios, búsqueda sin resultados, y pantalla de validación del token en /activar
  - Surfaced by: design-review — diseño #6 — la Sección 4 decía que el vacío ya existía y no existe
  - Files: apps/web/app/_screens/admin/AdminScreen.tsx, apps/web/app/activar/page.tsx
- [ ] **E7 (P2, human: 0.5 d / CC: 20 min) — contrato** — Añadir a docs/api/openapi.yaml los 3 endpoints nuevos Y los 2 existentes de invitación, que nunca llegaron al contrato
  - Surfaced by: eng-review — regla del repo: contrato primero; hoy el archivo tiene 24 rutas y ninguna de auth
  - Files: docs/api/openapi.yaml
- [ ] **E11 (P2, human: 0.5 d / CC: 15 min) — web** — Conectar '¿Olvidaste tu contraseña?' (hoy href='#' con preventDefault) al endpoint de reset con el mensaje neutro
  - Surfaced by: eng-review — síntoma 2 del dueño
  - Files: apps/web/app/_screens/Login.tsx
- [ ] **E12 (P2, human: 0.25 d / CC: 10 min) — web** — Sustituir el catch genérico de onResend/onInviteUser por mensajes por clase de error (401 = sesión caducada)
  - Surfaced by: eng-review — lección del repo blanket-catch-reports-dead-session-as-domain-error
  - Files: apps/web/app/_screens/admin/AdminScreen.tsx

## Temas que cruzaron fases (señal alta: aparecieron en fases independientes)

- **El fallo silencioso.** CEO: el `200` del proveedor no distingue enviado de perdido (H2)
  y no hay ninguna señal de envío (S8). Diseño: el único error que el director debe atender
  vivía en un toast de 2.6 s y `last_error` no se enseñaba en ninguna pantalla. Ingeniería:
  `userStatusName` devuelve `""` y pinta *Suspendido* a todo invitado, sin un solo error.
  **Tres fases, tres capas distintas, el mismo defecto.** Es el patrón de este sistema, no
  un descuido puntual — y es exactamente el motivo de que el bug del dueño llevara desde el
  7 de septiembre sin que nadie lo notara.
- **La suite codifica el bug.** Diseño e ingeniería llegaron por separado a
  `AdminScreen.test.tsx:98`, que afirma hoy que el botón de reenvío aparece en un usuario
  **activo** — justo lo que el dueño pidió arreglar. Arreglarlo es reescribir aserciones, no
  añadir casos. Es la segunda vez que pasa en este repo (la primera fue el registro del
  command palette).

---

# GATE — Decisiones del dueño (2026-09-08)

| # | Decisión | Elegido | Consecuencia |
|---|---|---|---|
| D1 | Vigencia de la invitación (**reto a la instrucción del dueño**) | **24 h con token propio (Enfoque B)** | El requisito se mantiene tal como se pidió. La tabla `user_invitations`, el hash del token y el vencimiento propio entran al alcance. El enlace de Firebase queda descartado como portador |
| D2 | Permiso `firebaseauth.users.update` | **Cuenta de servicio propia para `iam`** | `iam` estrena `google_service_account.iam_runtime`; el rol `garciaLorcaIdentityRuntime` (con el permiso nuevo) se le concede **sólo a ella**. Los otros nueve servicios siguen con la cuenta compartida y **sin** poder fijar contraseñas |
| D3 | Campos de contraseña en `/activar` | **Un campo con botón de ojo** | Sin campo de confirmación. La red de seguridad es el propio "olvidé mi contraseña" que este PR construye |
| D4 | Aprobación | **Aprobado, empezar** | Orden de arranque: C3 (rescate hoy) → C2 (deriva de registro abierto) → E10 (verificación de SMTP) → el resto tras la credencial |

**Estado del plan: APROBADO.** Alcance congelado en las 25 tareas del listado, con las tres
decisiones de arriba aplicadas.

---

# EJECUCIÓN — Día 0 (2026-09-08)

## C2 — Registro abierto cerrado. **HECHO Y VERIFICADO.**

La config viva traía `client.permissions: {}` mientras terraform declaraba
`disabled_user_signup = true`. Se convergió la config viva a lo que terraform ya decía
(`PATCH v2/projects/garcia-lorca/config`, `updateMask=client.permissions.disabledUserSignup`).

Verificación con la llave pública, que es la que cualquiera puede leer del bundle:

```
POST /v1/accounts:signUp?key=AIza…   →  HTTP 400  ADMIN_ONLY_OPERATION
```

Antes de esto, cualquiera con esa llave podía crear cuentas en el proyecto del colegio y
ocupar direcciones que el colegio aún no había invitado. Ahora no.

## C3 — Los dos usuarios bloqueados. **ENLACES GENERADOS.**

`docentes.cegl@gmail.com` y `pagos@ceglorca.com` llevaban desde el 7 de septiembre creados en
el proveedor, sin contraseña y sin forma de ponérsela. Enlace generado para cada uno con
`returnOobLink`, entregado al dueño para que los reparta. **Vigencia 1 hora** — si caducan, se
regeneran en segundos.

## E10 — Verificación del SMTP del colegio. **PARCIAL, y por qué.**

Hecho desde esta máquina:

```
mail.ceglorca.com:587  →  250-server2471w.entornodevhost.com
                          250-STARTTLS · 250-AUTH PLAIN LOGIN
                          250-LIMITS MAILMAX=1000 RCPTMAX=50000
```

Alcanzable, con STARTTLS y AUTH. **Lo que NO se puede verificar todavía**, y hay que decirlo:

1. **Que la autenticación funcione** — no tenemos credencial. Bloqueado en el colegio.
2. **Que el servidor acepte a Cloud Run** — su IP de salida es variable y los paneles cPanel
   con CSF suelen bloquear autenticaciones desde IPs desconocidas. Sólo se comprueba desde el
   propio Cloud Run, es decir, después del primer despliegue del adaptador.

Receta lista para el minuto en que llegue la credencial (una línea, dos redes distintas):

```bash
python3 - <<'EOF'
import smtplib
s = smtplib.SMTP("mail.ceglorca.com", 587, timeout=10)
s.starttls(); s.login("no-reply@ceglorca.com", "<contraseña>")
s.sendmail("no-reply@ceglorca.com", ["<destino>"],
           "Subject: prueba\r\nFrom: Colegio Garcia Lorca <no-reply@ceglorca.com>\r\n\r\nprueba")
s.quit(); print("OK")
EOF
```

Si falla desde Cloud Run y funciona desde una casa, el diagnóstico es CSF y la decisión es
VPC connector + Cloud NAT con IP reservada, que tiene coste mensual: **go/no-go del dueño**,
no una nota al pie.

## Lo que sigue bloqueado

**Una credencial.** El buzón `no-reply@ceglorca.com` con su contraseña. Sin ella, las 22
tareas restantes se pueden escribir y testear (el puerto `Mailer` se prueba contra un servidor
falso en memoria, sin red), pero **no se pueden verificar en dev ni enseñar en la demo**.
Pedirla es la acción con más apalancamiento del proyecto ahora mismo.

---

# ESTADO — 2026-09-09

## Implementado y verificado corriendo

El ciclo entero funciona en local y el camino que eligió el dueño está probado
contra Firebase de verdad: **el correo llega, y llega a spam**. El dueño lo dio
por bueno para el interino.

| Pieza | Estado |
|---|---|
| Tabla `user_invitations` con `purpose`, `consumed_at` y RLS | hecho |
| Estado `invited` en las cinco capas | hecho |
| Token propio de 24 h, un solo uso, sólo el hash guardado | hecho |
| `/activar` propia, en español, responsive a 390 px | hecho |
| Botón de reenvío sólo en quien no ha entrado | hecho |
| "¿Olvidaste tu contraseña?" conectado, respuesta constante | hecho |
| Envío por el proveedor mientras no haya SMTP | hecho y **verificado con correo recibido** |
| Activación al entrar (cierra la costura del camino de Firebase) | hecho |
| Registro abierto del proyecto cerrado | hecho |

## La decisión del dueño sobre el interino, y su precio

Firebase manda el correo hasta que llegue la credencial del colegio. El precio,
aceptado con los datos en la mano: **el enlace es de Google (una hora), la página
es suya en inglés, y el remitente compartido cae en spam.** Confirmado en la
práctica, no en teoría.

Es temporal **por construcción**: la cadena de entrega es relé propio → proveedor
→ enlace en pantalla, y el primero que funciona gana. El día que existan las
variables SMTP, el relé propio se impone solo. No hay nada que acordarse de apagar.

## Lo que falta para dev

El workflow de despliegue **mueve imágenes, no infraestructura** — está escrito en
su cabecera y es deliberado. Comprobado contra el servicio vivo:

- `IDENTITY_PLATFORM_PROJECT` y `IDENTITY_PLATFORM_API_KEY` **ya están puestos**, así
  que el camino de Firebase funciona con sólo mergear.
- `PUBLIC_WEB_URL` **no está**: el enlace de respaldo se construiría contra
  localhost.
- `iam` sigue con la cuenta compartida, **sin `firebaseauth.users.update`**: nuestra
  `/activar` fallaría con error de permisos.

Ambas resueltas en terraform, y ambas exigen **un `terraform apply` a mano** tras
el merge.

## Lo que sigue abierto

- La credencial de `no-reply@ceglorca.com`. Es lo único que separa el interino del
  estado final.
- `_dmarc.ceglorca.com` en `p=none`. Sin alinear el remitente, nuestro correo
  repetirá el destino del de Google: spam.
