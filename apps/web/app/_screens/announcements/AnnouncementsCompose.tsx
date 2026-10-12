"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { TextArea, TextField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";
import { Page } from "../../_design/Page";
import type { AnnouncementsScreenProps } from "./AnnouncementsScreen";
import { groupSections, levelSections, recipientNames } from "./audience";
import { AudiencePane } from "./AudiencePane";
import { AutomaticMessages } from "./AutomaticMessages";
import { GroupedTargets } from "./GroupedTargets";
import { SendFooter } from "./SendFooter";
import { StudentSearch } from "./StudentSearch";
import { useAnnouncementComposer, type AnnouncementComposer } from "./useAnnouncementComposer";

/** C2: the message box starts four lines tall. */
const MESSAGE_ROWS = 4;

/**
 * Avisos, "Destinatarios al lado" (PLAN-redesign-avisos §10, behind
 * module_announcements_view). Same composer as today's screen, so the same
 * calls; only the layout is new. The <form> IS the two-pane grid, so Enter in
 * Título sends (E3); the birthday greeting has its own form and sits below.
 */
export function AnnouncementsCompose({ onBack }: AnnouncementsScreenProps): ReactElement {
  const composer = useAnnouncementComposer();
  const [attempt, setAttempt] = useState(0);

  const send = (e: FormEvent): void => {
    setAttempt((n) => n + 1);
    void composer.submit(e);
  };

  return (
    <Page variant="reading">
      <button type="button" onClick={onBack} className="announce-back fade-up">
        <Icon name="back" size={18} stroke={2.4} />Volver al inicio
      </button>
      <header className="announce-head fade-up">
        <ModuleHeaderIcon icon="announcements" color="orange" shrinks />
        <div>
          <h1 className="announce-head__title">Avisos</h1>
          <p className="announce-head__lead">Un mensaje para un alumno, un grupo, un nivel o toda la escuela</p>
        </div>
      </header>

      <div className="announce fade-up">
        <form className="announce-grid" onSubmit={send}>
          <AudiencePane scope={composer.scope} onChooseScope={composer.chooseScope} pickedCount={composer.picked.length}>
            <AudienceList composer={composer} />
          </AudiencePane>
          <div className="announce-card announce-editor">
            {/* Never uppercased: the title is what reaches the parents' phones. */}
            <TextField label="Título" value={composer.title} onChange={composer.setTitle} required help="Lo que las familias verán en la lista de avisos, antes de abrirlo." />
            <TextArea
              label="Mensaje"
              value={composer.body}
              onChange={composer.setBody}
              rows={MESSAGE_ROWS}
              required
              help="El aviso completo. Llega tal cual a la app de los padres, así que se escribe como se quiere que se lea."
            />
          </div>
          <SendFooter
            scope={composer.scope}
            recipients={recipientNames(composer.scope, composer.picked, composer)}
            error={composer.error}
            sent={composer.sent}
            sending={composer.sending}
            attempt={attempt}
          />
        </form>
        <AutomaticMessages />
      </div>
    </Page>
  );
}

/** What the chosen audience offers under the four rows. */
function AudienceList({ composer }: { composer: AnnouncementComposer }): ReactElement {
  const { scope, picked, toggle, loaded } = composer;
  if (scope === "cycle") {
    const cycleName = composer.cycle === undefined ? "" : ` · ciclo ${composer.cycle.name}`;
    return <p className="announce-note">Toda la escuela{cycleName}</p>;
  }
  if (scope === "student") {
    return <StudentSearch students={composer.students} picked={picked} onToggle={toggle} loaded={loaded.students} />;
  }
  if (scope === "level") {
    return <GroupedTargets sections={levelSections(composer.levels)} picked={picked} onToggle={toggle} loaded={loaded.levels} emptyText="Todavía no hay niveles." />;
  }
  return <GroupedTargets sections={groupSections(composer.groups, composer.levels)} picked={picked} onToggle={toggle} loaded={loaded.groups} emptyText="Este ciclo no tiene grupos." />;
}
