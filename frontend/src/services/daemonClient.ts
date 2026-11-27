import type {
  DashboardSnapshot,
  OrgLoginSession,
  RoutingMode,
  RuleAction,
  RuleType,
  SanitizedExport,
  SystemSettings,
  TrafficPeriodKey,
} from "../types/contract";

export interface AddRulePayload {
  type: RuleType;
  payload: string;
  action: RuleAction;
}

export interface AddProviderPayload {
  tag: string;
  sourceName: string;
  url: string;
}

export interface AddOrgPayload {
  name: string;
  account: string;
  nodeHostname: string;
  publicIp: string;
  region: string;
}

export interface StartOrgLoginPayload {
  alias: string;
  controlUrl?: string;
}

export interface ImportOrgStateDirPayload {
  sourceDir: string;
  alias?: string;
}

export interface QuickRulePayload {
  connId: string;
  matchKind: "domain" | "process";
  action: RuleAction;
}

async function requestJson<T>(url: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(url, {
    ...init,
    headers,
  });

  if (!response.ok) {
    let message = `请求失败 (${response.status})`;
    const text = await response.text();
    if (text) {
      try {
        const parsed = JSON.parse(text) as { error?: unknown; message?: unknown };
        if (typeof parsed.error === "string" && parsed.error.trim()) {
          message = parsed.error;
        } else if (typeof parsed.message === "string" && parsed.message.trim()) {
          message = parsed.message;
        } else {
          message = text;
        }
      } catch {
        message = text;
      }
    }
    throw new Error(message);
  }

  return (await response.json()) as T;
}

export const daemonClient = {
  getSnapshot(period: TrafficPeriodKey = "live"): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>(
      `/api/snapshot?period=${encodeURIComponent(period)}`,
    );
  },

  setTun(enabled: boolean): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/tun", {
      method: "POST",
      body: JSON.stringify({ enabled }),
    });
  },

  setMode(mode: RoutingMode): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/mode", {
      method: "POST",
      body: JSON.stringify({ mode }),
    });
  },

  switchExit(exitId: string): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/exit/switch", {
      method: "POST",
      body: JSON.stringify({ exitId }),
    });
  },

  pingExits(): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/exit/ping", {
      method: "POST",
      body: JSON.stringify({}),
    });
  },

  dismissBanner(): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/banner/dismiss", {
      method: "POST",
      body: JSON.stringify({}),
    });
  },

  addRule(payload: AddRulePayload): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/rules", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  toggleRule(ruleId: string): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/rules/toggle", {
      method: "POST",
      body: JSON.stringify({ ruleId }),
    });
  },

  moveRule(ruleId: string, direction: -1 | 1): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/rules/move", {
      method: "POST",
      body: JSON.stringify({ ruleId, direction }),
    });
  },

  deleteRule(ruleId: string): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>(
      `/api/rules?id=${encodeURIComponent(ruleId)}`,
      {
        method: "DELETE",
      },
    );
  },

  importRules(rawText: string): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/rules/import", {
      method: "POST",
      body: JSON.stringify({ rawText }),
    });
  },

  quickRule(payload: QuickRulePayload): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/connections/quick-rule", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  addProvider(payload: AddProviderPayload): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/providers", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  refreshProviders(): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/providers/refresh", {
      method: "POST",
      body: JSON.stringify({}),
    });
  },

  addOrg(payload: AddOrgPayload): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/orgs", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  startOrgLogin(req: StartOrgLoginPayload): Promise<OrgLoginSession> {
    return requestJson<OrgLoginSession>("/api/orgs/login/start", {
      method: "POST",
      body: JSON.stringify(req),
    });
  },

  pollOrgLogin(sessionId: string): Promise<OrgLoginSession> {
    return requestJson<OrgLoginSession>(
      `/api/orgs/login/poll?sessionId=${encodeURIComponent(sessionId)}`,
    );
  },

  importOrgStateDir(
    req: ImportOrgStateDirPayload,
  ): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/orgs/import-dir", {
      method: "POST",
      body: JSON.stringify(req),
    });
  },

  removeOrg(orgId: string): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>(
      `/api/orgs?id=${encodeURIComponent(orgId)}`,
      {
        method: "DELETE",
      },
    );
  },

  syncOrg(orgId: string): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/orgs/sync", {
      method: "POST",
      body: JSON.stringify({ orgId }),
    });
  },

  updateSettings(settings: SystemSettings): Promise<DashboardSnapshot> {
    return requestJson<DashboardSnapshot>("/api/settings", {
      method: "PUT",
      body: JSON.stringify(settings),
    });
  },

  exportConfig(): Promise<SanitizedExport> {
    return requestJson<SanitizedExport>("/api/export");
  },

  openExternalUrl(url: string): Promise<{ ok: boolean }> {
    return requestJson<{ ok: boolean }>("/api/browser/open", {
      method: "POST",
      body: JSON.stringify({ url }),
    });
  },
};
