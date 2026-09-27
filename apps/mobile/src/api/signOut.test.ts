import { signOut } from "./endSession";

const mockLogout = jest.fn<Promise<void>, []>();
const mockClearPdfCache = jest.fn<void, []>();

jest.mock("./auth", () => ({ logout: () => mockLogout() }));
jest.mock("./providerFlag", () => ({ providerLoginEnabled: () => false }));
jest.mock("../files/reportCardPdf", () => ({ clearPdfCache: () => mockClearPdfCache() }));

describe("signOut", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockLogout.mockResolvedValue(undefined);
  });

  it("deletes the downloaded boletas along with the session", async () => {
    await signOut();

    expect(mockLogout).toHaveBeenCalled();
    expect(mockClearPdfCache).toHaveBeenCalled();
  });

  it("still deletes the boletas when clearing the tokens fails", async () => {
    mockLogout.mockRejectedValue(new Error("storage"));

    await expect(signOut()).resolves.toBeUndefined();
    expect(mockClearPdfCache).toHaveBeenCalled();
  });

  it("never strands the family in a session when the cache cannot be deleted", async () => {
    mockClearPdfCache.mockImplementation(() => {
      throw new Error("Unable to delete file or directory: busy");
    });

    await expect(signOut()).resolves.toBeUndefined();
    expect(mockLogout).toHaveBeenCalled();
  });
});
