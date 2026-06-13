// app.jsx — navigation shell, theme, surfaces
const { useState, useEffect } = React;

function ModulePlaceholder({ m, onBack }) {
  const c = COLOR[m.color] || COLOR.neutral;
  return (
    <div style={{ maxWidth:1320, margin:'0 auto', padding:'24px 28px 64px' }}>
      <button onClick={onBack} className="fade-up" style={{ display:'inline-flex', alignItems:'center', gap:7, color:'var(--text-muted)', fontSize:14.5, fontWeight:600, marginBottom:14 }}>
        <Icon name="back" size={18} stroke={2.4}/>Volver al inicio
      </button>
      <div className="fade-up" style={{ display:'flex', alignItems:'center', gap:16, marginBottom:40 }}>
        <div style={{ width:56, height:56, borderRadius:18, display:'grid', placeItems:'center', background: m.color==='neutral'?'var(--surface-3)':c.v, color: m.color==='neutral'?'var(--text-muted)':c.ink }}><Icon name={m.icon} size={30}/></div>
        <div><h1 style={{ fontSize:30, fontWeight:800, color:'var(--text)' }}>{m.label}</h1>
          <p style={{ color:'var(--text-muted)', fontSize:15, marginTop:3 }}>{m.desc}</p></div>
      </div>
      <div className="fade-up" style={{ display:'grid', placeItems:'center', padding:'60px 20px', textAlign:'center', background:'var(--surface)', border:'1px dashed var(--border-strong)', borderRadius:'var(--r-xl)' }}>
        <div style={{ width:96, height:96, borderRadius:28, display:'grid', placeItems:'center', marginBottom:22,
          background:`color-mix(in oklab, ${c.v} 14%, var(--surface))`, color: m.color==='neutral'?'var(--text-muted)':c.v }}><Icon name={m.icon} size={48} stroke={1.8}/></div>
        <h2 style={{ fontSize:24, fontWeight:700, color:'var(--text)' }}>Módulo «{m.label}»</h2>
        <p style={{ color:'var(--text-muted)', fontSize:16, maxWidth:440, marginTop:10, lineHeight:1.5 }}>
          Esta sección sigue el mismo patrón visual que <b style={{ color:'var(--forest)' }}>Finanzas</b>: lista clara, acciones de un clic y estados con color. Ábrela desde el panel para ver el detalle.
        </p>
        <div style={{ display:'flex', gap:12, marginTop:26 }}>
          <Btn icon="plus" kind={m.color==='neutral'?'forest':'primary'}>Crear nuevo</Btn>
          <Btn kind="ghost" onClick={onBack}>Volver</Btn>
        </div>
      </div>
    </div>
  );
}

const USERS = {
  admin:   { name:'Laura Mendizábal', first:'Laura',  role:'Dirección',     color:'forest' },
  teacher: { name:'Miguel Vázquez',   first:'Miguel', role:'Docente · 4° A', color:'purple' },
};

function App() {
  const [theme, setTheme] = useState(() => localStorage.getItem('gl-theme') || 'light');
  const [view, setView] = useState({ screen:'login' });
  const [role, setRole] = useState('admin');
  const [search, setSearch] = useState('');

  useEffect(() => { document.documentElement.setAttribute('data-theme', theme); localStorage.setItem('gl-theme', theme); }, [theme]);
  const toggle = () => setTheme(t => t==='dark'?'light':'dark');

  const login = (r) => { setRole(r); setView({ screen:'dashboard' }); };
  const logout = () => setView({ screen:'login' });
  const open = (id) => setView({ screen: id==='finance' ? 'finance' : 'module', moduleId:id });
  const home = () => setView({ screen:'dashboard' });

  if (view.screen === 'login') return <Login theme={theme} onToggle={toggle} onLogin={login} />;

  // Desktop portal surfaces
  const user = USERS[role] || USERS.admin;
  const mod = view.moduleId ? MODULES.find(m=>m.id===view.moduleId) : null;
  return (
    <div className="app-root" style={{ minHeight:'100vh' }}>
      <TopBar user={user} theme={theme} onToggle={toggle} onHome={home} onLogout={logout} search={search} onSearch={setSearch} />
      {view.screen==='dashboard' && <Dashboard user={user} onOpen={open} />}
      {view.screen==='finance' && <Finances onBack={home} onHome={home} />}
      {view.screen==='module' && mod && <ModulePlaceholder m={mod} onBack={home} />}
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);
