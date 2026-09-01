import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as commsApi from "../../comms/api";
import * as configApi from "../../config/api";
import { AnnouncementsScreen } from "./AnnouncementsScreen";

vi.mock("../../comms/api", () => ({ announce: vi.fn() }));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listLevels: vi.fn(), listGroups: vi.fn() }));

const comms = vi.mocked(commsApi);
const config = vi.mocked(configApi);

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listLevels.mockResolvedValue([{ id: "lv1", name: "Primaria", sepCode: "PRIM", enrollmentGranularity: "cycle" }]);
  config.listGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "1° A", capacity: 30 }]);
  comms.announce.mockResolvedValue({ created: 28, audience: 28 });
});
afterEach(() => vi.clearAllMocks());

describe("AnnouncementsScreen", () => {
  it("sends one message to a chosen group", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsScreen onBack={() => {}} />);

    await screen.findByRole("option", { name: "1° A" });
    await user.selectOptions(screen.getByLabelText("Grupo"), "g1");
    await user.type(screen.getByLabelText("Título"), "Junta de padres");
    await user.type(screen.getByLabelText("Mensaje"), "Jueves 8:00");
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));

    await waitFor(() => expect(comms.announce).toHaveBeenCalled());
    const sent = comms.announce.mock.calls[0]?.[1];
    expect(sent?.scope).toBe("group");
    expect(sent?.targetId).toBe("g1");
    expect(sent?.idempotencyKey).toBeTruthy();
    expect(await screen.findByRole("status")).toHaveTextContent("28 alumnos");
  });

  // Whole-school needs no target, and asking for one would be a field with
  // nothing in it that still blocks the send.
  it("does not ask for a target when the audience is the whole school", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsScreen onBack={() => {}} />);
    await screen.findByLabelText("Grupo");

    await user.click(screen.getByRole("button", { name: "Toda la escuela" }));

    expect(screen.queryByLabelText("Grupo")).not.toBeInTheDocument();
  });

  it("refuses to send a message with nothing in it", async () => {
    const user = userEvent.setup();
    render(<AnnouncementsScreen onBack={() => {}} />);
    await screen.findByLabelText("Grupo");

    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));

    expect(comms.announce).not.toHaveBeenCalled();
    expect(await screen.findByRole("alert")).toBeInTheDocument();
  });

  // The office has to read the refusal, not a generic failure: "nobody matched"
  // is the difference between fixing the audience and believing it went out.
  it("shows the gateway's own words when nobody matched", async () => {
    const user = userEvent.setup();
    comms.announce.mockRejectedValue(new Error("nadie coincide con ese destino"));
    render(<AnnouncementsScreen onBack={() => {}} />);

    await screen.findByRole("option", { name: "1° A" });
    await user.selectOptions(screen.getByLabelText("Grupo"), "g1");
    await user.type(screen.getByLabelText("Título"), "Junta");
    await user.type(screen.getByLabelText("Mensaje"), "Jueves");
    await user.click(screen.getByRole("button", { name: /Enviar aviso/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent("nadie coincide con ese destino");
  });
});
