import {
  Sparkles,
  Library,
  Bot,
  Languages,
  LayoutTemplate,
  FolderOpen,
  History,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";

export interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  planned?: boolean;
  adminOnly?: boolean;
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

export const NAV_GROUPS: NavGroup[] = [
  {
    label: "工作空间",
    items: [
      { to: "/aimed", label: "AIMed 学术助手", icon: Sparkles },
      { to: "/knowledge", label: "医疗知识库", icon: Library },
      { to: "/digital-staff", label: "医疗数字员工", icon: Bot, planned: true },
      { to: "/translation", label: "医学翻译", icon: Languages },
      { to: "/templates", label: "医疗模板库", icon: LayoutTemplate },
    ],
  },
  {
    label: "文档与任务",
    items: [
      { to: "/documents", label: "文档中心", icon: FolderOpen },
      { to: "/recent", label: "最近任务", icon: History },
    ],
  },
  {
    label: "管理",
    items: [{ to: "/admin", label: "管理后台", icon: ShieldCheck, adminOnly: true }],
  },
];

const ALL_ITEMS = NAV_GROUPS.flatMap((g) => g.items);

/** route path → { group, label } for breadcrumbs */
export function routeMeta(pathname: string): { group: string; label: string } {
  for (const g of NAV_GROUPS) {
    const hit = g.items.find((i) => pathname.startsWith(i.to));
    if (hit) return { group: g.label, label: hit.label };
  }
  const fallback = ALL_ITEMS.find((i) => pathname.startsWith(i.to));
  return fallback
    ? { group: "工作空间", label: fallback.label }
    : { group: "工作空间", label: "AIMed 学术助手" };
}
