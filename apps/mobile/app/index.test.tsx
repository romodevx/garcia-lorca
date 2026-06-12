import { render, screen } from "@testing-library/react-native";
import IndexScreen from "./index";

describe("IndexScreen", () => {
  // RNTL v14 render/act are async — tests must await render.
  it("renders the app name", async () => {
    await render(<IndexScreen />);
    expect(screen.getByText("García Lorca")).toBeTruthy();
  });
});
