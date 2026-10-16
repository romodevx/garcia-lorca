# Permisos de GCP para desplegar García Lorca en la cuenta del colegio

> Para el administrador de la cuenta de Google Cloud del colegio.
> Lista lo que necesita la persona que despliega (Andrés Romo) para hacer
> **todo** lo que hace hoy el deploy: Terraform, Cloud Build, Cloud Run, base de
> datos, secretos, Pub/Sub, inicio de sesión (Identity Platform), presupuesto y
> corte automático de gasto.
>
> Basado en `deploy/terraform/*.tf`, `deploy/cloudbuild/cloudbuild.yaml` y
> `.github/workflows/`. Fecha: 2026-10-16.

---

## 1. Lo que hay que pedir (la forma corta, recomendada)

Con estos permisos se cubre todo el deploy. Son **tres** asignaciones, en tres
lugares distintos:

| # | Dónde se asigna | Rol | Para qué |
|---|---|---|---|
| 1 | **Proyecto** (el que se cree para García Lorca, uno por ambiente si habrá dev y prod) | `Owner` (`roles/owner`) | Crear y administrar todo dentro del proyecto. |
| 2 | **Cuenta de facturación** del colegio | `Billing Account Administrator` (`roles/billing.admin`) | Ligar el proyecto a la facturación, crear el presupuesto con alertas y darle a la función de corte de gasto el permiso que necesita para apagar el proyecto. |
| 3 | **Organización** (solo si el colegio tiene organización en Google Workspace / Cloud Identity) | `Organization Policy Administrator` (`roles/orgpolicy.policyAdmin`), **o** que el administrador haga la excepción de la sección 4 | El gateway y la web son públicos (`allUsers`). Las organizaciones nuevas lo bloquean por omisión. |

> Si el colegio prefiere no dar `Owner`, la sección 2 lista los roles uno por
> uno. Juntos equivalen a lo mismo para este sistema.
>
> `Owner` del proyecto **no** basta para la facturación: el rol 2 se asigna
> aparte, en la cuenta de facturación (Facturación → Administración de cuentas
> → Agregar principal).

---

## 2. La forma detallada (si no quieren dar `Owner`)

Todos estos roles se asignan **a nivel proyecto**, salvo donde se indica otro
lugar.

### Base del despliegue

| Rol | ID | Por qué lo usa el deploy |
|---|---|---|
| Service Usage Admin | `roles/serviceusage.serviceUsageAdmin` | Habilitar las APIs (lista en la sección 3). |
| Project IAM Admin | `roles/resourcemanager.projectIamAdmin` | Terraform da roles a las cuentas de servicio de cada servicio (`google_project_iam_member`). |
| Service Account Admin | `roles/iam.serviceAccountAdmin` | Crear las 7 cuentas de servicio (gateway, iam, finance…, corte de gasto). |
| Service Account User | `roles/iam.serviceAccountUser` | Desplegar Cloud Run "como" esas cuentas de servicio. |
| Role Administrator | `roles/iam.roleAdmin` | Rol personalizado de Identity Platform (`google_project_iam_custom_role`). |
| Workload Identity Pool Admin | `roles/iam.workloadIdentityPoolAdmin` | Que GitHub Actions despliegue sin llaves (`google_iam_workload_identity_pool`). |
| Storage Admin | `roles/storage.admin` | Bucket del estado de Terraform y los 3 buckets del sistema (recibos, comprobantes, código de la función). |

### Cómputo, imágenes y build

| Rol | ID | Por qué |
|---|---|---|
| Cloud Run Admin | `roles/run.admin` | Los 10 servicios (gateway, web, iam, students, config, finance, grades, attendance, comms, uniforms), `gcloud run deploy/services update`, y dar `run.invoker`. |
| Artifact Registry Administrator | `roles/artifactregistry.admin` | Crear el repositorio de imágenes y subirlas. |
| Cloud Build Editor | `roles/cloudbuild.builds.editor` | `gcloud builds submit / describe / log / cancel`. |
| Cloud Functions Admin | `roles/cloudfunctions.admin` | La función de corte de gasto (gen2). |
| Eventarc Admin | `roles/eventarc.admin` | El disparador de esa función (gen2 usa Eventarc + Pub/Sub). |
| Cloud Scheduler Admin | `roles/cloudscheduler.admin` | Las 2 tareas programadas (cumpleaños, barridos). |

### Datos y mensajería

| Rol | ID | Por qué |
|---|---|---|
| Pub/Sub Admin | `roles/pubsub.admin` | 9 tópicos, 9 suscripciones y sus permisos. |
| Secret Manager Admin | `roles/secretmanager.admin` | 8 secretos (JWT, contraseñas de base, SMTP, token de Expo, …) y sus versiones. |
| Cloud SQL Admin | `roles/cloudsql.admin` | **Solo si se usa Cloud SQL** (`use_cloud_sql = true`). Con Neon como hoy no hace falta. |

