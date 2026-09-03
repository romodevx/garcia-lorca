# PLAN — Migrar el login a Google Cloud Identity Platform

> Decisión del dueño (2026-09-03), **contra la recomendación**: yo proponía cerrar los
> huecos en el iam propio. Queda registrada la razón de mi objeción al final, no para
> re-litigar sino para que exista si el plan se tuerce.
> Estado: **PLAN — pendiente de checkpoint humano.**

## Por qué se hace

El login propio está bien construido, y eso está medido, no supuesto:

| pieza | estado hoy |
|---|---|
| Hash | bcrypt (KDF adaptativo) |
| Tokens | JWT HMAC · acceso 15 min, refresh 24 h |
| Cookies web | `HttpOnly` + `Secure` + `SameSite=Strict` |
| Móvil | Bearer en el cuerpo, sin cookies |
| Revocación | cambiar contraseña invalida los refresh previos |
| Rate limit | por IP en `/auth/login` |
| Clave de firma | Secret Manager, fatal si falta fuera de local |

Lo que **no** tiene, y por lo que se migra:

1. **No hay recuperación de contraseña.** Los RPC son `Login`, `RefreshToken`,
   `ChangePassword` (exige la actual), `CreateUser`, `SetUserStatus`. **No existe reset
   de administrador.** Una maestra que olvide su contraseña queda fuera sin salida, y ni
   siquiera el director puede reponerla. Con 20 docentes de uso diario, esto ocurre.
2. **Sin bloqueo por cuenta** (el rate limit es por IP: no frena un ataque lento
   dirigido a una cuenta).
3. **Sin auditoría de accesos**, en un sistema con datos de menores.
4. **Sin MFA.**

**Costo: cero.** Identity Platform es gratis hasta 50 000 usuarios activos al mes; el
colegio ronda 1 500 contando tutores. El costo nunca fue el criterio — lo es el trabajo.

## Lo que la migración NO elimina

Hay que decirlo por delante, porque cambia las expectativas:

- **El servicio `iam` sobrevive.** Los permisos viven en Postgres y la **RLS** depende del
  contexto de tenant que sale del token. Identity Platform autentica ("quién eres"); los
  roles y permisos ("qué puedes") siguen siendo nuestros.
- Lo que `iam` deja de hacer es **guardar contraseñas y firmar tokens**. Nada más.
- `Principal` (`middleware/auth.go:12`) seguirá llevando `Permissions` y `TenantID`
  resueltos contra nuestra base, después de verificar el token de Google.

## Lo que abarata el trabajo (hallazgo de la investigación)

`services/gateway/internal/middleware/auth.go:24` define:

```go
type Authenticator interface {
    ValidateToken(ctx context.Context, accessToken string) (Principal, error)
}
```

Es una **interfaz definida por el consumidor** — exactamente el seam que da Clean
Architecture. Hoy la implementa `client/iamauth.go:103` llamando a `iam`. Cambiarla por
"verifica un ID token de Google y resuelve permisos en la base" es **un adaptador nuevo**,
no una reescritura del gateway. `RequireAuth` y todas las rutas quedan intactas.

**Eso baja mi estimación previa de 2-3 días a ~1.5 días CC.**

## El prerrequisito que marqué como bloqueante NO lo es (verificado)

Escribí que el reset por correo podía no funcionar sin buzones reales, y lo di como
bloqueante. **Lo comprobé y no se sostiene.** Los dos dominios aceptan correo:

```
garcialorca.edu.mx   MX = webmail.garcialorca.edu.mx
ceglorca.com         MX = mail.ceglorca.com
```

Y el punto que me faltaba: **Identity Platform envía desde SU propio remitente**
(`noreply@<proyecto>.firebaseapp.com`) hacia la dirección del usuario. Lo único que
importa es que el buzón del destinatario exista, no quién sea dueño del dominio.

Por tanto **no hace falta acceso al dominio para migrar**. Tener `ceglorca.com` sólo
compra dos cosas, ambas cosméticas y aplazables:

- remitente propio (`noreply@ceglorca.com`) en vez del de Firebase, que mejora entregabilidad
  y confianza visual;
- dominio propio para los enlaces de acción del correo.

Se añaden como **F7 (opcional, cuando haya acceso al dominio)**, no como requisito previo.

### Correos: decidido (2026-09-03)

