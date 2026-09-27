import { act, renderHook, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { requestFreshData } from "./freshness";
import { useChildAttendanceMonth } from "./familyData";

const MONTH_BODY = { month: "2026-09", today: "2026-09-28", days: [], summary: {} };

function mockAttendance(): jest.Mock {
  const fetchMock = jest.fn(async () => ({ ok: true, status: 200, json: async () => MONTH_BODY }));
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

const attendanceCalls = (mock: jest.Mock): number =>
  mock.mock.calls.filter((c) => String((c as unknown[])[0]).includes("attendance?month=2026-09")).length;

describe("useChildAttendanceMonth", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("shares one in-flight request between two screens reading the same child and month", async () => {
    const fetchMock = mockAttendance();
    const { result } = await renderHook(() => ({
      home: useChildAttendanceMonth("s1", "2026-09"),
      asistencia: useChildAttendanceMonth("s1", "2026-09"),
    }));
    await waitFor(() => expect(result.current.asistencia.data).not.toBeNull());
    expect(result.current.home.data).not.toBeNull();
    expect(attendanceCalls(fetchMock)).toBe(1);

    await act(async () => requestFreshData());
    await waitFor(() => expect(attendanceCalls(fetchMock)).toBe(2));
  });

  it("an idle resource (no month) fires nothing on the freshness signal", async () => {
    const fetchMock = mockAttendance();
    await renderHook(() => useChildAttendanceMonth("s1", null));
    await act(async () => requestFreshData());
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
