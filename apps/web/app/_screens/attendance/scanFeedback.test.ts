import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createScanFeedback } from "./scanFeedback";

interface FakeOscillator {
  type: string;
  frequency: { value: number };
  connect: ReturnType<typeof vi.fn>;
  start: ReturnType<typeof vi.fn>;
  stop: ReturnType<typeof vi.fn>;
}

const oscillators: FakeOscillator[] = [];
const close = vi.fn(() => Promise.resolve());

class FakeAudioContext {
  currentTime = 0;
  destination = {};
  state = "running";
  createOscillator(): FakeOscillator {
    const osc = { type: "", frequency: { value: 0 }, connect: vi.fn(), start: vi.fn(), stop: vi.fn() };
    osc.connect.mockReturnValue({ connect: vi.fn() });
    oscillators.push(osc);
    return osc;
  }
  createGain() {
    return { gain: { value: 0, setValueAtTime: vi.fn(), exponentialRampToValueAtTime: vi.fn() }, connect: vi.fn() };
  }
  resume = vi.fn(() => Promise.resolve());
  close = close;
}

beforeEach(() => {
  oscillators.length = 0;
  vi.stubGlobal("AudioContext", FakeAudioContext);
  Object.defineProperty(navigator, "vibrate", { value: vi.fn(() => true), configurable: true });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("createScanFeedback", () => {
  it("ok: one short high beep and a short vibration", () => {
    const fb = createScanFeedback();
    fb.ok();
    expect(oscillators).toHaveLength(1);
    expect(oscillators[0]?.frequency.value).toBeGreaterThanOrEqual(1000);
    expect(navigator.vibrate).toHaveBeenCalledWith(60);
  });

  it("duplicate: one soft, lower beep, no vibration", () => {
    const fb = createScanFeedback();
    fb.duplicate();
    expect(oscillators).toHaveLength(1);
    expect(oscillators[0]?.frequency.value).toBeLessThan(1000);
    expect(navigator.vibrate).not.toHaveBeenCalled();
  });

  it("error: a low double buzz and a vibration pattern", () => {
    const fb = createScanFeedback();
    fb.error();
    expect(oscillators).toHaveLength(2);
    expect(oscillators.every((o) => o.frequency.value < 400)).toBe(true);
    expect(navigator.vibrate).toHaveBeenCalledWith([80, 60, 80]);
  });

  it("dispose closes the audio context", () => {
    createScanFeedback().dispose();
    expect(close).toHaveBeenCalled();
  });

  it("stays silent (never throws) without Web Audio or vibration", () => {
    vi.stubGlobal("AudioContext", undefined);
    Object.defineProperty(navigator, "vibrate", { value: undefined, configurable: true });
    const fb = createScanFeedback();
    expect(() => {
      fb.ok();
      fb.duplicate();
      fb.error();
      fb.dispose();
    }).not.toThrow();
  });
});
