# @garcia-lorca/ui

Shared design-system components and the feature-flag gating primitives.

## Feature-flag gating (the ONLY flag API)

```tsx
import { FlagsProvider, FeatureGate, useFlag } from "@garcia-lorca/ui/flags";

// at the app root — flags fetched once on boot from gateway `/features`
<FlagsProvider flags={flags}>
  <App />
</FlagsProvider>;

// declarative gate
<FeatureGate flag="module_attendance">
  <AttendanceWidget />
</FeatureGate>;

// imperative
const enabled = useFlag("module_grades");
```

UX-ONLY. The backend gateway is the security boundary (disabled module routes
return 404). Disabled module route chunks must also be lazy-loaded so they are
never downloaded. `FlagKey` mirrors the registry in `FLAGS.md`.
