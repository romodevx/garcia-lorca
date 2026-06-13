/* Rounded line icons — single consistent family (stroke 2, round caps).
   <Icon name="home" size={22} /> ; inherits currentColor. */
(function () {
  const P = {
    home: <><path d="M4 11.5 12 4.5l8 7"/><path d="M6 10.5V19a1 1 0 0 0 1 1h3.5v-4.5a1.5 1.5 0 0 1 3 0V20H17a1 1 0 0 0 1-1v-8.5"/></>,
    calendar: <><rect x="3.5" y="5" width="17" height="15.5" rx="3.5"/><path d="M3.5 9.5h17M8 3.5v3M16 3.5v3"/></>,
    'calendar-check': <><rect x="3.5" y="5" width="17" height="15.5" rx="3.5"/><path d="M3.5 9.5h17M8 3.5v3M16 3.5v3"/><path d="m8.8 14.5 2 2 3.6-3.8"/></>,
    award: <><circle cx="12" cy="9.5" r="5.5"/><path d="m8.5 14 -1.3 6 4.8-2.6 4.8 2.6-1.3-6"/></>,
    chart: <><path d="M4.5 4.5v13a2 2 0 0 0 2 2h13"/><path d="M8.5 15.5v-3M12.5 15.5v-6M16.5 15.5v-9"/></>,
    wallet: <><path d="M3.5 8a3 3 0 0 1 3-3h10.5a1.5 1.5 0 0 1 1.5 1.5V8"/><rect x="3.5" y="7" width="17" height="12.5" rx="3.5"/><path d="M20.5 12h-3.2a2 2 0 0 0 0 4h3.2"/></>,
    megaphone: <><path d="M4 10v4a1.5 1.5 0 0 0 1.5 1.5H7l1 4h2l-.7-4"/><path d="M7 15.5V8.5l9.5-4v15z"/><path d="M16.5 8.5a3 3 0 0 1 0 7"/></>,
    grid: <><rect x="4" y="4" width="6.5" height="6.5" rx="2"/><rect x="13.5" y="4" width="6.5" height="6.5" rx="2"/><rect x="4" y="13.5" width="6.5" height="6.5" rx="2"/><rect x="13.5" y="13.5" width="6.5" height="6.5" rx="2"/></>,
    bell: <><path d="M6 10a6 6 0 0 1 12 0c0 4 1.2 5.2 2 6H4c.8-.8 2-2 2-6Z"/><path d="M9.5 20a2.5 2.5 0 0 0 5 0"/></>,
    'chevron-right': <path d="m9.5 5.5 6.5 6.5-6.5 6.5"/>,
    'chevron-left': <path d="m14.5 5.5-6.5 6.5 6.5 6.5"/>,
    'chevron-down': <path d="m5.5 9.5 6.5 6.5 6.5-6.5"/>,
    check: <path d="m5 12.5 4.5 4.5L19 7"/>,
    'check-circle': <><circle cx="12" cy="12" r="8.5"/><path d="m8.5 12 2.4 2.4 4.6-4.8"/></>,
    x: <path d="M6.5 6.5 17.5 17.5M17.5 6.5 6.5 17.5"/>,
    'x-circle': <><circle cx="12" cy="12" r="8.5"/><path d="M9.2 9.2 14.8 14.8M14.8 9.2 9.2 14.8"/></>,
    plus: <path d="M12 5.5v13M5.5 12h13"/>,
    clock: <><circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/></>,
    document: <><path d="M6.5 3.5h7l5 5v11a1.5 1.5 0 0 1-1.5 1.5h-10.5a1.5 1.5 0 0 1-1.5-1.5V5a1.5 1.5 0 0 1 1.5-1.5Z"/><path d="M13 3.5V8a1 1 0 0 0 1 1h4.5"/><path d="M8.5 13h7M8.5 16.5h5"/></>,
    shirt: <><path d="M8.5 4 5 6 3.5 9l2.5 1.5"/><path d="M15.5 4 19 6l1.5 3-2.5 1.5"/><path d="M8.5 4c0 1.8 1.4 3 3.5 3s3.5-1.2 3.5-3"/><path d="M6 10.5V20h12v-9.5"/></>,
    receipt: <><path d="M5.5 4.5h13v15.5l-2-1.4-2 1.4-2-1.4-1 .7-1-.7-2 1.4-2-1.4Z"/><path d="M8.5 9h7M8.5 12.5h7"/></>,
    cap: <><path d="m12 4 9 4.5-9 4.5-9-4.5Z"/><path d="M6.5 11v4.5c0 1.4 2.5 2.5 5.5 2.5s5.5-1.1 5.5-2.5V11"/><path d="M21 8.5v5"/></>,
    user: <><circle cx="12" cy="8.5" r="3.8"/><path d="M5 19.5a7 7 0 0 1 14 0"/></>,
    sun: <><circle cx="12" cy="12" r="4"/><path d="M12 2.5v2.5M12 19v2.5M4.2 4.2l1.8 1.8M18 18l1.8 1.8M2.5 12H5M19 12h2.5M4.2 19.8 6 18M18 6l1.8-1.8"/></>,
    moon: <path d="M19 13.5A7.5 7.5 0 0 1 10.5 5a6 6 0 1 0 8.5 8.5Z"/>,
    refresh: <><path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3"/><path d="M19.5 4.5V9H15"/></>,
    alert: <><path d="M12 4.5 21 19.5H3Z"/><path d="M12 10v4M12 16.8v.2"/></>,
    'arrow-right': <path d="M5 12h13M13 6.5 18.5 12 13 17.5"/>,
    'arrow-up-right': <path d="M7 17 17 7M9 7h8v8"/>,
    lock: <><rect x="5" y="10.5" width="14" height="10" rx="3"/><path d="M8 10.5V8a4 4 0 0 1 8 0v2.5"/><circle cx="12" cy="15.5" r="1.3"/></>,
    eye: <><path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="3"/></>,
    'eye-off': <><path d="M4 4.5 20 19.5"/><path d="M9.5 9.6A3 3 0 0 0 12 15c.5 0 1-.1 1.4-.4"/><path d="M6.6 6.8C4.2 8.3 2.5 12 2.5 12s3.5 6.5 9.5 6.5c1.6 0 3-.4 4.2-1"/><path d="M9.8 5.8A8 8 0 0 1 12 5.5c6 0 9.5 6.5 9.5 6.5a17 17 0 0 1-2.4 3.2"/></>,
    inbox: <><path d="M3.5 13.5 6 6a2 2 0 0 1 1.9-1.5h8.2A2 2 0 0 1 18 6l2.5 7.5"/><path d="M3.5 13.5V18a1.5 1.5 0 0 0 1.5 1.5h14a1.5 1.5 0 0 0 1.5-1.5v-4.5h-5a3 3 0 0 1-6 0Z"/></>,
    sparkles: <><path d="M12 4.5 13.4 9 18 10.4 13.4 11.8 12 16.3 10.6 11.8 6 10.4 10.6 9Z"/><path d="M18 4v3M19.5 5.5h-3"/></>,
    heart: <path d="M12 19.5S4.5 15 4.5 9.7A3.7 3.7 0 0 1 12 7.6a3.7 3.7 0 0 1 7.5 2.1C19.5 15 12 19.5 12 19.5Z"/>,
    pin: <><path d="M12 21s6-5.3 6-10a6 6 0 0 0-12 0c0 4.7 6 10 6 10Z"/><circle cx="12" cy="11" r="2.3"/></>,
    book: <><path d="M5 4.5h9a2 2 0 0 1 2 2v13a1.5 1.5 0 0 0-1.5-1.5H5Z"/><path d="M5 4.5v13.5"/><path d="M16 6.5h2.5a1 1 0 0 1 1 1V19a1.5 1.5 0 0 0-1.5-1.5H16"/></>,
    phone: <path d="M6 3.5h3l1.5 4-2 1.5a11 11 0 0 0 5 5l1.5-2 4 1.5v3a2 2 0 0 1-2 2A15.5 15.5 0 0 1 4 5.5a2 2 0 0 1 2-2Z"/>,
    map: <><path d="m9 4.5-5 2v13l5-2 6 2 5-2v-13l-5 2-6-2Z"/><path d="M9 4.5v15M15 6.5v15"/></>,
    star: <path d="m12 4 2.3 4.9 5.2.6-3.9 3.6 1.1 5.3L12 15.8 7.3 18.4l1.1-5.3L4.5 9.5l5.2-.6Z"/>,
    minus: <path d="M5.5 12h13"/>,
    download: <><path d="M12 4v10M7.5 10 12 14.5 16.5 10"/><path d="M5 19.5h14"/></>,
    'credit-card': <><rect x="3.5" y="5.5" width="17" height="13" rx="3"/><path d="M3.5 10h17M7 15h3"/></>,
    flame: <path d="M12 3.5s4.5 3.5 4.5 8a4.5 4.5 0 0 1-9 0c0-1.2.5-2.2 1-2.8.2 1 .8 1.6 1.5 1.8-.3-2.4 2-4.6 2-7Z"/>,
    leaf: <><path d="M5 19s-1-7 4-11 10-3 10-3 .5 7-3.5 11S5 19 5 19Z"/><path d="M8 16c2-3 5-5 8-6"/></>,
  };

  function Icon({ name, size = 22, stroke = 2, fill = false, style }) {
    const d = P[name];
    if (!d) return null;
    return (
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none"
        stroke="currentColor" strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round"
        style={style} aria-hidden="true">
        {d}
      </svg>
    );
  }

  window.Icon = Icon;
})();
