# ADR 0041: Familias: una capa que escribe los vínculos tutor→alumno

- Status: Accepted
- Date: 2026-10-23
- Context: la oficina daba acceso a la app **una vez por hijo** (los contactos viven por alumno en `guardian_students`), no había cómo filtrar ni avisar por familia, y un aviso a toda la escuela sonaba una vez por hijo en el celular del papá. Plan: `.claude/docs/PLAN-familias.md`. Se apoya en ADR 0004 (lectura familiar autoalcanzada), 0012 (`family:read` al owner) y 0033 (importación).

## Decisión (del dueño, 23-oct)

1. Una **familia** agrupa hermanos: `families` + `students.family_id`. Un alumno pertenece a **una sola** familia.
2. Las familias se forman con **sugerencias que la oficina confirma**: mismo apellido paterno + materno, o un contacto en común (correo, teléfono o cuenta de la app). Lo descartado no vuelve a sugerirse.
3. **Máximo 2 cuentas de la app por familia**, impuesto por el servidor. Contactos sin cuenta, ilimitados.
4. Un aviso suena **una vez por papá**, aunque tenga varios hijos.
5. La pantalla es una pestaña de Alumnos, no un módulo nuevo del menú.

## Opciones consideradas

- **Familia implícita** (sin tabla: alumnos que comparten tutor). Rechazada: no se puede confirmar ni descartar, no ve a quien aún no tiene cuenta, y el tope de 2 no tiene dónde vivir.
- **Los adultos cuelgan de la familia** y los hijos se leen a través de ella. Rechazada: cambia `ListStudentsForGuardian`, que **es** la autorización de toda ruta `/family/*` (ADR 0004), ya auditada. Mover esa costura por una mejora de captura es riesgo sin ganancia.
- **Híbrida (elegida):** `guardian_students` sigue siendo la única fuente de "qué hijo ve esta cuenta". La familia es la capa que **escribe** esos vínculos de forma consistente.

## Cómo

- **Sincronía.** Toda cuenta ligada a un hijo de la familia queda ligada a **todos** sus hijos colocables (un borrador sin CURP no recibe cuenta). Se ejecuta al confirmar una familia, al agregar un hijo, al dar acceso en la familia y al dar acceso a un contacto de un hijo que ya está en una familia. El contacto que se crea en el hermano copia nombre, teléfono, correo y parentesco del contacto de origen; si el hermano ya tiene un contacto con ese correo, se le pega la cuenta a ese (no se duplica a la persona).
- **Tope de 2.** Cuentas distintas ligadas a los hijos de la familia, contadas en la misma transacción que escribe, con la fila de `families` bloqueada (`SELECT … FOR UPDATE`): dos invitaciones simultáneas no pueden pasar las dos. Una tercera es `KindConflict` (409) y no escribe nada. Juntar hijos cuyas cuentas suman más de 2 también se rechaza.
- **Quitar un hijo de la familia no le quita accesos:** sus papás siguen siendo sus papás. Quitar el acceso de un adulto **en la familia** lo desliga de todos los hijos de la familia.
- **Disolver** deja a los hijos sin familia y no toca ningún vínculo.
- **Descartes.** Firma = SHA-256 de los ids ordenados del grupo sugerido. Un hermano nuevo cambia la firma y la sugerencia vuelve con él.
- **Sugerencias.** Unión-búsqueda sobre los alumnos sin familia (inscritos, aceptados o en trámite): aristas por llave de apellidos normalizada (sin acentos, mayúsculas ni espacios dobles, apellido paterno + materno; los importados viejos que traen ambos en `last_name` producen la misma llave), por correo o teléfono de contacto, y por cuenta de app compartida. Cada sugerencia lleva sus razones; un contacto en común es confianza alta y solo apellidos es media. O(n·α(n)) en memoria, n ≈ 600.
- **Permisos.** Leer: `students:read`. Armar, editar y descartar: `students:update`. Dar o quitar acceso: `guardians:write`, que sigue siendo **solo del owner** (ADR 0012); la familia no abre una vía nueva para ligar cuentas.
- **Flag.** `module_families` (gateway, 404 antes de auth) y exige `module_students`. Apagado, las rutas no existen y `GrantContactAccess` se comporta igual que hoy **salvo el tope**, que es una regla de datos y vale siempre que exista una familia.
- **Avisos.** "Familias" se resuelve a sus alumnos como un grupo. Comms escribe la notificación en el feed de cada alumno, como hoy, pero manda **un push por cuenta** en cada llamada a `Announce`. Un reintento a medias puede sonar una segunda vez para un hijo que no alcanzó: se acepta, porque perder el aviso es peor.

## Consecuencias

- La captura de los papás se hace una vez por familia, no una vez por hijo.
- Un papá con hijos en dos familias distintas (padres separados con hijos de parejas diferentes) es posible: cada familia cuenta su tope por separado.
- La tabla de contactos sigue siendo por alumno, así que el mismo adulto aparece en la ficha de cada hijo. Es a propósito: la ficha de un alumno muestra a quién llamar por **ese** alumno.
