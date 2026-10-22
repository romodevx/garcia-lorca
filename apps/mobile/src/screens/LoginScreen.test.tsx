import { Platform } from "react-native";
import { act, fireEvent, screen } from "@testing-library/react-native";
import { LoginScreen } from "./LoginScreen";
import { renderWithProviders } from "../test/renderWithProviders";

const platformDescriptor = Object.getOwnPropertyDescriptor(Platform, "OS");

function setPlatform(platform: "android" | "ios"): void {
  Object.defineProperty(Platform, "OS", { configurable: true, value: platform });
}

afterEach(() => {
  if (platformDescriptor) Object.defineProperty(Platform, "OS", platformDescriptor);
});

// The LoginScreen renders inside the full provider stack, whose AppState fetches
// family notifications on mount. Draining that promise chain (a macrotask flush)
// before interacting settles the background fetch so it neither overlaps the
// interaction's act() nor leaks a late update into the next test.
async function drainMount(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => {
      setTimeout(() => resolve(), 0);
    });
  });
}

describe("LoginScreen", () => {
  it("renders the portal branding", async () => {
    await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
    await drainMount();
    expect(screen.getByText(/Portal para padres y tutores/)).toBeTruthy();
  });

  it("submits the typed credentials via the Ingresar button", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    await renderWithProviders(<LoginScreen onSubmit={onSubmit} />);
    await drainMount();

    // React 19 + RNTL v14 flush controlled-input and async-handler updates
    // asynchronously. Every interaction runs inside act() so the typed state is
    // committed before the submit reads it and the async submit — including its
    // trailing `finally` (setSubmitting false) — settles fully and cannot leak
    // into the next test. (A bare fireEvent + waitFor between the two steps leaves
    // a pending update that corrupts the following render under this stack.)
    await act(async () => {
      fireEvent.changeText(screen.getByLabelText("Correo electrónico"), "laura@correo.com");
      fireEvent.changeText(screen.getByPlaceholderText("••••••••"), "secret123");
    });
    await act(async () => {
      fireEvent.press(screen.getByRole("button", { name: "Ingresar" }));
    });

    expect(onSubmit).toHaveBeenCalledWith("laura@correo.com", "secret123");
  });

  it("shows an error message when the submit is rejected", async () => {
    const onSubmit = jest.fn().mockRejectedValue(new Error("bad creds"));
    await renderWithProviders(<LoginScreen onSubmit={onSubmit} />);
    await drainMount();

    await act(async () => {
      fireEvent.press(screen.getByRole("button", { name: "Ingresar" }));
    });

    expect(screen.getByText(/No pudimos iniciar sesión/)).toBeTruthy();
  });

  it("shows fingerprint wording on Android", async () => {
    setPlatform("android");
    await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
    await drainMount();

    expect(screen.getByRole("button", { name: "Entrar con huella" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Entrar con Face ID" })).toBeNull();
  });

  it("does not submit credentials via Face ID (out of scope, not a silent login)", async () => {
    setPlatform("ios");
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    await renderWithProviders(<LoginScreen onSubmit={onSubmit} />);
    await drainMount();

    fireEvent.press(screen.getByRole("button", { name: "Entrar con Face ID" }));

    expect(onSubmit).not.toHaveBeenCalled();
  });

  // Not on every keystroke: rewriting a controlled TextInput mid-composition
  // duplicates characters on Android keyboards that compose (Samsung, predictive
  // text on). The field lowercases when the parent leaves it, and the submit
  // sends the lowercase address whatever the field shows.
  it("lowercases the email when the field is left", async () => {
    await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
    await drainMount();

    const field = screen.getByLabelText("Correo electrónico");
    await act(async () => {
      fireEvent.changeText(field, "Laura.Ruiz@Correo.COM");
    });
    // While typing, the field holds exactly what the keyboard sent.
    expect(screen.getByLabelText("Correo electrónico").props.value).toBe("Laura.Ruiz@Correo.COM");
    await act(async () => {
      fireEvent(field, "blur");
    });

    expect(screen.getByLabelText("Correo electrónico").props.value).toBe("laura.ruiz@correo.com");
  });

  it("submits the email lowercase even if the field was never left", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    await renderWithProviders(<LoginScreen onSubmit={onSubmit} />);
    await drainMount();

    await act(async () => {
      fireEvent.changeText(screen.getByLabelText("Correo electrónico"), " Laura@Correo.COM ");
      fireEvent.changeText(screen.getByPlaceholderText("••••••••"), "secret123");
    });
    await act(async () => {
      fireEvent.press(screen.getByRole("button", { name: "Ingresar" }));
    });

    expect(onSubmit).toHaveBeenCalledWith("laura@correo.com", "secret123");
  });

  // QA 21-oct: "¿Olvidaste tu contraseña?" had no onPress.
  describe("forgotten password", () => {
    const NEUTRAL = "Si el correo está registrado, te enviamos un enlace para crear una contraseña nueva.";

    function gatewayAnswers(status: number): jest.Mock {
      const fetchMock = jest.fn(async (url: string) =>
        String(url).includes("/auth/password-reset")
          ? { ok: status < 300, status, json: async () => ({}) }
          : { ok: true, status: 200, json: async () => ({ notifications: [] }) },
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;
      return fetchMock;
    }
    const resetCalls = (fetchMock: jest.Mock): unknown[][] =>
      fetchMock.mock.calls.filter(([u]) => String(u).includes("/auth/password-reset"));

    async function typeEmailAndAsk(email: string): Promise<void> {
      await act(async () => {
        fireEvent.changeText(screen.getByLabelText("Correo electrónico"), email);
      });
      await act(async () => {
        fireEvent.press(screen.getByRole("button", { name: "¿Olvidaste tu contraseña?" }));
      });
    }

    it("asks for the address first when the field is empty, and sends nothing", async () => {
      const fetchMock = gatewayAnswers(204);
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();

      await typeEmailAndAsk("  ");

      expect(screen.getByText("Escribe tu correo para enviarte el enlace")).toBeTruthy();
      expect(resetCalls(fetchMock)).toHaveLength(0);
    });

    it("sends the typed address, lowercase, and answers with the same neutral sentence", async () => {
      const fetchMock = gatewayAnswers(204);
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();

      await typeEmailAndAsk(" Laura@Correo.COM ");

      const [, init] = resetCalls(fetchMock)[0] as [string, RequestInit];
      expect(JSON.parse(String(init.body))).toEqual({ email: "laura@correo.com" });
      expect(screen.getByText(NEUTRAL)).toBeTruthy();
    });

    it("says to wait a few minutes when the gateway rate-limits", async () => {
      gatewayAnswers(429);
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();

      await typeEmailAndAsk("laura@correo.com");

      expect(screen.getByText("Intenta de nuevo en unos minutos.")).toBeTruthy();
      expect(screen.queryByText(NEUTRAL)).toBeNull();
    });
  });
});
