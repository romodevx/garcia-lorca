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
