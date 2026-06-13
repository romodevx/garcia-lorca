// screen_parent.jsx — parent mobile app content (rendered inside IOSDevice)
function gradeColor(s){ return s>=9 ? 'var(--forest)' : s>=8 ? 'var(--lime-deep)' : s>=7 ? 'var(--orange)' : 'var(--red)'; }
const ATT = { present:{c:'var(--forest)',l:'Asistió'}, late:{c:'var(--yellow)',l:'Retardo'}, absent:{c:'var(--red)',l:'Falta'} };

function Ring({ pct, size=84 }) {
  const r=(size-12)/2, C=2*Math.PI*r;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke="var(--surface-3)" strokeWidth="9"/>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke="var(--lime)" strokeWidth="9" strokeLinecap="round"
        strokeDasharray={C} strokeDashoffset={C*(1-pct/100)} transform={`rotate(-90 ${size/2} ${size/2})`} style={{ transition:'stroke-dashoffset 1s var(--ease)' }}/>
      <text x="50%" y="50%" textAnchor="middle" dominantBaseline="central" fontFamily="Poppins" fontWeight="800" fontSize={size*0.26} fill="var(--text)">{pct}%</text>
    </svg>
  );
}

function PCard({ children, style={} }) {
  return <div style={{ background:'var(--surface)', border:'1px solid var(--border)', borderRadius:22, padding:18, boxShadow:'var(--shadow-card)', ...style }}>{children}</div>;
}
function SectionTitle({ children, action, onAction }) {
  return <div style={{ display:'flex', alignItems:'center', justifyContent:'space-between', margin:'4px 4px 12px' }}>
    <h3 style={{ fontSize:18, fontWeight:700, color:'var(--text)' }}>{children}</h3>
    {action && <button onClick={onAction} style={{ fontSize:14, fontWeight:600, color:'var(--forest)' }}>{action}</button>}
  </div>;
}

