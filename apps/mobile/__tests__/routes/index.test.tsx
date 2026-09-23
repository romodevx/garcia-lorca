import { act, fireEvent, screen } from "@testing-library/react-native";
import IndexRoute from "../../app/index";
import { renderWithProviders } from "@/src/test/renderWithProviders";

// signIn — the route's actual dependency since the provider path landed — is
// mocked to succeed AND persist a token pair, so the post-login notifications
// fetch (FIX 2b) has an access token to authenticate with. Mocking `login`
// instead would not intercept anything: signIn imports it directly, not through
// this barrel.
jest.mock("@/src/api", () => {
  const actual = jest.requireActual("@/src/api");
  return {
    __esModule: true,
    ...actual,
    signIn: jest.fn(async () => {
      await actual.saveTokens({ accessToken: "a1", refreshToken: "r1" });
      return { mustChangePassword: false };
    }),
  };
});

const { clearTokens } = jest.requireActual("@/src/api") as typeof import("@/src/api");

function familyFetchCalls(mock: jest.Mock): unknown[] {
  return mock.mock.calls.filter(([url]) => String(url).includes("/api/v1/family/notifications"));
}

describe("IndexRoute (login → notifications)", () => {
  beforeEach(async () => {
    await clearTokens();
  });

  afterEach(async () => {
    await clearTokens();
  });

  it("fetches the family notifications after a successful login", async () => {
    const fetchMock = jest
      .fn()
      .mockResolvedValue({ ok: true, status: 200, json: async () => ({ notifications: [] }) });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<IndexRoute />);

    // On mount there is no token, so no request is fired.
    expect(familyFetchCalls(fetchMock)).toHaveLength(0);

    await act(async () => {
      fireEvent.changeText(screen.getByLabelText("Correo electrónico"), "laura@correo.com");
      fireEvent.changeText(screen.getByPlaceholderText("••••••••"), "secret123");
    });
    await act(async () => {
      fireEvent.press(screen.getByRole("button", { name: "Ingresar" }));
    });

    expect(familyFetchCalls(fetchMock).length).toBeGreaterThan(0);
  });
});
