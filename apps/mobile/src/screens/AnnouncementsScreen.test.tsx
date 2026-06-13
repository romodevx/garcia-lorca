import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { AnnouncementsScreen } from "./AnnouncementsScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("AnnouncementsScreen", () => {
  it("clears the unread state when Marcar leídos is pressed", async () => {
    await renderWithProviders(<AnnouncementsScreen />);

    // Unread items exist, so the bulk action is shown.
    const markAll = screen.getByRole("button", { name: "Marcar leídos" });
    fireEvent.press(markAll);

    // Once everything is read, the action disappears (unread count is zero).
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Marcar leídos" })).toBeNull();
    });
  });

  it("renders a friendly empty state when there are no announcements", async () => {
    await renderWithProviders(<AnnouncementsScreen emptyOverride />);
    expect(screen.getByText("No hay avisos nuevos")).toBeTruthy();
    expect(screen.getByText("Te avisaremos cuando la escuela publique algo.")).toBeTruthy();
  });
});
