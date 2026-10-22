import { renderHook, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { useStudentPhoto } from "./useStudentPhoto";

const JPEG = new Uint8Array([0xff, 0xd8, 0xff, 0xe0]);

function mockPhotos(status: number): jest.Mock {
  const fetchMock = jest.fn(async (_url: string, _init?: RequestInit) => ({
    ok: status === 200,
    status,
    headers: { get: (name: string) => (name.toLowerCase() === "content-type" ? "image/jpeg" : null) },
    arrayBuffer: async () => JPEG.buffer,
    json: async () => ({}),
  }));
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

const photoCalls = (mock: jest.Mock, id: string): unknown[][] =>
  mock.mock.calls.filter((c) => String((c as unknown[])[0]).endsWith(`/family/students/${id}/photo`));

// Android release never drew the photo: RN Image's own headers were not sent.
// The photo now travels the app's authenticated client and lands as a data URI.
describe("useStudentPhoto", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("downloads the photo with the session and hands back a data URI", async () => {
    const fetchMock = mockPhotos(200);
    const { result } = await renderHook(() => useStudentPhoto("s1"));

    await waitFor(() => expect(result.current.data).toBe("data:image/jpeg;base64,/9j/4A=="));
    const init = photoCalls(fetchMock, "s1")[0]?.[1] as { headers: Record<string, string> };
    expect(init.headers.Authorization).toBe("Bearer a1");
  });

  it("has no photo when the school has none (404)", async () => {
    mockPhotos(404);
    const { result } = await renderHook(() => useStudentPhoto("s1"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBe(false);
  });

  it("fetches the other child's photo when the child changes", async () => {
    const fetchMock = mockPhotos(200);
    const { result, rerender } = await renderHook(({ id }: { id: string }) => useStudentPhoto(id), { initialProps: { id: "s1" } });
    await waitFor(() => expect(result.current.data).not.toBeNull());

    await rerender({ id: "s2" });

    await waitFor(() => expect(photoCalls(fetchMock, "s2")).toHaveLength(1));
  });
});
