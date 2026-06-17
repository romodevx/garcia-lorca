/**
 * FormatJS locale-data entry points ship plain JS with no type declarations.
 * They are pure side-effect imports — each registers CLDR plural data onto the
 * polyfilled `Intl`. This ambient declaration lets the side-effect imports in
 * `intl-polyfill.ts` type-check without pulling in non-existent typings.
 */
declare module "@formatjs/intl-pluralrules/locale-data/*";
