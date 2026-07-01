"use client";

import { CommandPalette, type Command, type CommandKind } from "@garcia-lorca/ui";
import { useFlags } from "@garcia-lorca/ui/flags";
import { useTranslations } from "next-intl";
import { type ReactNode, useCallback, useEffect, useMemo, useState } from "react";

import { Icon, type IconName } from "../_design/Icon";
import { COLOR, type ColorKey } from "../_design/colors";
import { buildCommandRegistry } from "./registry";
import { pushRecent, RECENTS_KEY } from "./recents";

/**
 * Web host for the Command Palette. Assembles the flag-filtered Layer A registry,
 * localizes it (next-intl `palette` namespace), tracks recents (command IDs
 * only), renders design-system icons, and maps a selected command's route to the
 * app's navigation. Opens on Cmd/Ctrl+K or when the TopBar trigger calls open.
 *
 *   flags ─> buildCommandRegistry(t) ─┐
 *   recents (localStorage, ids only) ─┴─> matchCommands ─> cmdk ─> onNavigate(route)
 */
export interface CommandPaletteHostProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  /** Navigate to a command's route (e.g. "module:students"). Never mutates. */
  readonly onNavigate: (route: string) => void;
}

const RECENT_PREFIX = "recent:";

export function CommandPaletteHost({ open, onOpenChange, onNavigate }: CommandPaletteHostProps): ReactNode {
  const flags = useFlags();
  const t = useTranslations("palette");
  const [recents, setRecents] = useState<string[]>([]);

  // Global Cmd/Ctrl+K shortcut.
  useEffect(() => {
    const onKey = (event: KeyboardEvent): void => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        onOpenChange(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onOpenChange]);

  // Load recents when the palette opens (fresh read; ids only).
  useEffect(() => {
    if (!open) return;
    try {
      const raw = window.localStorage.getItem(RECENTS_KEY);
      setRecents(raw !== null ? (JSON.parse(raw) as string[]) : []);
    } catch {
      setRecents([]);
    }
  }, [open]);

  const base = useMemo(() => buildCommandRegistry(flags, (key) => t(key)), [flags, t]);

  // Recent commands: resolve stored ids against the flag-filtered registry, so a
  // command whose module is now disabled simply drops (no leak). Prefixed id
  // keeps them distinct from their canonical entry for keyboard nav.
  const commands = useMemo<Command[]>(() => {
    const byId = new Map(base.map((command) => [command.id, command]));
    const recentCommands: Command[] = recents
      .map((id) => byId.get(id))
      .filter((command): command is Command => command !== undefined)
      .map((command) => ({ ...command, id: `${RECENT_PREFIX}${command.id}`, kind: "recent" as CommandKind }));
    return [...recentCommands, ...base];
  }, [base, recents]);

  const labels = useMemo(
    () => ({
      placeholder: t("ui.placeholder"),
      empty: t("ui.empty"),
      groups: {
        module: t("group.module"),
        create: t("group.create"),
        recent: t("group.recent"),
        record: t("group.record"),
        quick: t("group.quick"),
      } satisfies Record<CommandKind, string>,
    }),
    [t],
  );

  const handleSelect = useCallback((command: Command): void => {
    const canonicalId = command.id.startsWith(RECENT_PREFIX)
      ? command.id.slice(RECENT_PREFIX.length)
      : command.id;
    setRecents((current) => {
      const next = pushRecent(current, canonicalId);
      try {
        window.localStorage.setItem(RECENTS_KEY, JSON.stringify(next));
      } catch {
        // Persisting recents is best-effort (private mode); ignore.
      }
      return next;
    });
    onNavigate(command.route);
  }, [onNavigate]);

  const renderIcon = useCallback((icon: string, color?: string): ReactNode => {
    const token = COLOR[(color as ColorKey) ?? "neutral"] ?? COLOR.neutral;
    return (
      <span
        aria-hidden
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          width: 30,
          height: 30,
          borderRadius: 9,
          background: token.v,
          color: token.ink,
          flex: "0 0 auto",
        }}
      >
        <Icon name={icon as IconName} size={17} />
      </span>
    );
  }, []);

  return (
    <CommandPalette
      open={open}
      onOpenChange={onOpenChange}
      commands={commands}
      labels={labels}
      onSelect={handleSelect}
      renderIcon={renderIcon}
    />
  );
}
