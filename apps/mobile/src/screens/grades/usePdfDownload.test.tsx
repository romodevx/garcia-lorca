import { act, renderHook } from "@testing-library/react-native";
import { downloadPdf, sharePdf } from "../../files/reportCardPdf";
import { usePdfDownload } from "./usePdfDownload";

jest.mock("../../files/reportCardPdf", () => {
  const actual = jest.requireActual("../../files/reportCardPdf");
  return { ...actual, downloadPdf: jest.fn(), sharePdf: jest.fn(async () => undefined) };
});

const target = { path: "/p", studentId: "s1", fallbackName: "boleta-t1-v1.pdf", title: "Boleta T1" };

describe("usePdfDownload", () => {
  beforeEach(() => jest.clearAllMocks());

  it("does not open the share sheet when the screen closed mid-download", async () => {
    let finish: (uri: string) => void = () => undefined;
    (downloadPdf as jest.Mock).mockImplementation(() => new Promise<string>((resolve) => (finish = resolve)));
    const hook = await renderHook(() => usePdfDownload(target));

    await act(async () => hook.result.current.start());
    await hook.unmount();
    await act(async () => finish("file:///cache/boletas/s1/boleta-t1-v1.pdf"));

    expect(sharePdf).not.toHaveBeenCalled();
  });

  it("opens the share sheet when the screen is still open", async () => {
    (downloadPdf as jest.Mock).mockResolvedValue("file:///b.pdf");
    const hook = await renderHook(() => usePdfDownload(target));

    await act(async () => hook.result.current.start());

    expect(sharePdf).toHaveBeenCalledWith("file:///b.pdf", "Boleta T1");
    expect(hook.result.current.state).toEqual({ status: "idle" });
  });
});