Los sembrados (`director@garcialorca.edu.mx`, `sara@gal.com`) eran **de prueba**. En
producción se usarán direcciones reales con buzón. Para probar el flujo de aquí en
adelante, **yopmail**: buzón desechable, sin registro, recibe al instante — encaja bien
para verificar que el correo de reset sale, llega y el enlace funciona.

> **Una condición, y es de seguridad, no de estilo.** Los buzones de yopmail son
> **públicos**: cualquiera que sepa la dirección lee la bandeja. Un enlace de reset que
> caiga ahí lo puede usar quien sea. Y `dev` está en Internet, no en tu máquina.
>
> Por eso: las cuentas de prueba con yopmail llevan **rol de privilegio bajo, nunca
> `owner`**, en cualquier entorno alcanzable desde fuera. Probar el reset con una cuenta
> de dueño contra un buzón público es entregar el entorno a quien pase por ahí.
> Para probar el reset de un `owner`, usar un buzón real y privado.

## Fases

Ninguna fase enciende nada en producción: todo entra detrás del flag `auth_provider`,
con el camino viejo intacto hasta el corte.

| # | Entregable | Esfuerzo (humano / CC) |
|---|---|---|
| **F1** | Aprovisionar Identity Platform en terraform (proveedor email/password, dominios autorizados). Sin tocar código. | ~0.5 d / **~30 min** |
| **F2** | `GoogleTokenAuthenticator`: verifica el ID token (JWKS de Google, cacheado) y resuelve `Principal` contra Postgres. Implementa la interfaz que ya existe. Tests de tabla con tokens falsos firmados. | ~1 d / **~45 min** |
| **F3** | Migrar los usuarios existentes a Identity Platform. Son 2 en dev; en producción, los que haya. Script idempotente, `must_change_password` en el primer acceso. | ~0.5 d / **~30 min** |
| **F4** | Web: sustituir el formulario propio por el SDK. **Aquí se pierden las cookies `HttpOnly`** — ver riesgo abajo. | ~1 d / **~45 min** |
| **F5** | Móvil (Expo): SDK en lugar de `/auth/mobile/login`. `tokenStore` pasa a delegar en el SDK. | ~0.5 d / **~30 min** |
| **F6** | Flag `auth_provider` + corte + retirada del camino viejo tras una semana en verde. | ~0.5 d / **~30 min** |
| **F7** | *(opcional, cuando haya acceso a `ceglorca.com`)* remitente y dominio de acción propios. No bloquea nada. | ~0.25 d / **~20 min** |

**Total ~4 días humano / ~3 h CC**, más el re-QA completo del login en web y móvil.

## Riesgos, y el que más me preocupa

1. **Se pierde `HttpOnly`.** Hoy los tokens viven **sólo** en cookies que JavaScript no
   puede leer: un XSS no puede robar la sesión. Los SDK de Firebase/Identity Platform
   guardan el token en `IndexedDB`/`localStorage`, **accesible desde JS**. Esto es un
   **retroceso de seguridad real y deliberado**, no un detalle.
   *Mitigación:* mantener el intercambio en el gateway — el cliente manda el ID token de
   Google una vez, el gateway lo verifica y sigue emitiendo **nuestras** cookies
   `HttpOnly`. Conserva la propiedad y usa Identity Platform sólo como verificador de
   credenciales. **Recomiendo esta variante**; añade ~30 min a F4.
2. **La demo es diaria.** Cualquier fallo del login se ve el mismo día. Por eso el flag y
   el corte reversible.
3. **Atadura de proveedor** en la ruta de autenticación.
4. **Los tests de login existentes se reescriben**, igual que pasó con el intent del
   palette: la suite actual codifica el flujo propio.

## Mi objeción, para el registro

Recomendé cerrar los cuatro huecos en el iam propio (~4 h CC) en lugar de migrar
(~3 h CC + 4 días humano + re-QA). El razonamiento: el reset de administrador es el
problema urgente y son ~1 h; migrar no borra el servicio iam ni el modelo de permisos; y
la variante segura de F4 termina reconstruyendo las cookies propias de todos modos.

El dueño decidió migrar. Las razones que lo justifican y que yo no puedo pesar: MFA sin
escribirlo, gestión de sesiones y verificación de correo gratis, y no cargar nunca más
con la responsabilidad de guardar contraseñas.

## Qué NO entra

- Login social (Google/Microsoft) — es aditivo, se enciende después si el colegio lo pide.
- SMS/teléfono — de pago fuera del tramo gratuito.
- Cambiar el modelo de permisos o la RLS. No se tocan.
