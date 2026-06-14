import { useEffect, useState } from "react";
import ModuleShell from "../portal/ModuleShell";
import { api } from "../../lib/api";

const SOURCES = [
  "AIMed 学术助手",
  "医疗知识库问答",
  "医疗数字员工",
  "医学翻译",
  "在线文档 AI 操作",
  "模板生成文档",
] as const;

interface Task {
  taskId: string;
  source: string;
  title: string;
  titlePreview: string;
  timeGroup: string;
  updatedAt: string;
}

const GROUP_LABEL: Record<string, string> = {
  today: "今天",
  "7d": "7 天内",
  "30d": "30 天内",
  "1y": "1 年内",
  all: "全部",
};

export default function RecentTasksPage() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [filter, setFilter] = useState<string[]>([]);

  async function load() {
    const q =
      filter.length > 0
        ? `?sources=${encodeURIComponent(filter.join(","))}`
        : "";
    const res = await api<{ tasks: Task[] }>(`/api/recent-tasks${q}`);
    setTasks(res.tasks);
  }

  useEffect(() => {
    load();
  }, [filter]);

  async function rename(id: string, title: string) {
    const newTitle = prompt("新标题", title);
    if (!newTitle) return;
    await api(`/api/recent-tasks/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ title: newTitle }),
    });
    load();
  }

  async function remove(id: string, title: string) {
    const delDoc = confirm(
      `确认删除「${title}」？\n\n勾选确定 = 同时删除关联文档（若存在）`,
    );
    await api(`/api/recent-tasks/${id}`, {
      method: "DELETE",
      body: JSON.stringify({ deleteLinkedDocument: delDoc }),
    });
    load();
  }

  const grouped = tasks.reduce<Record<string, Task[]>>((acc, t) => {
    const g = t.timeGroup;
    if (!acc[g]) acc[g] = [];
    acc[g].push(t);
    return acc;
  }, {});

  return (
    <ModuleShell title="最近任务" breadcrumb="最近任务">
      <div style={{ marginBottom: 16 }}>
        <span className="muted">按来源筛选：</span>
        {SOURCES.map((s) => (
          <label key={s} style={{ marginRight: 12 }}>
            <input
              type="checkbox"
              checked={filter.includes(s)}
              onChange={(e) => {
                setFilter((prev) =>
                  e.target.checked ? [...prev, s] : prev.filter((x) => x !== s),
                );
              }}
            />
            {s}
          </label>
        ))}
      </div>
      {tasks.length === 0 ? (
        <p className="muted">暂无最近任务</p>
      ) : (
        Object.entries(grouped).map(([group, items]) => (
          <div key={group} style={{ marginBottom: 24 }}>
            <h3>{GROUP_LABEL[group] ?? group}</h3>
            <ul style={{ listStyle: "none", padding: 0 }}>
              {items.map((t) => (
                <li
                  key={t.taskId}
                  style={{
                    padding: "8px 0",
                    borderBottom: "1px solid #f0f0f0",
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                  }}
                >
                  <span title={t.title}>
                    {t.titlePreview}
                    <span className="muted" style={{ marginLeft: 8, fontSize: 12 }}>
                      {t.source}
                    </span>
                  </span>
                  <span>
                    <button className="ghost" onClick={() => rename(t.taskId, t.title)}>
                      重命名
                    </button>
                    <button className="ghost" onClick={() => remove(t.taskId, t.title)}>
                      删除
                    </button>
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ))
      )}
    </ModuleShell>
  );
}
