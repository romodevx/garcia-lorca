import { useState, type ReactElement } from "react";
import { Image, Pressable, StyleSheet, Text, TextInput, View } from "react-native";
import { Icon } from "../icons";
import { useTheme } from "../theme";
import { Button } from "../ui";

export interface LoginScreenProps {
  /**
   * Submits the entered credentials. Resolves on success (the caller navigates);
   * rejects on failure, which this screen surfaces as an inline error. The screen
   * owns the loading + error UI so the caller only wires the network call.
   */
  onSubmit: (email: string, password: string) => Promise<void>;
}

// The official school logo (299×168 lockup). Shown on a white rounded backing so
// the dark-green wordmark reads on any theme background.
const LOGO_ASPECT = 299 / 168;

/** School brandmark: the real logo on a white rounded card. */
function Brandmark({ height = 64 }: { height?: number }): ReactElement {
  return (
    <View style={styles.brand}>
      <Image
        // eslint-disable-next-line @typescript-eslint/no-require-imports -- Metro resolves static image assets via require()
        source={require("../../assets/logo.png")}
        style={{ width: height * LOGO_ASPECT, height }}
        resizeMode="contain"
        accessibilityLabel="Centro Educativo García Lorca"
      />
    </View>
  );
}

// Message shown when a sign-in attempt is rejected. Deliberately generic (never
// "wrong password" vs "no such user") so the screen does not leak which half of
// the credential was wrong — the gateway equalizes login timing for the same
// reason.
const SIGN_IN_ERROR = "No pudimos iniciar sesión. Verifica tu correo y contraseña.";

// Face ID / biometric unlock is out of scope for this pass. The button stays for
// design fidelity but MUST NOT fake a login: it explains it is not yet available
// instead of silently authenticating.
// TODO(pendiente-mobile: biometric-unlock) — gate biometric sign-in on
// expo-local-authentication + a stored refresh token.
const FACE_ID_PENDING = "El acceso con Face ID estará disponible pronto.";

/** Login screen — consistent with the web portal (logo, lime primary, fields). */
export function LoginScreen({ onSubmit }: LoginScreenProps): ReactElement {
  const theme = useTheme();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [show, setShow] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async (): Promise<void> => {
    if (submitting) {
      return;
    }
    setError(null);
    setSubmitting(true);
    try {
      await onSubmit(email.trim(), password);
    } catch {
      setError(SIGN_IN_ERROR);
    } finally {
      setSubmitting(false);
    }
  };

  const fieldStyle = [styles.field, { backgroundColor: theme.colors.surface2 }];

  return (
    <View style={[styles.root, { backgroundColor: theme.colors.bg }]}>
      <View style={styles.brandWrap}>
        <Brandmark />
        <Text style={[styles.subtitle, { color: theme.colors.text3, fontFamily: theme.fonts.ui }]}>
          Portal para padres y tutores
        </Text>
      </View>

      <Text style={[styles.label, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>
        Correo electrónico
      </Text>
      <View style={fieldStyle}>
        <Icon name="user" size={20} color={theme.colors.text3} />
        <TextInput
          accessibilityLabel="Correo electrónico"
          value={email}
          onChangeText={setEmail}
          placeholderTextColor={theme.colors.text3}
          autoCapitalize="none"
          keyboardType="email-address"
          style={[styles.input, { color: theme.colors.text, fontFamily: theme.fonts.uiSemi }]}
        />
      </View>

      <Text style={[styles.label, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra, marginTop: 14 }]}>
        Contraseña
      </Text>
      <View style={fieldStyle}>
        <Icon name="lock" size={20} color={theme.colors.text3} />
        <TextInput
          accessibilityLabel="Contraseña"
          value={password}
          onChangeText={setPassword}
          placeholder="••••••••"
          placeholderTextColor={theme.colors.text3}
          secureTextEntry={!show}
          style={[styles.input, { color: theme.colors.text, fontFamily: theme.fonts.uiSemi }]}
        />
        <Pressable accessibilityRole="button" accessibilityLabel="Mostrar contraseña" onPress={() => setShow((s) => !s)}>
          <Icon name={show ? "eye-off" : "eye"} size={20} color={theme.colors.text3} />
        </Pressable>
      </View>

      <Pressable accessibilityRole="button" style={styles.forgot}>
        <Text style={{ fontSize: 13, fontWeight: "800", color: theme.colors.primaryInk, fontFamily: theme.fonts.uiExtra }}>
          ¿Olvidaste tu contraseña?
        </Text>
      </Pressable>

      {error ? (
        <Text
          accessibilityRole="alert"
          style={[styles.error, { color: theme.colors.bad, fontFamily: theme.fonts.uiSemi }]}
        >
          {error}
        </Text>
      ) : null}

      <Button label="Ingresar" icon="arrow-right" onPress={submit} loading={submitting} />

      <View style={styles.divider}>
        <View style={[styles.line, { backgroundColor: theme.colors.line }]} />
        <Text style={{ fontSize: 12, fontWeight: "700", color: theme.colors.text3, fontFamily: theme.fonts.ui }}>o</Text>
        <View style={[styles.line, { backgroundColor: theme.colors.line }]} />
      </View>

      <Button
        label="Entrar con Face ID"
        variant="outline"
        icon="lock"
        onPress={() => setError(FACE_ID_PENDING)}
      />

      <Text style={[styles.help, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>
        ¿Problemas para entrar? Contacta a la escuela
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flexGrow: 1, justifyContent: "center", paddingHorizontal: 20, paddingVertical: 30 },
  brandWrap: { alignItems: "center", marginBottom: 26 },
  brand: {
    backgroundColor: "#FFFFFF",
    borderRadius: 18,
    paddingVertical: 12,
    paddingHorizontal: 20,
    alignItems: "center",
    justifyContent: "center",
  },
  subtitle: { fontSize: 13.5, fontWeight: "700", marginTop: 14 },
  label: { fontSize: 12.5, fontWeight: "800", marginHorizontal: 4, marginBottom: 7 },
  field: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    height: 54,
    paddingHorizontal: 16,
    borderRadius: 14,
  },
  input: { flex: 1, fontSize: 16, fontWeight: "600", padding: 0 },
  forgot: { alignSelf: "flex-end", marginVertical: 12, marginRight: 4 },
  error: { textAlign: "center", fontSize: 13, fontWeight: "700", marginBottom: 12, marginHorizontal: 4 },
  divider: { flexDirection: "row", alignItems: "center", gap: 12, marginVertical: 18 },
  line: { flex: 1, height: 1 },
  help: { textAlign: "center", fontSize: 12.5, fontWeight: "700", marginTop: 22 },
});
