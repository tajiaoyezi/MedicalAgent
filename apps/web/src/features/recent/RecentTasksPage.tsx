import { useEffect, useState } from "react";
import { Pencil, Trash2 } from "lucide-react";
import ModuleShell from "../portal/ModuleShell";
import { api } from "../../lib/api";
import { Button, Tag, EmptyState } from "../../components";

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
  restorable: boolean;
  canContinue: boolean;
  refType?: string | null;
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
    const q = filter.length > 0 ? `?sources=${encodeURIComponent(filter.join(","))}` : "";
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

  // 查看：经恢复分发器按 ref_type 回源（详情由各来源 owner 保证）。
  async function view(id: string) {
    try {
      const res = await api<Record<string, unknown>>(`/api/recent-tasks/${id}/restore`);
      if (res.restorable === false) {
        alert("医疗数字员工执行历史规划中，暂不可恢复。");
        return;
      }
      alert(`恢复来源：${res.source}\n动作：${res.action}\n（详情由对应模块加载）`);
    } catch (e) {
      alert(e instanceof Error ? e.message : "恢复失败");
    }
  }

  async function remove(id: string, title: string) {
    const delDoc = confirm(`确认删除「${title}」？\n\n勾选确定 = 同时删除关联文档（若存在）`);
    await api(`/api/recent-tasks/${id}`, {
      method: "DELETE",
      body: JSON.stringify({ deleteLinkedDocument: delDoc }),
    });
    load();
  }

  function toggle(s: string, checked: boolean) {
    setFilter((prev) => (checked ? [...prev, s] : prev.filter((x) => x !== s)));
  }

  const grouped = tasks.reduce<Record<string, Task[]>>((acc, t) => {
    (acc[t.timeGroup] ??= []).push(t);
    return acc;
  }, {});

  return (
    <ModuleShell title="最近任务" breadcrumb="文档与任务 · 最近任务">
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 18 }}>
        {SOURCES.map((s) => {
          const on = filter.includes(s);
          return (
            <button
              key={s}
              onClick={() => toggle(s, !on)}
              className={`btn btn-sm ${on ? "btn-primary" : "btn-secondary"}`}
            >
              {s}
            </button>
          );
        })}
      </div>

      {tasks.length === 0 ? (
        <EmptyState title="暂无最近任务" desc="在 AIMed、知识库、翻译等模块产生的任务会出现在这里。" />
      ) : (
        Object.entries(grouped).map(([group, items]) => (
          <div key={group} style={{ marginBottom: 22 }}>
            <h3 style={{ fontSize: 13, color: "var(--color-text-3)", margin: "0 0 8px", fontWeight: 700 }}>
              {GROUP_LABEL[group] ?? group}
            </h3>
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {items.map((t) => (
                <div
                  key={t.taskId}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 12,
                    padding: "11px 14px",
                    borderRadius: 11,
                    border: "1px solid var(--color-border)",
                    background: "var(--color-surface)",
                  }}
                >
                  <div style={{ minWidth: 0, flex: 1 }} title={t.title}>
                    <div
                      style={{
                        fontSize: 14,
                        fontWeight: 600,
                        color: "var(--color-text)",
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                      }}
                    >
                      {t.titlePreview}
                    </div>
                    <div style={{ marginTop: 3 }}>
                      <Tag tone="info">{t.source}</Tag>
                    </div>
                  </div>
                  {t.restorable && (
                    <Button size="sm" variant="ghost" onClick={() => view(t.taskId)}>
                      查看
                    </Button>
                  )}
                  {/* 继续追问仅会话类来源（AIMed / 医疗知识库问答）可用 */}
                  {t.canContinue && (
                    <Button size="sm" variant="ghost" onClick={() => view(t.taskId)}>
                      继续追问
                    </Button>
                  )}
                  <Button size="sm" variant="ghost" icon={<Pencil size={14} />} onClick={() => rename(t.taskId, t.title)}>
                    重命名
                  </Button>
                  <Button size="sm" variant="ghost" icon={<Trash2 size={14} />} onClick={() => remove(t.taskId, t.title)}>
                    删除
                  </Button>
                </div>
              ))}
            </div>
          </div>
        ))
      )}
    </ModuleShell>
  );
}