### Inicio de sesión, monitoreo y diagnóstico

| Rol | ID | Por qué |
|---|---|---|
| Identity Platform Admin | `roles/identityplatform.admin` | Configurar el inicio de sesión de usuarios (`google_identity_platform_config`). |
| Monitoring Editor | `roles/monitoring.editor` | Canal de notificaciones de las alertas de presupuesto. |
| Logs Viewer | `roles/logging.viewer` | Leer los logs de Cloud Run y Cloud Build para diagnosticar. |

### En la cuenta de facturación (no en el proyecto)

| Rol | ID | Por qué |
|---|---|---|
| Billing Account User | `roles/billing.user` | Ligar el proyecto a la cuenta de facturación. |
| Billing Account Costs Manager | `roles/billing.costsManager` | Crear el presupuesto y sus alertas (`google_billing_budget`). |
| Billing Account Administrator | `roles/billing.admin` | **Solo para el corte automático de gasto:** darle a su cuenta de servicio `roles/billing.projectManager` sobre la cuenta de facturación. Si no lo quieren dar, el administrador del colegio puede correr ese único comando (sección 5) y basta con los dos de arriba. |

---

## 3. APIs que se van a habilitar en el proyecto

Las habilita Terraform (con el rol de Service Usage). Se listan para que el
administrador sepa qué se va a encender:

`run`, `artifactregistry`, `cloudbuild`, `pubsub`, `secretmanager`, `sqladmin`,
`storage`, `iam`, `iamcredentials`, `cloudresourcemanager`, `cloudscheduler`,
`identitytoolkit`, `monitoring`, `cloudfunctions`, `eventarc`,
`cloudbilling`, `billingbudgets` (todas `*.googleapis.com`).

---

## 4. Políticas de la organización que pueden bloquear el deploy

Solo aplica si el proyecto vive dentro de una **organización**. Las
organizaciones creadas desde 2024 traen estas restricciones activas por omisión:

| Política | Qué bloquea | Qué se necesita |
|---|---|---|
| `iam.allowedPolicyMemberDomains` (uso compartido restringido a un dominio) | El gateway y la web necesitan `roles/run.invoker` para `allUsers`; sin eso nadie puede abrir el sistema desde internet. | Una excepción **para este proyecto** que permita `allUsers`, o el rol `roles/orgpolicy.policyAdmin` para hacerla. |
| `iam.disableServiceAccountKeyCreation` | Nada: el deploy no usa llaves de cuenta de servicio (GitHub entra por Workload Identity). | Nada. Se puede dejar activa. |
| `gcp.resourceLocations` (si la configuraron) | Todo se despliega en `us-central1`. | Que `us-central1` esté permitida, o avisar para cambiar la región. |

---

## 5. Lo que el administrador del colegio puede hacer él mismo, en vez de dar permisos

Si prefieren no dar algún rol, estos pasos se pueden hacer una sola vez desde su
lado:

1. **Crear el proyecto** y ligarlo a la cuenta de facturación (sustituye a `roles/billing.user`).
2. **Dar el permiso del corte de gasto** (sustituye a `roles/billing.admin`), después del primer `terraform apply`:
   ```sh
   gcloud billing accounts add-iam-policy-binding BILLING_ACCOUNT_ID \
     --member="serviceAccount:gl-billing-killswitch-<env>@<PROJECT_ID>.iam.gserviceaccount.com" \
     --role="roles/billing.projectManager"
   ```
3. **La excepción de `allUsers`** de la sección 4.

---

## 6. Fuera de Google Cloud (para que el deploy completo funcione)

- **GitHub** (`romodevx/garcia-lorca`): ya se tiene. Hay que poner el proyecto
  nuevo en la configuración de Workload Identity y en las variables del
  workflow.
- **Base de datos Neon** (si se queda Neon): cuenta o proyecto en Neon con rol
  de administrador para crear la base y las credenciales.
- **Correo (SMTP)** para las invitaciones: la credencial del buzón que se vaya
  a usar.
- **Expo / EAS** (push de la app de padres): la cuenta de Expo y su token.
- **Dominio** (si se usará uno propio, p. ej. `sistema.garcialorca.edu.mx`):
  acceso al DNS del dominio para apuntarlo.

---

## Resumen para mandar

> Necesito, para el proyecto de Google Cloud del sistema escolar:
> 1. **Owner** del proyecto.
> 2. **Billing Account Administrator** en la cuenta de facturación del colegio
>    (o, como mínimo, Billing Account User + Billing Account Costs Manager).
> 3. Si hay organización: una excepción a "uso compartido restringido a un
>    dominio" para este proyecto (el sistema es público en internet), o el rol
>    Organization Policy Administrator.
>
> Región: `us-central1`. Correo de la cuenta: el que me vayan a dar de alta.
