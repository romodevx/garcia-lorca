/* ANNOUNCEMENTS — list with read/unread, + detail view. */
(function () {
  function Announcements({ D, readIds, openAnnouncement, markAll }) {
    const list = D.announcements;
    const unread = list.filter((a) => a.unread && !readIds.has(a.id)).length;

    return (
      <div className="gl-screen-anim">
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "8px 4px 12px" }}>
          <div className="gl-detail-title">Avisos</div>
          {unread > 0 && (
            <button onClick={markAll} style={{ fontSize: 13.5, fontWeight: 800, color: "var(--primary-ink)", whiteSpace: "nowrap" }}>
              Marcar leídos
            </button>
          )}
        </div>

        {list.length === 0 ? (
          <div className="gl-empty">
            <div className="gl-empty-ico"><Icon name="inbox" size={34} /></div>
            <div className="gl-empty-title">No hay avisos nuevos</div>
            <div className="gl-empty-sub">Te avisaremos cuando la escuela publique algo.</div>
          </div>
        ) : (
          <div className="gl-card">
            {list.map((a) => {
              const isUnread = a.unread && !readIds.has(a.id);
              const cat = D.catMeta[a.cat];
              return (
                <div className="gl-ann-row" key={a.id} onClick={() => openAnnouncement(a)} style={{ cursor: "pointer" }}>
                  <span className={"gl-ann-unreaddot" + (isUnread ? "" : " hidden")} />
                  <TintIcon name={cat.icon} tint={cat.tint} size={42} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className={"gl-ann-title " + (isUnread ? "unread" : "read")}>{a.title}</div>
                    <div className="gl-ann-meta">
                      <span style={{ fontWeight: 700, color: "var(--text-2)" }}>{a.from}</span>
                      <span>·</span><span>{a.time}</span>
                    </div>
                    <div className="gl-ann-excerpt">{a.excerpt}</div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    );
  }

  function AnnouncementDetail({ D, ann }) {
    const cat = D.catMeta[ann.cat];
    return (
      <div className="gl-screen-anim">
        <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 14 }}>
          <TintIcon name={cat.icon} tint={cat.tint} size={46} radius={15} icon={24} />
          <div>
            <Chip kind={cat.tint === "purple" ? "purple" : cat.tint === "forest" ? "ok" : "warn"}>{cat.label}</Chip>
          </div>
        </div>
        <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 24, lineHeight: 1.18, letterSpacing: "-0.4px" }}>
          {ann.title}
        </div>
        <div className="gl-ann-meta" style={{ marginTop: 10, fontSize: 13 }}>
          <Icon name="user" size={15} />
          <span style={{ fontWeight: 700, color: "var(--text-2)" }}>{ann.from}</span>
          <span>·</span><span>{ann.time}</span>
        </div>

        <div className="gl-card gl-card-pad" style={{ marginTop: 16 }}>
          <div style={{ fontSize: 14.5, lineHeight: 1.55, color: "var(--text-2)", whiteSpace: "pre-line", fontWeight: 500 }}>
            {ann.body}
          </div>
        </div>

        {ann.cat === "evento" && (
          <button className="gl-btn primary" style={{ marginTop: 16 }}>
            <Icon name="check" size={20} stroke={2.6} /> Confirmar asistencia
          </button>
        )}
      </div>
    );
  }

  Object.assign(window, { Announcements, AnnouncementDetail });
})();
