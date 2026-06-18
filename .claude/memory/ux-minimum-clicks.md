# UX north star — mínimo de clics, todo a un buscador

**Principio rector (binding, del usuario):** **todo debe ser muy fácil de hacer, con el
MENOR número de clics posible.** El usuario no debería navegar menús para ejecutar una
tarea común: la debe poder **buscar y ejecutar** directamente.

**Por qué:** el público (administración + maestros) es no-técnico y trabaja a volumen.
Cada clic ahorrado se multiplica por cientos de operaciones al día. La velocidad de
ejecución es una característica de producto, no un lujo.

**Cómo se aplica (a cada feature, siempre):**
- El **[[command-palette]]** (Cmd+K) es el camino primario: navegar Y ejecutar acciones
  desde un solo lugar. Toda acción frecuente (crear, generar, enviar, cobrar, tomar
  asistencia…) debe ser alcanzable ahí.
- **Acciones primarias de un clic** (botones grandes lima), jerarquía fuerte (lo más usado,
  más grande/arriba), estados vacíos amables, tipografía legible. Ver [[web-frontend-design]].
- **Regla de diseño por feature nueva:** antes de dar por terminada una pantalla o módulo,
  preguntar "¿esto se puede hacer desde el buscador? ¿en cuántos clics?" Si la respuesta no
  es ~2, falta registrar la acción en el palette o simplificar el flujo.
- Mobile (padres): mismo espíritu — pocos taps, acción primaria visible, gestos naturales.
  Ver [[mobile-app-design]].

Esto NO es decoración: es criterio de aceptación. Una feature que obliga a 5 clics donde
2 bastan está incompleta.
