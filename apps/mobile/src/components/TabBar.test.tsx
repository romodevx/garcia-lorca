import { fireEvent, screen } from "@testing-library/react-native";
import type { BottomTabBarProps } from "expo-router/tabs";
import { renderWithProviders } from "../test/renderWithProviders";
import { TabBar } from "./TabBar";

const tabProps = {
  state: { index: 0, routes: [{ key: "inicio", name: "inicio" }] },
  navigation: { navigate: jest.fn() },
} as unknown as BottomTabBarProps;

describe("TabBar", () => {
  it("reports its laid-out height", async () => {
    const onHeight = jest.fn();
    await renderWithProviders(<TabBar {...tabProps} onHeight={onHeight} />);

    fireEvent(screen.getByTestId("tab-bar"), "layout", { nativeEvent: { layout: { x: 0, y: 0, width: 390, height: 83 } } });

    expect(onHeight).toHaveBeenCalledWith(83);
  });
});
