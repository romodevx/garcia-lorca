/**
 * Command Palette CORE — platform-agnostic entry point (NO cmdk / DOM).
 *
 * apps/mobile imports this subpath (`@garcia-lorca/ui/command-core`) to reuse the
 * exact matching + types on React Native, pairing them with a native modal.
 * apps/web gets the same core plus the cmdk `CommandPalette` from the root entry.
 */
export { fold } from "./fold";
export { matchCommands } from "./match";
export {
  type Command,
  type CommandKind,
  type MatchOptions,
  type ScoredCommand,
  KIND_ORDER,
} from "./types";
