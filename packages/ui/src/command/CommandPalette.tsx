"use client";

import * as Dialog from "@radix-ui/react-dialog";
import { Command } from "cmdk";
import { type ReactNode, useMemo, useState } from "react";

import { matchCommands } from "./match";
import { type Command as GLCommand, type CommandKind, KIND_ORDER } from "./types";

/**
 * CommandPalette — the "todo a un buscador" surface. Built on `cmdk` (proven
 * keyboard/focus/ARIA), but ranking + grouping + accent-folding come from our
 * own `matchCommands` core (cmdk's own filter is disabled). Presentation-only:
 * selecting a command hands it back via `onSelect`; the app NAVIGATES to its
 * route (never mutates from here).
 *
 *   query ──> matchCommands (fold+fuzzy+kind-rank) ──> groups by kind ──> cmdk items
 */

/** Read by assistive tech, not drawn: the screen already says it. */
const VISUALLY_HIDDEN = {
  position: "absolute",
  width: 1,
  height: 1,
  margin: -1,
  padding: 0,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
  border: 0,
} as const;

export interface CommandPaletteLabels {
  readonly placeholder: string;
  readonly empty: string;
  /**
   * Shown under `empty`. "No results" on its own is a dead end, and it arrives
   * at the moment the user is most ready to be told what this box is for.
   */
  readonly hint?: string;
  /** Heading per kind, e.g. { module: "Módulos", create: "Crear", … }. */
  readonly groups: Record<CommandKind, string>;
}

export interface CommandPaletteProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly commands: readonly GLCommand[];
  readonly labels: CommandPaletteLabels;
  /** Called with the selected command; the caller navigates to its route. */
  readonly onSelect: (command: GLCommand) => void;
  /** Optional icon renderer (app supplies its design-system icon). */
  readonly renderIcon?: (icon: string, color?: string) => ReactNode;
  /** Controlled query (optional); uncontrolled otherwise. */
  readonly query?: string;
  readonly onQueryChange?: (query: string) => void;
  readonly limit?: number;
}

/** Kinds in display order, filtered to those with results. */
const KINDS_IN_ORDER = (Object.keys(KIND_ORDER) as CommandKind[]).sort(
  (a, b) => KIND_ORDER[a] - KIND_ORDER[b],
);

export function CommandPalette({
  open,
  onOpenChange,
  commands,
  labels,
  onSelect,
  renderIcon,
  query,
  onQueryChange,
  limit = 10,
}: CommandPaletteProps): ReactNode {
  // Controlled when `query` is provided; otherwise manage it internally.
  const [internalQuery, setInternalQuery] = useState("");
  const activeQuery = query ?? internalQuery;
  const setQuery = (next: string): void => {
    if (query === undefined) setInternalQuery(next);
    onQueryChange?.(next);
  };

  const results = useMemo(
    () => matchCommands(commands, activeQuery, { limit }),
    [commands, activeQuery, limit],
  );

  const byKind = useMemo(() => {
    const map = new Map<CommandKind, GLCommand[]>();
    for (const { command } of results) {
      const list = map.get(command.kind) ?? [];
      list.push(command);
      map.set(command.kind, list);
    }
    return map;
  }, [results]);

  const choose = (command: GLCommand): void => {
    onSelect(command);
    onOpenChange(false);
  };

  return (
    <Command.Dialog
      open={open}
      onOpenChange={onOpenChange}
      shouldFilter={false}
      label={labels.placeholder}
    >
      {/*
        Radix labels its dialog by REFERENCE: it puts `aria-labelledby` on the
        content pointing at the id a `Dialog.Title` would claim. cmdk renders
        none, so that attribute pointed at nothing — and since aria-labelledby
        outranks aria-label, the dialog carried a broken label that overrode the
        working one cmdk sets from `label`. Rendering the Title (kept off-screen,
        because the input's placeholder already says this on screen) is what
        makes the reference resolve.
      */}
      <Dialog.Title style={VISUALLY_HIDDEN}>
        {labels.placeholder}
      </Dialog.Title>
      {/*
        Radix also points `aria-describedby` at a Description and warns when
        there is none (ISSUE-T14-021). cmdk does not forward props to the
        content, so the description is rendered, off-screen: the hint says what
        the box is for; without one, the placeholder does.
      */}
      <Dialog.Description style={VISUALLY_HIDDEN}>{labels.hint ?? labels.placeholder}</Dialog.Description>
      <Command.Input
        placeholder={labels.placeholder}
        value={activeQuery}
        onValueChange={setQuery}
      />
      <Command.List>
        {results.length === 0 ? (
          <Command.Empty>
            {labels.empty}
            {labels.hint !== undefined ? <span data-hint>{labels.hint}</span> : null}
          </Command.Empty>
        ) : (
          KINDS_IN_ORDER.filter((kind) => byKind.has(kind)).map((kind) => (
            <Command.Group key={kind} heading={labels.groups[kind]}>
              {(byKind.get(kind) ?? []).map((command) => (
                <Command.Item key={command.id} value={command.id} onSelect={() => choose(command)}>
                  {renderIcon?.(command.icon, command.color)}
                  <span>{command.title}</span>
                  {command.subtitle !== undefined ? <span data-subtitle>{command.subtitle}</span> : null}
                </Command.Item>
              ))}
            </Command.Group>
          ))
        )}
      </Command.List>
    </Command.Dialog>
  );
}
