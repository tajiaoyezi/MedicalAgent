export default function ModuleShell({
  title,
  breadcrumb,
  children,
  toolbar,
}: {
  title: string;
  breadcrumb: string;
  children: React.ReactNode;
  toolbar?: React.ReactNode;
}) {
  return (
    <div>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 16,
        }}
      >
        <div>
          <div className="muted" style={{ fontSize: 12 }}>{breadcrumb}</div>
          <h1 style={{ margin: "4px 0 0", fontSize: 22 }}>{title}</h1>
        </div>
        {toolbar}
      </div>
      <div className="card">{children}</div>
      <p className="muted" style={{ marginTop: 12, fontSize: 12 }}>
        ONLYOFFICE 编辑器原生 UI 不承诺跟随主题，仅外部宿主页面与面板入口适配主题。
      </p>
    </div>
  );
}
