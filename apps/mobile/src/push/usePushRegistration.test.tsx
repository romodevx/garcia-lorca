import { act, renderHook, waitFor } from "@testing-library/react-native";
import { requestFreshData } from "../state/freshness";
import { registerForPush, type PushRegistration } from "./register";
import { unregisterPush } from "./unregister";
import { usePushRegistration } from "./usePushRegistration";

jest.mock("./register", () => ({ registerForPush: jest.fn() }));
jest.mock("./unregister", () => ({ unregisterPush: jest.fn(async () => undefined) }));
jest.mock("./notificationsModule", () => ({ loadNotifications: () => null }));

const mockRegister = registerForPush as jest.MockedFunction<typeof registerForPush>;

/** A registration the test resolves by hand, to sign out while it is in flight. */
function deferredRegistration(): (outcome: PushRegistration) => Promise<void> {
  let resolve: (outcome: PushRegistration) => void = () => undefined;
  mockRegister.mockImplementationOnce(
    () =>
      new Promise<PushRegistration>((r) => {
        resolve = r;
      }),
  );
  return async (outcome) => {
    await act(async () => {
      resolve(outcome);
    });
  };
}

function renderRegistration(signedIn: boolean) {
  return renderHook(({ on }: { on: boolean }) => usePushRegistration(on), { initialProps: { on: signedIn } });
}

describe("usePushRegistration — sign-out during an in-flight registration", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockRegister.mockResolvedValue("registered");
  });

  it("unregisters the token that was just saved and does not settle", async () => {
    const finish = deferredRegistration();
    const hook = await renderRegistration(true);
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(1));

    await hook.rerender({ on: false });
    await finish("registered");

    await waitFor(() => expect(unregisterPush).toHaveBeenCalledTimes(1));

    // Not settled: the next sign-in registers again.
    await hook.rerender({ on: true });
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(2));
    expect(unregisterPush).toHaveBeenCalledTimes(1);
  });

  it("registers guardian B when B signs in while A's registration is still in flight", async () => {
    const finish = deferredRegistration();
    const hook = await renderRegistration(true);
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(1));

    await hook.rerender({ on: false });
    await hook.rerender({ on: true });
    await finish("registered");

    await waitFor(() => expect(unregisterPush).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(2));
  });

  it("does not unregister when the stale attempt registered nothing", async () => {
    const finish = deferredRegistration();
    const hook = await renderRegistration(true);
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(1));

    await hook.rerender({ on: false });
    await finish("denied");

    expect(unregisterPush).not.toHaveBeenCalled();
  });

  it("still settles a registration that finishes inside its own session", async () => {
    await renderRegistration(true);
    await waitFor(() => expect(mockRegister).toHaveBeenCalledTimes(1));

    await act(async () => {
      requestFreshData();
    });

    expect(mockRegister).toHaveBeenCalledTimes(1);
    expect(unregisterPush).not.toHaveBeenCalled();
  });
});
