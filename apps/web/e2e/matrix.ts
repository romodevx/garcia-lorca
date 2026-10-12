/**
 * The responsive/zoom matrix of PLAN-redesign-home §4.3, shared by the home's
 * spec and the other modules' spec so both walk the same rows.
 *
 * Browser zoom shrinks the CSS width: 1280px at 400% is 320px wide at a
 * deviceScaleFactor of 4. A zoom row is that CSS viewport, not a real zoom.
 */
export interface Row {
  width: number;
  height: number;
  dsf: number;
  /** Tablets are touched: their rows run with `hasTouch`. */
  touch: boolean;
  /** Set on the zoom rows (1.5 = 150%); D9 makes ≥ 3 report-only outside the home. */
  zoom?: number;
}

const tablet = (width: number, height: number): Row => ({ width, height, dsf: 2, touch: true });
const desk = (width: number, height: number): Row => ({ width, height, dsf: 1, touch: false });
const zoomed = (width: number, height: number, zoom: number): Row => ({ width, height, dsf: zoom, touch: false, zoom });

export const HOME_ROWS: readonly Row[] = [
  tablet(768, 1024),
  tablet(820, 1180),
  tablet(1024, 768),
  tablet(1180, 820),
  tablet(1366, 1024),
  desk(1280, 800),
  desk(1440, 900),
  desk(1920, 1080),
  desk(2560, 1440),
  zoomed(853, 533, 1.5),
  zoomed(640, 400, 2),
  zoomed(427, 267, 3),
  zoomed(320, 200, 4),
  // 50% and 67% over a 1920 window.
  { ...desk(3840, 2160), zoom: 0.5 },
  { ...desk(2880, 1620), zoom: 0.67 },
];

/** D9: outside the home, a row at 300% zoom or more is report-only. */
export const REPORT_ONLY_ZOOM = 3;

export function isReportOnly(row: Row): boolean {
  return row.zoom !== undefined && row.zoom >= REPORT_ONLY_ZOOM;
}

export function rowName(row: Row): string {
  const zoom = row.zoom === undefined ? "" : ` · zoom ${Math.round(row.zoom * 100)}%`;
  return `${row.width}×${row.height} @${row.dsf}x${row.touch ? " táctil" : ""}${zoom}`;
}
