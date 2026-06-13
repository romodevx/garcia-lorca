import { useState, type ReactElement } from "react";
import { Pressable, StyleSheet, Text, TextInput, View } from "react-native";
import Svg, { Path } from "react-native-svg";
import { Icon } from "../icons";
import { useTheme } from "../theme";
import { Button } from "../ui";

export interface LoginScreenProps {
  /** Invoked on a successful sign-in (real auth lands with the IAM module). */
  onLogin: () => void;
}

/** School brandmark: lime→forest rounded tile with a faint leaf glyph + "GL". */
function Brandmark({ size = 76 }: { size?: number }): ReactElement {
  const theme = useTheme();
  return (
    <View
      style={[
        styles.brand,
        { width: size, height: size, borderRadius: size * 0.32, backgroundColor: theme.palette.lime },
      ]}
    >
      <View style={[StyleSheet.absoluteFill, { backgroundColor: theme.palette.forest, opacity: 0.55, borderRadius: size * 0.32 }]} />
      <Svg width={size} height={size} viewBox="0 0 76 76" style={[StyleSheet.absoluteFill, { opacity: 0.22 }]}>
        <Path d="M16 60s-3-22 13-34 33-9 33-9 1.5 22-14 34S16 60 16 60Z" fill="#fff" />
      </Svg>
      <Text style={[styles.brandText, { fontSize: size * 0.42, fontFamily: theme.fonts.display }]}>GL</Text>
    </View>
  );
}

/** Login screen — consistent with the web portal (logo, lime primary, fields). */
export function LoginScreen({ onLogin }: LoginScreenProps): ReactElement {
  const theme = useTheme();
  const [email, setEmail] = useState("laura.diaz@gmail.com");
  const [password, setPassword] = useState("123456");
  const [show, setShow] = useState(false);

  const fieldStyle = [styles.field, { backgroundColor: theme.colors.surface2 }];

  return (
    <View style={[styles.root, { backgroundColor: theme.colors.bg }]}>
      <View style={styles.brandWrap}>
        <Brandmark />
        <Text style={[styles.heading, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>
          Centro Educativo{"\n"}García Lorca
        </Text>
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
          placeholder="tu@correo.com"
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

      <Button label="Ingresar" icon="arrow-right" onPress={onLogin} />

      <View style={styles.divider}>
        <View style={[styles.line, { backgroundColor: theme.colors.line }]} />
        <Text style={{ fontSize: 12, fontWeight: "700", color: theme.colors.text3, fontFamily: theme.fonts.ui }}>o</Text>
        <View style={[styles.line, { backgroundColor: theme.colors.line }]} />
      </View>

      <Button label="Entrar con Face ID" variant="outline" icon="lock" onPress={onLogin} />

      <Text style={[styles.help, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>
        ¿Problemas para entrar? Contacta a la escuela
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flexGrow: 1, justifyContent: "center", paddingHorizontal: 20, paddingVertical: 30 },
  brandWrap: { alignItems: "center", marginBottom: 26 },
  brand: { alignItems: "center", justifyContent: "center", overflow: "hidden" },
  brandText: { color: "#FFFFFF", fontWeight: "700", letterSpacing: 1 },
  heading: { textAlign: "center", marginTop: 18, fontSize: 23, fontWeight: "700", lineHeight: 28, letterSpacing: -0.4 },
  subtitle: { fontSize: 13.5, fontWeight: "700", marginTop: 8 },
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
  divider: { flexDirection: "row", alignItems: "center", gap: 12, marginVertical: 18 },
  line: { flex: 1, height: 1 },
  help: { textAlign: "center", fontSize: 12.5, fontWeight: "700", marginTop: 22 },
});
