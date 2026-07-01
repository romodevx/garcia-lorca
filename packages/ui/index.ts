/**
 * @garcia-lorca/ui — shared design-system components + feature-flag primitives.
 *
 * The flag primitives (FlagsProvider / useFlag / FeatureGate) are
 * presentation-agnostic (no DOM or React Native imports) so both apps/web and
 * apps/mobile share one gating implementation. DOM-specific components (Button)
 * are web-only.
 */
export * from "./src/flags";
export { Button, type ButtonProps } from "./src/components/Button";

// Command Palette — shell + platform-agnostic match core.
export {
  CommandPalette,
  type CommandPaletteLabels,
  type CommandPaletteProps,
} from "./src/command/CommandPalette";
export { matchCommands } from "./src/command/match";
export { fold } from "./src/command/fold";
export {
  type Command,
  type CommandKind,
  type ScoredCommand,
  type MatchOptions,
  KIND_ORDER,
} from "./src/command/types";
