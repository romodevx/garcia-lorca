---
name: identity-platform-cutover-2026-09
description: El corte a Identity Platform está HECHO en dev (2026-09-07); qué se encendió, dónde viven los interruptores y cómo se apaga.
metadata:
  type: project
---

**dev entra por Identity Platform desde el 2026-09-07.** Las dos cuentas
(`luibetito@yahoo.com.mx`, `andres.roqa93@gmail.com`) se importaron **con su
contraseña de siempre** vía `iam import-users`; nadie tuvo que resetear nada.

**Los tres interruptores, y ninguno está en el código:**
1. Variables del repo (`gh variable set`): `AUTH_PROVIDER=identity-platform`,
   `FIREBASE_API_KEY`, `FIREBASE_AUTH_DOMAIN`, `FIREBASE_PROJECT_ID`. El workflow
   las pasa como substitutions al build → build-args → `NEXT_PUBLIC_*`. Se
   inlinean en el BUILD: **apagar el proveedor es un rebuild, no un redeploy**.
2. `deploy/terraform/environments/dev.tfvars` (gitignored):
   `identity_platform_api_key` → env de iam, sólo para MANDAR invitaciones.
3. El rol a medida `garciaLorcaIdentityRuntime` sobre la SA de runtime.

**La trampa que costó una hora:** `terraform apply -target=module.iam` NO crea
el rol ni su binding (viven en `identity_platform.tf`, fuera del módulo). Sin
ellos iam arranca diciendo "identity provider wired" y **rechaza todo con 401**,
sin log de error. Si el login por proveedor da 401 en cualquier entorno, mirar
primero `gcloud iam roles describe garciaLorcaIdentityRuntime` antes que el
código.

**Al aplicar terraform, pasar siempre `-var="image_tag=<sha desplegado>"`**: el
default es `latest` y un apply sin él tira la imagen desplegada hacia atrás.

**Grepear el bundle no prueba si el flag está encendido.** El minificador pliega
`"identity-platform" === "identity-platform"` y borra la cadena igual que si
estuviera vacía. Lo que sí prueba: sacar la imagen y buscar la llave real y el
authDomain en `.next/static/chunks/`. Ver [[dev-is-the-live-demo]].
