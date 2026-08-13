import { type ReactElement } from "react";
import { Card, EmptyState } from "../ui";

/**
 * Eventos: the agenda list (reached from the Inicio "próximo evento" tile).
 *
 * The comms agenda has no backend yet (deferred to Phase 9d). The mock `events`
 * in `mock.ts` are intentionally NOT rendered — showing fake agenda items would
 * mislead the parent. A neutral "Próximamente" state stands in until the real
 * endpoint lands.
 */
export function EventsScreen(): ReactElement {
  return (
    <Card>
      <EmptyState
        icon="calendar"
        title="Próximamente"
        subtitle="La agenda de eventos estará disponible pronto."
      />
    </Card>
  );
}
