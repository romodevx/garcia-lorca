import { easProjectConfig } from "./app.config";
import type { ExpoConfig } from "expo/config";

describe("easProjectConfig", () => {
  it("adds the EAS project id and updates URL while preserving existing config", () => {
    const result = easProjectConfig(" project-123 ", {
      name: "School",
      slug: "school",
      updates: { enabled: true },
      extra: { eas: { owner: "owner" }, existing: true },
    } as Partial<ExpoConfig>);
    expect(result.extra).toEqual({ eas: { owner: "owner", projectId: "project-123" }, existing: true });
    expect(result.updates).toEqual({ enabled: true, url: "https://u.expo.dev/project-123" });
  });

  it("leaves current updates behavior untouched when the project id is absent", () => {
    const config = { updates: { enabled: true }, extra: { other: "value" } } as Partial<ExpoConfig>;
    expect(easProjectConfig(undefined, config)).toEqual({ extra: { ...config.extra, eas: {} } });
  });
});
