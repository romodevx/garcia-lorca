// icons.jsx — rounded line icon set (stroke = currentColor)
// Usage: <Icon name="students" size={28} />
const ICON_PATHS = {
  // ── modules ──
  students: <g><circle cx="9" cy="8" r="3.2"/><path d="M3.5 20c.4-3.2 2.8-5 5.5-5s5.1 1.8 5.5 5"/><path d="M16 6.2a2.8 2.8 0 0 1 0 5.4"/><path d="M17.5 14.4c2 .7 3.4 2.3 3.7 4.6"/></g>,
  finance: <g><path d="M3.5 7.5h17a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1h-17a1 1 0 0 1-1-1v-9a1 1 0 0 1 1-1Z"/><path d="M3 11h18"/><circle cx="17" cy="14.5" r="1.4"/></g>,
  scholarship: <g><circle cx="12" cy="9" r="5"/><path d="M9 13.5 7.5 21l4.5-2.5L16.5 21 15 13.5"/><path d="m12 6.6.9 1.8 2 .3-1.45 1.4.34 2L12 12.1l-1.8.95.34-2L9.1 9l2-.3z"/></g>,
  documents: <g><path d="M6 2.5h8l4 4V20a1.5 1.5 0 0 1-1.5 1.5h-10A1.5 1.5 0 0 1 5 20V4a1.5 1.5 0 0 1 1-1.5Z"/><path d="M14 2.5V6a1 1 0 0 0 1 1h3"/><path d="M8.5 12.5h7M8.5 16h5"/></g>,
  qr: <g><rect x="3" y="3" width="7" height="7" rx="1.4"/><rect x="14" y="3" width="7" height="7" rx="1.4"/><rect x="3" y="14" width="7" height="7" rx="1.4"/><path d="M14 14h3v3M21 14v.01M14 21h3M20 17.5v3.5M17 21h.01"/></g>,
  grades: <g><path d="M4 19.5V5a1.5 1.5 0 0 1 1.5-1.5H18A1.5 1.5 0 0 1 19.5 5v14.5"/><path d="M4 19.5h16"/><path d="M8 16v-3M12 16v-6M16 16v-4"/></g>,
  uniforms: <g><path d="M8.5 3 5 6 3 9l2.5 2 1-1v8.5A1.5 1.5 0 0 0 8 20h8a1.5 1.5 0 0 0 1.5-1.5V10l1 1L21 9l-2-3-3.5-3"/><path d="M8.5 3c0 2 1.6 3 3.5 3s3.5-1 3.5-3"/></g>,
  announcements: <g><path d="M4 9.5v3a1.5 1.5 0 0 0 1.5 1.5H8l5.5 4V4L8 9.5H5.5A1.5 1.5 0 0 0 4 11"/><path d="M17.5 8.5a4 4 0 0 1 0 6"/><path d="M8 14.5v3a1.5 1.5 0 0 0 1.5 1.5h.5a1 1 0 0 0 1-1v-2.5"/></g>,
  billing: <g><path d="M6 2.5h12a1 1 0 0 1 1 1V21l-2.2-1.4L14.5 21l-2.5-1.5L9.5 21l-2.3-1.4L5 21V3.5a1 1 0 0 1 1-1Z"/><path d="M8.5 8h7M8.5 11.5h7M8.5 15h4"/></g>,
  reports: <g><rect x="3" y="3" width="18" height="18" rx="2.5"/><path d="M8 16v-4M12 16V8M16 16v-6"/></g>,
  settings: <g><circle cx="12" cy="12" r="3"/><path d="M12 2.5v2.4M12 19.1v2.4M21.5 12h-2.4M4.9 12H2.5M18.7 5.3l-1.7 1.7M7 17l-1.7 1.7M18.7 18.7 17 17M7 7 5.3 5.3"/></g>,
  // ── UI ──
  search: <g><circle cx="11" cy="11" r="7"/><path d="m20 20-3.2-3.2"/></g>,
  bell: <g><path d="M6 9a6 6 0 0 1 12 0c0 5 1.5 6.5 2 7H4c.5-.5 2-2 2-7Z"/><path d="M10 20a2 2 0 0 0 4 0"/></g>,
  sun: <g><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M22 12h-2M4 12H2M19 5l-1.5 1.5M6.5 17.5 5 19M19 19l-1.5-1.5M6.5 6.5 5 5"/></g>,
  moon: <path d="M20 14.5A8 8 0 1 1 9.5 4 6.5 6.5 0 0 0 20 14.5Z"/>,
  back: <path d="M15 5l-7 7 7 7"/>,
  chevron: <path d="M9 5l7 7-7 7"/>,
  chevronDown: <path d="M5 9l7 7 7-7"/>,
  check: <path d="M5 12.5l4.5 4.5L19 7"/>,
  clock: <g><circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/></g>,
  calendar: <g><rect x="3.5" y="5" width="17" height="16" rx="2"/><path d="M3.5 9.5h17M8 3v4M16 3v4"/></g>,
  logout: <g><path d="M14 4.5H6A1.5 1.5 0 0 0 4.5 6v12A1.5 1.5 0 0 0 6 19.5h8"/><path d="M17 8l4 4-4 4M9 12h12"/></g>,
  plus: <path d="M12 5v14M5 12h14"/>,
  filter: <path d="M3.5 5.5h17l-6.5 8v5l-4 2v-7Z"/>,
  more: <g><circle cx="5" cy="12" r="1.6"/><circle cx="12" cy="12" r="1.6"/><circle cx="19" cy="12" r="1.6"/></g>,
  download: <g><path d="M12 3v12M7.5 10.5 12 15l4.5-4.5"/><path d="M4.5 19.5h15"/></g>,
  mail: <g><rect x="3" y="5" width="18" height="14" rx="2"/><path d="m4 7 8 6 8-6"/></g>,
  phone: <path d="M6.5 3.5h3l1.5 4-2 1.5a11 11 0 0 0 5.5 5.5l1.5-2 4 1.5v3a1.5 1.5 0 0 1-1.6 1.5C12 23 3 14 3.5 5.1A1.5 1.5 0 0 1 5 3.5Z"/>,
  pin: <g><path d="M12 21s7-6.3 7-11a7 7 0 0 0-14 0c0 4.7 7 11 7 11Z"/><circle cx="12" cy="10" r="2.6"/></g>,
  card: <g><rect x="2.5" y="5" width="19" height="14" rx="2.5"/><path d="M2.5 9.5h19M6 15h4"/></g>,
  user: <g><circle cx="12" cy="8" r="4"/><path d="M4.5 20c.7-4 3.8-6 7.5-6s6.8 2 7.5 6"/></g>,
  warning: <g><path d="M12 3.5 21.5 20H2.5L12 3.5Z"/><path d="M12 10v4.5M12 17.5v.01"/></g>,
  trend: <g><path d="M3 16.5 9 10l4 3.5 8-8.5"/><path d="M16 5h5v5"/></g>,
  graduation: <g><path d="m12 4 9 4-9 4-9-4 9-4Z"/><path d="M6 10v4.5c0 1.4 2.7 2.5 6 2.5s6-1.1 6-2.5V10"/><path d="M21 8v5"/></g>,
  heart: <path d="M12 20S4 14.5 4 9.2A4 4 0 0 1 12 7a4 4 0 0 1 8 2.2C20 14.5 12 20 12 20Z"/>,
  message: <path d="M4 5.5h16a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H9l-4 3.5V6.5a1 1 0 0 1 1-1Z"/>,
  star: <path d="m12 4 2.4 5 5.5.6-4.1 3.7 1.2 5.4L12 16l-4.9 2.7 1.2-5.4L4.2 9.6 9.7 9 12 4Z"/>,
  shield: <g><path d="M12 3 5 5.5v5c0 4.4 3 7.7 7 9.5 4-1.8 7-5.1 7-9.5v-5L12 3Z"/><path d="m9 11.5 2 2 4-4"/></g>,
  close: <path d="M6 6l12 12M18 6 6 18"/>,
  qrScan: <g><path d="M4 8V5.5A1.5 1.5 0 0 1 5.5 4H8M16 4h2.5A1.5 1.5 0 0 1 20 5.5V8M20 16v2.5a1.5 1.5 0 0 1-1.5 1.5H16M8 20H5.5A1.5 1.5 0 0 1 4 18.5V16"/><path d="M4 12h16"/></g>,
  eye: <g><path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="3"/></g>,
};

function Icon({ name, size = 24, stroke = 2, className, style }) {
  const p = ICON_PATHS[name];
  if (!p) return null;
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none"
      stroke="currentColor" strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round"
      className={className} style={style} aria-hidden="true">{p}</svg>
  );
}

window.Icon = Icon;
