// data.jsx — mock content for the prototype (Spanish UI)

const MODULES = [
  { id:'students',      label:'Estudiantes',   icon:'students',      color:'forest',  desc:'1,248 alumnos',        badge:null },
  { id:'finance',       label:'Finanzas y Adeudos', icon:'finance', color:'orange',  desc:'37 con adeudo',        badge:37 },
  { id:'attendance',    label:'Asistencia QR', icon:'qr',            color:'lime',    desc:'92% hoy',              badge:null },
  { id:'grades',        label:'Calificaciones',icon:'grades',        color:'yellow',  desc:'Bloque 3 abierto',     badge:null },
  { id:'announcements', label:'Avisos',        icon:'announcements', color:'orange',  desc:'2 nuevos',             badge:2 },
  { id:'scholarships',  label:'Becas',         icon:'scholarship',   color:'purple',  desc:'84 activas',           badge:null },
  { id:'documents',     label:'Documentos',    icon:'documents',     color:'forest',  desc:'Constancias, kardex',  badge:null },
  { id:'uniforms',      label:'Uniformes',     icon:'uniforms',      color:'purple',  desc:'Pedidos y tallas',     badge:null },
  { id:'billing',       label:'Facturación',   icon:'billing',       color:'lime',    desc:'CFDI 4.0',             badge:null },
  { id:'reports',       label:'Reportes',      icon:'reports',       color:'yellow',  desc:'Indicadores',          badge:null },
  { id:'settings',      label:'Configuración', icon:'settings',      color:'neutral', desc:'Centro y permisos',    badge:null },
];

const LEVELS = ['Kínder','Primaria','Secundaria','Preparatoria'];

// finance student rows
const STUDENTS = [
  { id:'GL-2041', name:'Sofía Hernández Ríos',     level:'Primaria',     grade:'4° A', guardian:'Marisol Ríos',     amount:0,     status:'paid',    due:'—',          concept:'Colegiatura junio' },
  { id:'GL-1187', name:'Mateo Domínguez Lara',     level:'Secundaria',   grade:'2° B', guardian:'Jorge Domínguez',  amount:3200,  status:'overdue', due:'5 jun',      concept:'Colegiatura mayo' },
  { id:'GL-3320', name:'Valentina Cruz Mendoza',   level:'Preparatoria', grade:'1° C', guardian:'Ana Mendoza',      amount:1850,  status:'due',     due:'15 jun',     concept:'Colegiatura junio' },
  { id:'GL-2298', name:'Diego Navarro Estrada',    level:'Primaria',     grade:'6° A', guardian:'Luis Navarro',     amount:0,     status:'paid',    due:'—',          concept:'Colegiatura junio' },
  { id:'GL-0455', name:'Renata Flores Aguilar',    level:'Kínder',       grade:'K3',   guardian:'Paola Aguilar',    amount:5400,  status:'overdue', due:'28 may',     concept:'Colegiatura + inscripción' },
  { id:'GL-2710', name:'Emiliano Reyes Castro',    level:'Secundaria',   grade:'1° A', guardian:'Sandra Castro',    amount:0,     status:'paid',    due:'—',          concept:'Colegiatura junio' },
  { id:'GL-1903', name:'Camila Ortega Salinas',    level:'Preparatoria', grade:'2° A', guardian:'Hugo Ortega',      amount:1850,  status:'due',     due:'15 jun',     concept:'Colegiatura junio' },
  { id:'GL-3088', name:'Santiago Vega Morales',    level:'Primaria',     grade:'3° B', guardian:'Karla Morales',    amount:0,     status:'paid',    due:'—',          concept:'Colegiatura junio' },
  { id:'GL-2566', name:'Isabella Ramírez Peña',    level:'Kínder',       grade:'K2',   guardian:'Diana Peña',       amount:2700,  status:'overdue', due:'1 jun',      concept:'Colegiatura mayo' },
  { id:'GL-1422', name:'Sebastián Luna Carrillo',  level:'Secundaria',   grade:'3° C', guardian:'Raúl Luna',        amount:0,     status:'paid',    due:'—',          concept:'Colegiatura junio' },
  { id:'GL-2845', name:'Regina Campos Téllez',     level:'Primaria',     grade:'5° A', guardian:'Mónica Téllez',    amount:1600,  status:'due',     due:'18 jun',     concept:'Material y laboratorio' },
  { id:'GL-3501', name:'Leonardo Fuentes Bravo',   level:'Preparatoria', grade:'3° B', guardian:'Pedro Fuentes',    amount:0,     status:'paid',    due:'—',          concept:'Colegiatura junio' },
];

const FIN_SUMMARY = { collected: 842500, pending: 128300, overdueCount: 37, paidPct: 87 };

// parent app — child profile
const CHILD = {
  name:'Mateo Domínguez Lara',
  short:'Mateo',
  level:'Secundaria · 2° B',
  group:'Grupo García Lorca',
  id:'GL-1187',
  attendancePct: 96,
  present: 168, absent: 4, late: 3, totalDays: 175,
  balance: 3200, balanceConcept:'Colegiatura de mayo',
  nextEvent:{ title:'Festival de primavera', date:'Vie 14 jun · 10:00' },
};

const ATT_WEEK = [
  { d:'Lun', date:'9',  s:'present' },
  { d:'Mar', date:'10', s:'present' },
  { d:'Mié', date:'11', s:'late' },
  { d:'Jue', date:'12', s:'present' },
  { d:'Vie', date:'13', s:'present' },
];

const GRADES = [
  { subject:'Matemáticas',        score:9.4, trend:'up',   teacher:'Prof. Beltrán' },
  { subject:'Español',            score:8.8, trend:'up',   teacher:'Profa. Reséndiz' },
  { subject:'Ciencias',           score:9.1, trend:'flat', teacher:'Prof. Ibáñez' },
  { subject:'Historia',           score:8.2, trend:'down', teacher:'Profa. Quiroz' },
  { subject:'Inglés',             score:9.7, trend:'up',   teacher:'Miss Carter' },
  { subject:'Educación Física',   score:10,  trend:'flat', teacher:'Prof. Solís' },
];

const PARENT_MSGS = [
  { from:'Dirección', subject:'Reunión de padres — 2° B', time:'9:12', unread:true },
  { from:'Profa. Reséndiz', subject:'Entrega de proyecto de Español', time:'Ayer', unread:false },
];

Object.assign(window, { MODULES, LEVELS, STUDENTS, FIN_SUMMARY, CHILD, ATT_WEEK, GRADES, PARENT_MSGS });
