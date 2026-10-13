import { screen } from "@testing-library/react";
import type userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";

import { AnnouncementsCompose } from "./AnnouncementsCompose";
import { AnnouncementsScreen, type AnnouncementsScreenProps } from "./AnnouncementsScreen";

/*
 * The views of /avisos the characterization suites run against
 * (PLAN-redesign-avisos §5, E5). Both share the same props and every
 * assertion; the only per-view step is how a pupil gets picked.
 */

export type User = ReturnType<typeof userEvent.setup>;

export interface ViewUnderTest {
  name: string;
  Screen: (props: AnnouncementsScreenProps) => ReactElement;
  /** Picks one pupil by the name the button announces ("Cruz Valentina"). */
  pickStudent: (user: User, name: string) => Promise<void>;
}

/** Today's screen: every pupil is already a button. */
export const OLD_VIEW: ViewUnderTest = {
  name: "old view",
  Screen: AnnouncementsScreen,
  pickStudent: async (user, name) => {
    await user.click(await screen.findByRole("button", { name }));
  },
};

/** "Destinatarios al lado" (module_announcements_view): a pupil is found by typing first. */
export const NEW_VIEW: ViewUnderTest = {
  name: "new view",
  Screen: AnnouncementsCompose,
  pickStudent: async (user, name) => {
    const search = await screen.findByRole("searchbox", { name: "Buscar alumno" });
    await user.clear(search);
    await user.type(search, name.split(" ")[0] ?? name);
    await user.click(await screen.findByRole("button", { name }));
  },
};

export const VIEWS: readonly ViewUnderTest[] = [OLD_VIEW, NEW_VIEW];
