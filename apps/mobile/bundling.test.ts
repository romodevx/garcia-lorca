import { readFileSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { join, relative, resolve } from "node:path";

/**
 * Guards the one thing 128 green unit tests could not: that the app actually
 * BUNDLES.
 *
 * `react-native-svg` publishes `"react-native": "src/index.ts"`, so Metro
 * compiles its TypeScript source rather than the shipped CommonJS build. That
 * source imports `buffer`, which the package does not declare as a dependency.
 * Under npm's flat layout the import resolves by accident off some other
 * package's copy; under pnpm's strict store there is no accident to rely on and
 * Metro answers `UnableToResolveError: buffer`, which fails the bundle before a
 * single screen renders.
 *
 * So `buffer` is OUR declared dependency: the app really does need it at
 * runtime, and saying so out loud is what makes it resolvable.
 */
describe("bundle resolution", () => {
  it("resolves the `buffer` PACKAGE, which react-native-svg's source imports but does not declare", () => {
    // `buffer/package.json`, not bare `buffer`: the bare specifier resolves to
    // Node's built-in module, which Metro does not have and so proves nothing.
    const requireFromApp = createRequire(resolve(__dirname, "package.json"));
    expect(() => requireFromApp.resolve("buffer/package.json")).not.toThrow();
  });

  it("declares `buffer` as a dependency, so the resolution is not a hoisting accident", () => {
    const pkg = JSON.parse(readFileSync(resolve(__dirname, "package.json"), "utf8")) as {
      dependencies: Record<string, string>;
    };
    expect(pkg.dependencies).toHaveProperty("buffer");
  });

  /**
   * expo-router turns EVERY `.ts`/`.tsx` file under `app/` into a route — its
   * context regex (expo-router/_ctx.ios.js) excludes only `+api`, `+html` and
   * `+middleware`. A test file colocated there is therefore not merely bundled,
   * it is a screen: it drags `@testing-library/react-native` into the graph,
   * which fails the dev bundle outright and, worse, would ship the testing
   * library and the test code itself inside a production build.
   *
   * Route tests live outside `app/` for that reason. See `__tests__/routes/`.
   */
  it("keeps test files out of app/, where every file is a route", () => {
    const appRoot = resolve(__dirname, "app");
    const walk = (dir: string): string[] =>
      readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
        e.isDirectory() ? walk(join(dir, e.name)) : [join(dir, e.name)],
      );
    const colocated = walk(appRoot)
      .filter((f) => /\.(test|spec)\.[tj]sx?$/.test(f))
      .map((f) => relative(appRoot, f));
    expect(colocated).toEqual([]);
  });
});