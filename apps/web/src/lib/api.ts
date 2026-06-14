const API_BASE = "";

export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    credentials: "include",
    headers: {
      ...(options.body instanceof FormData
        ? {}
        : { "Content-Type": "application/json" }),
      ...options.headers,
    },
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? "请求失败");
  }
  return res.json() as Promise<T>;
}

export interface SessionUser {
  userId: string;
  username: string;
  displayName: string;
  roleSlugs: string[];
  isAdmin: boolean;
}

export interface Branding {
  logo_url?: string | null;
  primary_color?: string;
  secondary_color?: string;
  login_background?: string | null;
  nav_style?: string;
  button_radius?: string;
  font_size?: string;
  default_theme?: string;
}
