/**
 * The sound and touch of a scan, so the teacher knows the result without
 * looking at the screen: a short high beep (registered), a soft lower beep
 * (already registered), a low double buzz (rejected).
 *
 * Tones are synthesised with the Web Audio API — no audio asset to download.
 * The AudioContext must be created INSIDE the user gesture that opens the
 * scanner: iOS Safari keeps a context made outside a gesture suspended forever.
 * Every call is best-effort: no Web Audio or no vibration means silence, never
 * an error.
 */

export interface ScanFeedback {
  ok: () => void;
  duplicate: () => void;
  error: () => void;
  dispose: () => void;
}

interface Tone {
  frequency: number;
  /** Seconds after "now". */
  at: number;
  duration: number;
  volume: number;
  type: OscillatorType;
}

const OK_TONE: Tone = { frequency: 1320, at: 0, duration: 0.12, volume: 0.25, type: "sine" };
const DUPLICATE_TONE: Tone = { frequency: 660, at: 0, duration: 0.14, volume: 0.12, type: "sine" };
const BUZZ = { frequency: 180, duration: 0.14, gap: 0.08, volume: 0.3, type: "square" as OscillatorType };
const ERROR_TONES: readonly Tone[] = [
  { frequency: BUZZ.frequency, at: 0, duration: BUZZ.duration, volume: BUZZ.volume, type: BUZZ.type },
  { frequency: BUZZ.frequency, at: BUZZ.duration + BUZZ.gap, duration: BUZZ.duration, volume: BUZZ.volume, type: BUZZ.type },
];
const OK_VIBRATION_MS = 60;
const ERROR_VIBRATION: readonly number[] = [80, 60, 80];
const SILENT_GAIN = 0.0001;

type AudioContextCtor = new () => AudioContext;

function newAudioContext(): AudioContext | undefined {
  const scope = globalThis as unknown as { AudioContext?: AudioContextCtor; webkitAudioContext?: AudioContextCtor };
  const Ctor = scope.AudioContext ?? scope.webkitAudioContext;
  if (Ctor === undefined) return undefined;
  try {
    return new Ctor();
  } catch {
    return undefined;
  }
}

function play(ctx: AudioContext | undefined, tone: Tone): void {
  if (ctx === undefined) return;
  try {
    if (ctx.state === "suspended") void ctx.resume();
    const start = ctx.currentTime + tone.at;
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = tone.type;
    osc.frequency.value = tone.frequency;
    gain.gain.setValueAtTime(tone.volume, start);
    gain.gain.exponentialRampToValueAtTime(SILENT_GAIN, start + tone.duration);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start(start);
    osc.stop(start + tone.duration);
  } catch {
    // A sound is a courtesy: the card on screen already said it.
  }
}

function vibrate(pattern: number | readonly number[]): void {
  try {
    navigator.vibrate?.(pattern as number | number[]);
  } catch {
    // Not every browser lets a page vibrate.
  }
}

/** Call from the click that opens the scanner (iOS unlocks audio only inside a gesture). */
export function createScanFeedback(): ScanFeedback {
  const ctx = newAudioContext();
  return {
    ok: () => {
      play(ctx, OK_TONE);
      vibrate(OK_VIBRATION_MS);
    },
    duplicate: () => play(ctx, DUPLICATE_TONE),
    error: () => {
      for (const tone of ERROR_TONES) play(ctx, tone);
      vibrate(ERROR_VIBRATION);
    },
    dispose: () => {
      void ctx?.close().catch(() => undefined);
    },
  };
}