function ParentApp({ theme, onLogout }) {
  const [tab, setTab] = React.useState('home');
  const [paid, setPaid] = React.useState(false);
  const c = CHILD;
  const avg = (GRADES.reduce((a,b)=>a+b.score,0)/GRADES.length).toFixed(1);

  const header = (title, sub) => (
    <div style={{ padding:'8px 20px 14px', display:'flex', alignItems:'center', justifyContent:'space-between' }}>
      <div><div style={{ fontSize:13, color:'var(--text-faint)', fontWeight:600 }}>{sub}</div>
        <h1 style={{ fontSize:26, fontWeight:800, color:'var(--text)', letterSpacing:'-.02em' }}>{title}</h1></div>
      <button style={{ position:'relative', width:44, height:44, borderRadius:'50%', display:'grid', placeItems:'center', background:'var(--surface)', border:'1px solid var(--border)', color:'var(--text)' }}>
        <Icon name="bell" size={22}/><span style={{ position:'absolute', top:8, right:9, width:9, height:9, borderRadius:99, background:'var(--red)', boxShadow:'0 0 0 2px var(--surface)' }}/></button>
    </div>
  );

  // child banner
  const childBanner = (
    <div style={{ margin:'0 20px 18px', borderRadius:22, padding:'16px 18px', display:'flex', alignItems:'center', gap:14,
      background:'linear-gradient(135deg, var(--forest), color-mix(in oklab, var(--forest) 70%, #04331A))', color:'#fff', boxShadow:'0 10px 26px color-mix(in oklab, var(--forest) 30%, transparent)' }}>
      <div style={{ width:52, height:52, borderRadius:'50%', flexShrink:0, display:'grid', placeItems:'center', background:'rgba(255,255,255,.18)', fontFamily:'Poppins', fontWeight:700, fontSize:20 }}>{c.short[0]}D</div>
      <div style={{ flex:1, minWidth:0 }}>
        <div style={{ fontFamily:'Poppins', fontWeight:700, fontSize:17 }}>{c.name}</div>
        <div style={{ fontSize:13.5, color:'rgba(255,255,255,.82)' }}>{c.level} · {c.id}</div>
      </div>
      <button style={{ display:'flex', alignItems:'center', gap:4, padding:'7px 12px', borderRadius:99, background:'rgba(255,255,255,.16)', color:'#fff', fontSize:13, fontWeight:600 }}>Cambiar<Icon name="chevronDown" size={15}/></button>
    </div>
  );

  const balanceCard = (
    <PCard style={{ padding:0, overflow:'hidden', borderColor: paid?'var(--border)':'color-mix(in oklab, var(--orange) 40%, var(--border))' }}>
      <div style={{ padding:18, display:'flex', alignItems:'center', gap:14 }}>
        <div style={{ width:48, height:48, borderRadius:14, display:'grid', placeItems:'center', flexShrink:0,
          background: paid?'var(--ok-bg)':'color-mix(in oklab, var(--orange) 16%, var(--surface))', color: paid?'var(--forest)':'var(--orange)' }}>
          <Icon name={paid?'check':'finance'} size={26} stroke={paid?2.6:2}/></div>
        <div style={{ flex:1 }}>
          <div style={{ fontSize:13.5, color:'var(--text-muted)', fontWeight:600 }}>{paid?'Estado de cuenta':'Adeudo actual'}</div>
          <div style={{ fontFamily:'Poppins', fontWeight:800, fontSize:24, color: paid?'var(--forest)':'var(--text)', lineHeight:1.1 }}>{paid?'Al corriente':'$3,200'}</div>
          <div style={{ fontSize:13, color:'var(--text-faint)' }}>{paid?'Gracias por tu pago':c.balanceConcept}</div>
        </div>
      </div>
      {!paid && <button onClick={()=>setPaid(true)} style={{ width:'100%', height:52, background:'var(--primary)', color:'var(--primary-ink)', fontFamily:'Poppins', fontWeight:700, fontSize:16, display:'flex', alignItems:'center', justifyContent:'center', gap:9 }}>
        <Icon name="card" size={20}/>Pagar ahora</button>}
    </PCard>
  );

  const TABS = {
    home: (
      <>
        {header('Hola, Jorge','Familia Domínguez')}
        {childBanner}
        <div style={{ padding:'0 20px 20px', display:'flex', flexDirection:'column', gap:18 }}>
          {/* attendance */}
          <PCard>
            <div style={{ display:'flex', alignItems:'center', gap:16 }}>
              <Ring pct={c.attendancePct}/>
              <div style={{ flex:1 }}>
                <div style={{ fontFamily:'Poppins', fontWeight:700, fontSize:17, color:'var(--text)' }}>Asistencia</div>
                <div style={{ fontSize:13.5, color:'var(--text-muted)', marginTop:2 }}>{c.present} días presente de {c.totalDays}</div>
                <div style={{ display:'flex', gap:14, marginTop:10 }}>
                  {[['Faltas',c.absent,'var(--red)'],['Retardos',c.late,'var(--yellow)']].map(([l,v,col])=>
                    <div key={l} style={{ display:'flex', alignItems:'center', gap:6 }}><span style={{ width:8,height:8,borderRadius:99,background:col }}/><span style={{ fontSize:13, color:'var(--text-muted)' }}><b style={{ color:'var(--text)' }}>{v}</b> {l}</span></div>)}
                </div>
              </div>
            </div>
            <div style={{ display:'flex', gap:8, marginTop:16, paddingTop:16, borderTop:'1px solid var(--border)' }}>
              {ATT_WEEK.map(d=>{ const a=ATT[d.s]; return (
                <div key={d.date} style={{ flex:1, textAlign:'center' }}>
                  <div style={{ fontSize:11.5, color:'var(--text-faint)', fontWeight:600 }}>{d.d}</div>
                  <div style={{ width:34, height:34, margin:'6px auto 0', borderRadius:12, display:'grid', placeItems:'center',
                    background:`color-mix(in oklab, ${a.c} 18%, var(--surface))`, color:a.c, fontFamily:'Poppins', fontWeight:700, fontSize:14 }}>
                    {d.s==='present'?<Icon name="check" size={16} stroke={3}/>:d.s==='late'?'R':'F'}</div>
                  <div style={{ fontSize:11, color:'var(--text-faint)', marginTop:4 }}>{d.date}</div>
                </div> ); })}
            </div>
          </PCard>

          {balanceCard}

          {/* grades preview */}
          <div>
            <SectionTitle action="Ver todas" onAction={()=>setTab('grades')}>Calificaciones</SectionTitle>
            <PCard style={{ padding:8 }}>
              <div style={{ display:'flex', alignItems:'center', justifyContent:'space-between', padding:'10px 12px', marginBottom:4 }}>
                <span style={{ fontSize:14, color:'var(--text-muted)', fontWeight:600 }}>Promedio general · Bloque 3</span>
                <span style={{ fontFamily:'Poppins', fontWeight:800, fontSize:22, color:'var(--forest)' }}>{avg}</span>
              </div>
              {GRADES.slice(0,3).map(g=>(
                <div key={g.subject} style={{ display:'flex', alignItems:'center', gap:12, padding:'11px 12px', borderTop:'1px solid var(--border)' }}>
                  <div style={{ flex:1, fontSize:15, fontWeight:600, color:'var(--text)' }}>{g.subject}</div>
                  {g.trend==='up' && <span style={{ color:'var(--forest)' }}><Icon name="trend" size={16}/></span>}
                  <span style={{ fontFamily:'Poppins', fontWeight:800, fontSize:18, color:gradeColor(g.score) }}>{g.score.toFixed(1)}</span>
                </div>
              ))}
            </PCard>
          </div>

          {/* next event */}
          <PCard style={{ display:'flex', alignItems:'center', gap:14 }}>
            <div style={{ width:48, height:48, borderRadius:14, flexShrink:0, display:'grid', placeItems:'center', background:'color-mix(in oklab, var(--purple) 16%, var(--surface))', color:'var(--purple)' }}><Icon name="calendar" size={24}/></div>
            <div style={{ flex:1 }}><div style={{ fontSize:13, color:'var(--text-faint)', fontWeight:600 }}>Próximo evento</div>
              <div style={{ fontFamily:'Poppins', fontWeight:700, fontSize:16, color:'var(--text)' }}>{c.nextEvent.title}</div>
              <div style={{ fontSize:13, color:'var(--text-muted)' }}>{c.nextEvent.date}</div></div>
          </PCard>
        </div>
      </>
    ),
    grades: (
      <>
        {header('Calificaciones','Bloque 3 · 2° B')}
        <div style={{ padding:'0 20px 20px', display:'flex', flexDirection:'column', gap:14 }}>
          <PCard style={{ display:'flex', alignItems:'center', gap:16, background:'linear-gradient(135deg, color-mix(in oklab, var(--lime) 16%, var(--surface)), var(--surface))' }}>
            <div style={{ width:64, height:64, borderRadius:18, display:'grid', placeItems:'center', background:'var(--lime)', color:'var(--primary-ink)', fontFamily:'Poppins', fontWeight:800, fontSize:26 }}>{avg}</div>
            <div><div style={{ fontFamily:'Poppins', fontWeight:700, fontSize:18, color:'var(--text)' }}>Promedio general</div>
              <div style={{ fontSize:14, color:'var(--text-muted)' }}>6 materias · sube 0.3 vs bloque 2</div></div>
          </PCard>
          <PCard style={{ padding:6 }}>
            {GRADES.map((g,i)=>(
              <div key={g.subject} style={{ display:'flex', alignItems:'center', gap:13, padding:'13px 12px', borderTop:i?'1px solid var(--border)':'none' }}>
                <div style={{ width:40, height:40, borderRadius:12, display:'grid', placeItems:'center', background:`color-mix(in oklab, ${gradeColor(g.score)} 15%, var(--surface))`, color:gradeColor(g.score) }}><Icon name="grades" size={21}/></div>
                <div style={{ flex:1 }}><div style={{ fontSize:15.5, fontWeight:600, color:'var(--text)' }}>{g.subject}</div>
                  <div style={{ fontSize:12.5, color:'var(--text-faint)' }}>{g.teacher}</div></div>
                <span style={{ fontFamily:'Poppins', fontWeight:800, fontSize:22, color:gradeColor(g.score) }}>{g.score.toFixed(1)}</span>
              </div>
            ))}
          </PCard>
        </div>
      </>
    ),
    pay: (
      <>
        {header('Pagos','Familia Domínguez')}
        <div style={{ padding:'0 20px 20px', display:'flex', flexDirection:'column', gap:16 }}>
          {balanceCard}
          <SectionTitle>Historial</SectionTitle>
          <PCard style={{ padding:6 }}>
            {[['Colegiatura abril','$3,200','12 abr','paid'],['Colegiatura marzo','$3,200','10 mar','paid'],['Inscripción anual','$8,500','15 ago','paid']].map((r,i)=>(
              <div key={i} style={{ display:'flex', alignItems:'center', gap:12, padding:'13px 12px', borderTop:i?'1px solid var(--border)':'none' }}>
                <div style={{ width:38, height:38, borderRadius:11, display:'grid', placeItems:'center', background:'var(--ok-bg)', color:'var(--forest)' }}><Icon name="check" size={19} stroke={2.6}/></div>
                <div style={{ flex:1 }}><div style={{ fontSize:15, fontWeight:600, color:'var(--text)' }}>{r[0]}</div><div style={{ fontSize:12.5, color:'var(--text-faint)' }}>Pagado · {r[2]}</div></div>
                <span style={{ fontFamily:'Poppins', fontWeight:700, fontSize:15, color:'var(--text)' }}>{r[1]}</span>
              </div>
            ))}
          </PCard>
        </div>
      </>
    ),
    news: (
      <>
        {header('Avisos','García Lorca')}
        <div style={{ padding:'0 20px 20px', display:'flex', flexDirection:'column', gap:12 }}>
          {PARENT_MSGS.map((m,i)=>(
            <PCard key={i} style={{ display:'flex', gap:13, alignItems:'flex-start', borderColor:m.unread?'color-mix(in oklab, var(--lime) 40%, var(--border))':'var(--border)' }}>
              <div style={{ width:44, height:44, borderRadius:13, flexShrink:0, display:'grid', placeItems:'center', background:'color-mix(in oklab, var(--orange) 15%, var(--surface))', color:'var(--orange)' }}><Icon name="announcements" size={22}/></div>
              <div style={{ flex:1 }}>
                <div style={{ display:'flex', justifyContent:'space-between', gap:8 }}><span style={{ fontFamily:'Poppins', fontWeight:700, fontSize:15, color:'var(--text)' }}>{m.from}</span><span style={{ fontSize:12, color:'var(--text-faint)' }}>{m.time}</span></div>
                <div style={{ fontSize:14, color:'var(--text-muted)', marginTop:3 }}>{m.subject}</div>
              </div>
              {m.unread && <span style={{ width:9, height:9, borderRadius:99, background:'var(--lime)', marginTop:6 }}/>}
            </PCard>
          ))}
          <PCard style={{ display:'flex', alignItems:'center', gap:14 }}>
            <div style={{ width:48, height:48, borderRadius:14, flexShrink:0, display:'grid', placeItems:'center', background:'color-mix(in oklab, var(--purple) 16%, var(--surface))', color:'var(--purple)' }}><Icon name="calendar" size={24}/></div>
            <div style={{ flex:1 }}><div style={{ fontSize:13, color:'var(--text-faint)', fontWeight:600 }}>Evento</div>
              <div style={{ fontFamily:'Poppins', fontWeight:700, fontSize:15.5, color:'var(--text)' }}>{CHILD.nextEvent.title}</div>
              <div style={{ fontSize:13, color:'var(--text-muted)' }}>{CHILD.nextEvent.date}</div></div>
          </PCard>
        </div>
      </>
    ),
  };

  const navItems = [ ['home','Inicio','students'], ['grades','Notas','grades'], ['pay','Pagos','finance'], ['news','Avisos','announcements'] ];

  return (
    <div style={{ height:'100%', display:'flex', flexDirection:'column', background:'var(--bg)', backgroundImage:'var(--bg-grad)' }}>
      <div style={{ flex:1, overflowY:'auto', paddingTop:54 }}>
        {TABS[tab]}
        <div style={{ height:90 }}/>
      </div>
      {/* bottom nav */}
      <div style={{ position:'absolute', left:0, right:0, bottom:0, paddingBottom:30, paddingTop:8, background:'var(--topbar-bg)', backdropFilter:'blur(18px) saturate(160%)', WebkitBackdropFilter:'blur(18px) saturate(160%)', borderTop:'1px solid var(--border)' }}>
        <div style={{ display:'flex', justifyContent:'space-around', padding:'0 8px' }}>
          {navItems.map(([id,label,icon])=>{ const on=tab===id; return (
            <button key={id} onClick={()=>setTab(id)} style={{ flex:1, display:'flex', flexDirection:'column', alignItems:'center', gap:4, padding:'6px 0', color:on?'var(--forest)':'var(--text-faint)' }}>
              <div style={{ width:46, height:30, borderRadius:99, display:'grid', placeItems:'center', background:on?'color-mix(in oklab, var(--lime) 20%, transparent)':'transparent', transition:'background .15s' }}><Icon name={icon} size={22} stroke={on?2.3:2}/></div>
              <span style={{ fontSize:11, fontWeight:on?700:600, fontFamily:'var(--font-display)' }}>{label}</span>
            </button> ); })}
        </div>
      </div>
    </div>
  );
}

window.ParentApp = ParentApp;
