# Configurar push real (Expo)

El feed de avisos de la app funciona sin Expo. El envío al teléfono queda apagado (`PUSH_PROVIDER=log`) hasta completar esta guía. No guardes claves ni tokens en Git, `.env` compartidos ni archivos `.tfvars`.

## 1. Crear el proyecto Expo

1. Crea una cuenta en [expo.dev](https://expo.dev/) y una organización o proyecto para el colegio.
2. Desde la raíz del repo ejecuta `cd apps/mobile` y luego `pnpm exec eas init`. Vincula la app existente.
3. Copia el project ID que muestra Expo. Configura `EAS_PROJECT_ID` en el entorno local de builds y en EAS. La app lo publica en `extra.eas.projectId` y construye `updates.url`; sin él el registro de push se omite.

## 2. Configurar las credenciales de plataforma

En `apps/mobile`, ejecuta `pnpm exec eas credentials` y configura:

- **iOS / APNs:** inicia sesión con la cuenta Apple Developer del colegio y deja que EAS gestione la clave APNs o carga la credencial autorizada por el colegio.
- **Android / FCM v1:** en Firebase Console crea o selecciona el proyecto, habilita Firebase Cloud Messaging y registra la app Android. Genera una service account con los permisos de FCM requeridos y entrégala a EAS mediante el flujo interactivo de credenciales. No guardes el JSON en el repo.

## 3. Crear e instalar una build de desarrollo

Con `EAS_PROJECT_ID` disponible en el entorno:

```sh
cd apps/mobile
pnpm exec eas build --profile development --platform ios
# o --platform android
```

Instala el build en un teléfono real desde el enlace que entrega EAS. Expo Go no basta para validar el envío remoto de producción. El perfil `preview` también distribuye internamente; `production` incrementa automáticamente la versión.

## 4. Habilitar Expo en dev

El token de acceso de Expo es opcional mientras el proyecto no habilite enhanced push security. Si lo habilitas, escríbelo directamente a Secret Manager:

```sh
printf '%s' "$EXPO_ACCESS_TOKEN" | gcloud secrets versions add "garcia-lorca-expo-access-token-dev" --data-file=-
```

En `deploy/terraform`, configura `push_provider = "expo"` y `expo_access_token_enabled = true` en el archivo local de variables del entorno dev, y aplica Terraform por el flujo habitual del equipo. Para apagarlo, usa `push_provider = "log"`. Terraform monta `EXPO_ACCESS_TOKEN` desde Secret Manager; el token no va en variables planas ni en Git. No habilites el montaje antes de crear la versión del secreto.

## 5. Probar un aviso

1. Inicia sesión en la app instalada, acepta el permiso de notificaciones y confirma que el dispositivo quedó registrado.
2. Desde la interfaz web crea un aviso/operación que genere una notificación para un alumno con un tutor que tenga esa app instalada. La notificación aparece primero en el feed durable y luego se envía al teléfono.
3. Revisa los logs de `comms` para confirmar que el provider Expo aceptó el envío. Expo devuelve tickets al enviar. El servicio ya elimina tokens que reporta como `DeviceNotRegistered` en esos tickets; la consulta posterior de receipts todavía debe quedar implementada antes de considerar completo el ciclo de limpieza.

Las credenciales APNs/FCM pertenecen al dueño del proyecto Expo y se mantienen en EAS. El token opcional de acceso del servidor pertenece a Secret Manager.
