import { AccessibilityInfo, Platform, TextInput } from "react-native";
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

  // QA 21-oct: "Entrar con huella" looked like a way in and only said
  // "disponible pronto". No biometric sign-in exists, so neither does the button.
  it.each(["android", "ios"] as const)("offers no biometric sign-in on %s", async (platform) => {
    setPlatform(platform);
    await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
    await drainMount();

    expect(screen.queryByRole("button", { name: /huella|Face ID/ })).toBeNull();
    expect(screen.queryByText("o")).toBeNull();
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

    it("asks for the address first when the field is empty, sends nothing, and puts the cursor there", async () => {
      const fetchMock = gatewayAnswers(204);
      const focus = jest.spyOn(TextInput.prototype as unknown as { focus: () => void }, "focus");
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();
      focus.mockClear();

      await typeEmailAndAsk("  ");

      expect(screen.getByText("Escribe tu correo para enviarte el enlace.")).toBeTruthy();
      expect(resetCalls(fetchMock)).toHaveLength(0);
      expect(focus).toHaveBeenCalledTimes(1);
      focus.mockRestore();
    });

    // An answer about one address must not stay under a different one.
    it("clears the reset answer once the address changes", async () => {
      gatewayAnswers(204);
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();
      await typeEmailAndAsk("laura@correo.com");
      expect(screen.getByText(NEUTRAL)).toBeTruthy();

      await act(async () => {
        fireEvent.changeText(screen.getByLabelText("Correo electrónico"), "laura@correo.co");
      });

      expect(screen.queryByText(NEUTRAL)).toBeNull();
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

    it("says the link could not be sent when the network fails, never that it was", async () => {
      globalThis.fetch = jest.fn(async (url: string) => {
        if (String(url).includes("/auth/password-reset")) throw new TypeError("Network request failed");
        return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
      }) as unknown as typeof fetch;
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();

      await typeEmailAndAsk("laura@correo.com");

      expect(screen.getByText("No pudimos enviar el enlace. Revisa tu conexión e inténtalo de nuevo.")).toBeTruthy();
      expect(screen.queryByText(NEUTRAL)).toBeNull();
    });

    it("sends one request however many times the link is tapped while it is on its way", async () => {
      let answer: (value: unknown) => void = () => undefined;
      const fetchMock = jest.fn((url: string) =>
        String(url).includes("/auth/password-reset")
          ? new Promise((resolve) => {
              answer = resolve;
            })
          : Promise.resolve({ ok: true, status: 200, json: async () => ({ notifications: [] }) }),
      );
      globalThis.fetch = fetchMock as unknown as typeof fetch;
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();

      await typeEmailAndAsk("laura@correo.com");
      await act(async () => {
        fireEvent.press(screen.getByRole("button", { name: "¿Olvidaste tu contraseña?" }));
      });
      await act(async () => {
        answer({ ok: true, status: 204 });
      });

      expect(resetCalls(fetchMock)).toHaveLength(1);
      expect(screen.getByText(NEUTRAL)).toBeTruthy();
    });

    // The answer is about the address it was asked for; once the parent has
    // typed another one, a late answer must not land under it.
    it("drops an answer that arrives after the address was edited", async () => {
      let answer: (value: unknown) => void = () => undefined;
      globalThis.fetch = jest.fn((url: string) =>
        String(url).includes("/auth/password-reset")
          ? new Promise((resolve) => {
              answer = resolve;
            })
          : Promise.resolve({ ok: true, status: 200, json: async () => ({ notifications: [] }) }),
      ) as unknown as typeof fetch;
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();

      await typeEmailAndAsk("laura@correo.com");
      await act(async () => {
        fireEvent.changeText(screen.getByLabelText("Correo electrónico"), "mario@correo.com");
      });
      await act(async () => {
        answer({ ok: true, status: 204 });
      });

      expect(screen.queryByText(NEUTRAL)).toBeNull();
    });

    // accessibilityLiveRegion is Android-only: on iOS VoiceOver said nothing.
    it.each(["android", "ios"] as const)("announces the answer to a screen reader on %s", async (platform) => {
      setPlatform(platform);
      gatewayAnswers(204);
      const announce = jest.spyOn(AccessibilityInfo, "announceForAccessibility").mockImplementation(() => undefined);
      await renderWithProviders(<LoginScreen onSubmit={async () => undefined} />);
      await drainMount();
      // The preset's AccessibilityInfo is already a mock: drop earlier tests' calls.
      announce.mockClear();

      await typeEmailAndAsk("laura@correo.com");

      expect(announce).toHaveBeenCalledTimes(1);
      expect(announce).toHaveBeenCalledWith(NEUTRAL);
      announce.mockRestore();
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
