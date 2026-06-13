/* Mock data for the García Lorca parent app (single child). */
(function () {
  const child = {
    name: "Mateo Herrera Díaz",
    short: "Mateo",
    initials: "MH",
    parent: "Laura",
    teacher: "Mtra. Patricia Lugo",
    avatar: { from: "#7AC400", to: "#0A7A2F" },
  };

  // Level config drives grades style + group label.
  const levels = {
    kinder:     { id: "kinder",     name: "Kínder",      group: "K3 · Girasoles", grading: "qual" },
    primaria:   { id: "primaria",   name: "Primaria",    group: "3° A",           grading: "num"  },
    secundaria: { id: "secundaria", name: "Secundaria",  group: "2° B",           grading: "num"  },
  };

  // Today summary
  const today = {
    dateLabel: "vie 12 jun",
    attendance: { status: "present", label: "Presente", detail: "Entrada 7:52 a.m." },
    unread: 2,
    balance: { amount: 1850, status: "warn", label: "Pendiente", due: "Vence 15 jun" },
    nextEvent: { title: "Festival de Primavera", when: "vie 20 jun · 9:00 a.m.", cat: "evento" },
  };

  // June 2026 — 1 Jun is a Monday. status: present/absent/excused/none(weekend/holiday)/future
  const att = {
    monthLabel: "Junio 2026",
    todayDate: 12,
    firstDow: 1, // Monday=1 (Sun=0)
    days: {
      1:"present",2:"present",3:"present",4:"absent",5:"present",
      8:"present",9:"present",10:"excused",11:"present",12:"present",
      15:"future",16:"holiday",17:"future",18:"future",19:"future",
      22:"future",23:"future",24:"future",25:"future",26:"future",
      29:"future",30:"future",
    },
    stats: { pct: 96, faltas: 1, justificadas: 1, retardos: 2 },
    log: [
      { date: "jue 4 jun", status: "absent",  note: "Falta sin justificar" },
      { date: "mié 10 jun", status: "excused", note: "Justificada · Cita médica" },
      { date: "lun 2 jun",  status: "retardo", note: "Retardo · 8:10 a.m." },
    ],
  };

  // Grades — numeric (primaria/secundaria) and qualitative (kínder)
  const gradesNum = {
    average: 9.3,
    period: "2.º periodo",
    subjects: [
      { name: "Español",               grade: 9.2, icon: "book",   tint: "forest", terms: [9.0, 9.1, 9.5] },
      { name: "Matemáticas",           grade: 8.5, icon: "chart",  tint: "orange", terms: [8.0, 8.6, 8.9] },
      { name: "Ciencias Naturales",    grade: 9.6, icon: "leaf",   tint: "lime",   terms: [9.4, 9.6, 9.8] },
      { name: "Historia",              grade: 8.8, icon: "clock",  tint: "purple", terms: [8.5, 8.8, 9.1] },
      { name: "Geografía",             grade: 9.0, icon: "map",    tint: "forest", terms: [8.8, 9.0, 9.2] },
      { name: "Formación C. y Ética",  grade: 9.8, icon: "heart",  tint: "purple", terms: [9.7, 9.8, 9.9] },
      { name: "Inglés",                grade: 9.4, icon: "star",   tint: "lime",   terms: [9.2, 9.4, 9.6] },
      { name: "Educación Física",      grade: 10,  icon: "flame",  tint: "orange", terms: [10, 10, 10] },
    ],
  };

  const gradesQual = {
    period: "2.º periodo",
    summary: { logrado: 4, proceso: 2, apoyo: 0 },
    subjects: [
      { name: "Lenguaje y Comunicación",   level: "logrado", icon: "book" },
      { name: "Pensamiento Matemático",    level: "proceso", icon: "chart" },
      { name: "Exploración del Mundo",     level: "logrado", icon: "leaf" },
      { name: "Desarrollo Personal y Social", level: "logrado", icon: "heart" },
      { name: "Artes",                     level: "logrado", icon: "sparkles" },
      { name: "Educación Física",          level: "proceso", icon: "flame" },
    ],
  };

  const qualMeta = {
    logrado: { label: "Logrado",       chip: "ok",   pct: 100 },
    proceso: { label: "En proceso",    chip: "warn", pct: 60 },
    apoyo:   { label: "Requiere apoyo", chip: "bad", pct: 30 },
  };

  // Balance / account
  const balance = {
    amount: 1850,
    status: "warn",
    label: "Pendiente",
    due: "Vence 15 de junio",
    movements: [
      { date: "02 jun 2026", title: "Pago recibido", sub: "Colegiatura mayo", amount: -2400, kind: "pago" },
      { date: "01 jun 2026", title: "Colegiatura junio", sub: "Cargo mensual", amount: 2400, kind: "cargo" },
      { date: "28 may 2026", title: "Beca aplicada", sub: "Descuento 23%", amount: -550, kind: "descuento" },
      { date: "15 may 2026", title: "Material didáctico", sub: "Pagado", amount: 300, kind: "cargo", paid: true },
      { date: "02 may 2026", title: "Pago recibido", sub: "Colegiatura abril", amount: -2400, kind: "pago" },
    ],
  };

  // Announcements
  const announcements = [
    { id: 1, unread: true,  cat: "evento",        from: "Dirección",        time: "Hace 2 h",
      title: "Festival de Primavera: confirma tu asistencia",
      excerpt: "Los esperamos el viernes 20 de junio a las 9:00 a.m. en el patio central…",
      body: "Estimadas familias:\n\nCon gran alegría los invitamos al Festival de Primavera, donde nuestros alumnos presentarán bailes, música y los proyectos del ciclo escolar.\n\nFecha: viernes 20 de junio\nHora: 9:00 a.m.\nLugar: patio central\n\nPor motivos de aforo, les pedimos confirmar su asistencia antes del 17 de junio." },
    { id: 2, unread: true,  cat: "academico",     from: "Mtra. Patricia Lugo", time: "Hace 5 h",
      title: "Reunión de padres del grupo",
      excerpt: "Revisaremos avances del 2.º periodo y la organización del cierre de ciclo.",
      body: "Buenas tardes:\n\nLos convoco a la reunión informativa del grupo el próximo miércoles 18 de junio a las 5:00 p.m. en el salón.\n\nTrataremos los avances del 2.º periodo y la logística del cierre de ciclo escolar. Su presencia es muy importante." },
    { id: 3, unread: false, cat: "administrativo", from: "Administración",   time: "Ayer",
      title: "Recordatorio de pago de colegiatura",
      excerpt: "La colegiatura de junio vence el día 15. Evita recargos pagando a tiempo.",
      body: "Le recordamos que la colegiatura del mes de junio vence el 15 de junio. Puede realizar su pago desde la sección Cuenta de la app o en caja. Gracias." },
    { id: 4, unread: false, cat: "administrativo", from: "Dirección",        time: "Hace 2 días",
      title: "Suspensión de clases — 16 de junio",
      excerpt: "No habrá clases el lunes 16 por junta de Consejo Técnico Escolar.",
      body: "Informamos que el lunes 16 de junio se suspenden las clases por sesión de Consejo Técnico Escolar. Las actividades se reanudan el martes 17 con normalidad." },
    { id: 5, unread: false, cat: "evento",         from: "Comité ambiental", time: "Hace 4 días",
      title: "Campaña de reciclaje de tapitas",
      excerpt: "Trae tus tapitas de plástico y apoya la causa. Contenedor en la entrada.",
      body: "Iniciamos la campaña de acopio de tapitas de plástico a beneficio de la fundación. Pueden depositarlas en el contenedor verde ubicado en la entrada principal. ¡Gracias por participar!" },
  ];

  const catMeta = {
    evento:         { label: "Evento",         icon: "pin",      tint: "purple" },
    academico:      { label: "Académico",      icon: "book",     tint: "forest" },
    administrativo: { label: "Administrativo", icon: "document", tint: "orange" },
  };

  // Agenda / events
  const events = [
    { day: "16", mon: "JUN", title: "Suspensión de clases", sub: "Consejo Técnico Escolar", tint: "orange" },
    { day: "18", mon: "JUN", title: "Reunión de padres", sub: "5:00 p.m. · Salón 3°A", tint: "forest" },
    { day: "20", mon: "JUN", title: "Festival de Primavera", sub: "9:00 a.m. · Patio central", tint: "purple" },
  ];

  // More modules
  const modules = [
    { id: "documentos", title: "Documentos", sub: "Boletas y constancias", icon: "document", tint: "forest" },
    { id: "uniformes",  title: "Uniformes",  sub: "Tienda y tallas",       icon: "shirt",    tint: "lime"  },
    { id: "facturas",   title: "Facturas",   sub: "CFDI y recibos",        icon: "receipt",  tint: "orange" },
    { id: "becas",      title: "Becas",      sub: "Estado de tu beca",     icon: "cap",      tint: "purple" },
  ];

  const reasons = [
    { id: "medica",   label: "Cita médica" },
    { id: "familiar", label: "Asunto familiar" },
    { id: "enfermedad", label: "Enfermedad" },
    { id: "otro",     label: "Otro motivo" },
  ];

  window.GLDATA = {
    child, levels, today, att,
    gradesNum, gradesQual, qualMeta,
    balance, announcements, catMeta, events, modules, reasons,
  };
})();
