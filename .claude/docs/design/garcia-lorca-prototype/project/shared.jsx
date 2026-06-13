// shared.jsx — brand emblem, top bar, theme toggle, common UI

// module color → CSS var + readable glyph ink for solid chips
const COLOR = {
  lime:   { v:'var(--lime)',   ink:'#0C3318' },
  forest: { v:'var(--forest)', ink:'#FFFFFF' },
  yellow: { v:'var(--yellow)', ink:'#3A2E00' },
  orange: { v:'var(--orange)', ink:'#FFFFFF' },
  purple: { v:'var(--purple)', ink:'#FFFFFF' },
  red:    { v:'var(--red)',    ink:'#FFFFFF' },
  neutral:{ v:'var(--text-muted)', ink:'#FFFFFF' },
};

// ── Brand emblem: colorful 6-petal rosette with GL monogram ──
function Emblem({ size = 44 }) {
  const petals = [
    { a:-90,  c:'var(--lime)' },
    { a:-30,  c:'var(--forest)' },
    { a:30,   c:'var(--yellow)' },
    { a:90,   c:'var(--orange)' },
    { a:150,  c:'var(--purple)' },
    { a:210,  c:'var(--lime-deep)' },
  ];
  const R = 50, pr = 26, dist = 22;
  return (
    <svg width={size} height={size} viewBox="0 0 100 100" aria-label="García Lorca">
      <g>
        {petals.map((p,i) => {
          const rad = p.a * Math.PI/180;
          return <circle key={i} cx={R + Math.cos(rad)*dist} cy={R + Math.sin(rad)*dist} r={pr} fill={p.c} opacity="0.92"/>;
        })}
      </g>
      <circle cx={R} cy={R} r="24" fill="var(--surface)"/>
      <text x="50" y="50" textAnchor="middle" dominantBaseline="central"
        fontFamily="Poppins, sans-serif" fontWeight="800" fontSize="26" fill="var(--forest)">GL</text>
    </svg>
  );
}

function Wordmark({ size = 44, stacked = false }) {
  return (
    <div style={{ display:'flex', alignItems:'center', gap:12 }}>
      <Emblem size={size} />
      <div style={{ lineHeight:1.05 }}>
        <div style={{ fontFamily:'var(--font-display)', fontWeight:800, fontSize:size*0.36, color:'var(--text)', letterSpacing:'-.02em' }}>García Lorca</div>
        <div style={{ fontSize:size*0.24, color:'var(--text-muted)', fontWeight:600, marginTop:2 }}>Centro Educativo</div>
      </div>
    </div>
  );
}

function Avatar({ name, color='lime', size=40, ring=false }) {
  const ini = name.split(' ').slice(0,2).map(w=>w[0]).join('');
  const c = COLOR[color] || COLOR.lime;
  return (
    <div style={{
      width:size, height:size, borderRadius:'50%', flexShrink:0,
      background:c.v, color:c.ink, display:'grid', placeItems:'center',
      fontFamily:'var(--font-display)', fontWeight:700, fontSize:size*0.38,
      boxShadow: ring ? '0 0 0 3px var(--surface), 0 0 0 5px var(--lime)' : 'none',
    }}>{ini}</div>
  );
}

function ThemeToggle({ theme, onToggle }) {
  const dark = theme === 'dark';
  return (
    <button onClick={onToggle} aria-label="Cambiar tema" title={dark?'Modo claro':'Modo oscuro'}
      style={{
        width:46, height:46, borderRadius:'50%', display:'grid', placeItems:'center',
        background:'var(--surface-3)', color:'var(--text)', border:'1px solid var(--border)',
        transition:'all .2s var(--ease)',
      }}
      onMouseEnter={e=>{e.currentTarget.style.background='var(--surface-2)';e.currentTarget.style.color='var(--forest)';}}
      onMouseLeave={e=>{e.currentTarget.style.background='var(--surface-3)';e.currentTarget.style.color='var(--text)';}}>
      <Icon name={dark?'sun':'moon'} size={22} />
    </button>
  );
}

