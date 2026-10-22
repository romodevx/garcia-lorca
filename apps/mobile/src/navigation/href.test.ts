import { hrefOf } from "./href";

describe("hrefOf", () => {
  it.each([
    ["a static route", "/avisos", ["(tabs)", "avisos"], {}, "/avisos"],
    [
      "a dynamic route keeps its query but not the param already in the path",
      "/boleta/r1",
      ["boleta", "[id]"],
      { id: "r1", studentId: "s1" },
      "/boleta/r1?studentId=s1",
    ],
    ["a catch-all route drops its own param", "/modulo/a/b", ["modulo", "[...rest]"], { rest: ["a", "b"], tab: "x" }, "/modulo/a/b?tab=x"],
    ["repeated query values", "/eventos", ["eventos"], { day: ["1", "2"] }, "/eventos?day=1&day=2"],
    ["values that need escaping", "/eventos", ["eventos"], { q: "a b&c=d" }, "/eventos?q=a%20b%26c%3Dd"],
    ["an empty value list", "/eventos", ["eventos"], { day: [] }, "/eventos"],
  ])("%s", (_case, pathname, segments, params, expected) => {
    expect(hrefOf(pathname, segments, params)).toBe(expected);
  });
});
