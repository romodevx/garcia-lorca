import AsyncStorage from "@react-native-async-storage/async-storage";
import { fireEvent, render, screen, waitFor } from "@testing-library/react-native";
import { type ReactElement } from "react";
import { Pressable, Text } from "react-native";
import { ThemeProvider, useTheme, useThemeControls } from "./ThemeProvider";

function Probe(): ReactElement {
  const theme = useTheme();
  const { toggleTheme } = useThemeControls();
  return (
    <Pressable accessibilityRole="button" accessibilityLabel="toggle" onPress={toggleTheme}>
      <Text>{theme.name}</Text>
    </Pressable>
  );
}

describe("ThemeProvider persistence", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
  });

  it("toggles the theme and persists the choice", async () => {
    await render(
      <ThemeProvider initial="light">
        <Probe />
      </ThemeProvider>,
    );
    expect(screen.getByText("light")).toBeTruthy();

    fireEvent.press(screen.getByRole("button", { name: "toggle" }));

    expect(await screen.findByText("dark")).toBeTruthy();
    await waitFor(async () => {
      expect(await AsyncStorage.getItem("gl_theme")).toBe("dark");
    });
  });

  it("hydrates the persisted theme on a fresh mount", async () => {
    await AsyncStorage.setItem("gl_theme", "dark");
    await render(
      <ThemeProvider initial="light">
        <Probe />
      </ThemeProvider>,
    );
    // Starts at the initial value then hydrates to the stored "dark".
    await waitFor(() => {
      expect(screen.getByText("dark")).toBeTruthy();
    });
  });
});