// primary / ghost button
function Btn({ children, kind='primary', icon, onClick, full, danger, style={} }) {
  const base = {
    display:'inline-flex', alignItems:'center', justifyContent:'center', gap:9,
    height:50, padding:'0 22px', borderRadius:'var(--r-pill)',
    fontFamily:'var(--font-display)', fontWeight:600, fontSize:16,
    width: full?'100%':'auto', transition:'all .18s var(--ease)', whiteSpace:'nowrap',
    ...style,
  };
  const kinds = {
    primary: { background:'var(--primary)', color:'var(--primary-ink)', boxShadow:'0 4px 14px color-mix(in oklab, var(--lime) 40%, transparent)' },
    forest:  { background:'var(--forest)', color:'#fff' },
    ghost:   { background:'var(--surface-3)', color:'var(--text)', border:'1px solid var(--border)' },
    outline: { background:'transparent', color:'var(--text)', border:'1.5px solid var(--border-strong)' },
  };
  const k = danger ? { background:'var(--red)', color:'#fff' } : kinds[kind];
  return (
    <button onClick={onClick} style={{ ...base, ...k }}
      onMouseEnter={e=>{ e.currentTarget.style.transform='translateY(-1px)'; e.currentTarget.style.filter='brightness(1.04)'; }}
      onMouseLeave={e=>{ e.currentTarget.style.transform='none'; e.currentTarget.style.filter='none'; }}>
      {icon && <Icon name={icon} size={20} />}{children}
    </button>
  );
}

function IconBtn({ name, onClick, badge, label, size=46 }) {
  return (
    <button onClick={onClick} aria-label={label} title={label}
      style={{ position:'relative', width:size, height:size, borderRadius:'50%', display:'grid', placeItems:'center',
        background:'var(--surface-3)', color:'var(--text)', border:'1px solid var(--border)', transition:'all .2s var(--ease)' }}
      onMouseEnter={e=>{e.currentTarget.style.color='var(--forest)';e.currentTarget.style.background='var(--surface-2)';}}
      onMouseLeave={e=>{e.currentTarget.style.color='var(--text)';e.currentTarget.style.background='var(--surface-3)';}}>
      <Icon name={name} size={22} />
      {badge>0 && <span style={{ position:'absolute', top:6, right:6, minWidth:18, height:18, padding:'0 4px',
        borderRadius:99, background:'var(--red)', color:'#fff', fontSize:11, fontWeight:700,
        display:'grid', placeItems:'center', boxShadow:'0 0 0 2px var(--surface)' }}>{badge}</span>}
    </button>
  );
}

// Desktop top bar
function TopBar({ user, theme, onToggle, onHome, onLogout, search, onSearch }) {
  return (
    <header style={{
      position:'sticky', top:0, zIndex:30, height:78, display:'flex', alignItems:'center', gap:20,
      padding:'0 28px', background:'var(--topbar-bg)', backdropFilter:'blur(18px) saturate(160%)',
      WebkitBackdropFilter:'blur(18px) saturate(160%)', borderBottom:'1px solid var(--border)',
    }}>
      <button onClick={onHome} style={{ display:'flex', alignItems:'center' }} aria-label="Inicio"><Wordmark size={42} /></button>
      <div style={{ flex:1, maxWidth:520, marginLeft:8, position:'relative' }}>
        <span style={{ position:'absolute', left:18, top:'50%', transform:'translateY(-50%)', color:'var(--text-faint)', pointerEvents:'none' }}><Icon name="search" size={20} /></span>
        <input value={search} onChange={e=>onSearch&&onSearch(e.target.value)} placeholder="Buscar alumno, factura, documento…"
          style={{ width:'100%', height:50, paddingLeft:48, paddingRight:18, borderRadius:'var(--r-pill)',
            border:'1px solid var(--border)', background:'var(--surface)', color:'var(--text)', fontSize:15.5, outline:'none' }} />
      </div>
      <div style={{ flex:1 }} />
      <div style={{ display:'flex', alignItems:'center', gap:12 }}>
        <IconBtn name="bell" badge={3} label="Notificaciones" />
        <ThemeToggle theme={theme} onToggle={onToggle} />
        <button onClick={onLogout} title="Cerrar sesión" style={{ display:'flex', alignItems:'center', gap:10, padding:'5px 6px 5px 14px',
          borderRadius:'var(--r-pill)', background:'var(--surface-3)', border:'1px solid var(--border)' }}>
          <div style={{ textAlign:'right', lineHeight:1.15 }}>
            <div style={{ fontWeight:700, fontSize:14, color:'var(--text)', fontFamily:'var(--font-display)' }}>{user.name}</div>
            <div style={{ fontSize:12, color:'var(--text-muted)' }}>{user.role}</div>
          </div>
          <Avatar name={user.name} color={user.color} size={40} />
        </button>
      </div>
    </header>
  );
}

Object.assign(window, { COLOR, Emblem, Wordmark, Avatar, ThemeToggle, Btn, IconBtn, TopBar });
