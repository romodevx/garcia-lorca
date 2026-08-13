import { emitSessionLost, onSessionLost } from "./session";

describe("session-lost signal", () => {
  it("notifies every current subscriber when emitted", () => {
    const a = jest.fn();
    const b = jest.fn();
    const unsubA = onSessionLost(a);
    const unsubB = onSessionLost(b);

    emitSessionLost();

    expect(a).toHaveBeenCalledTimes(1);
    expect(b).toHaveBeenCalledTimes(1);

    unsubA();
    unsubB();
  });

  it("stops notifying a listener after it unsubscribes", () => {
    const listener = jest.fn();
    const unsubscribe = onSessionLost(listener);

    unsubscribe();
    emitSessionLost();

    expect(listener).not.toHaveBeenCalled();
  });
});
