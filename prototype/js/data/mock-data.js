// Desensitized dataset for SingTail Desktop Prototype (with multi-granularity traffic history)

export const INITIAL_ORGS = [
  {
    id: "org-personal",
    name: "Personal Tailnet",
    account: "alex.dev@gmail.com",
    localTailscaleIp: "100.98.210.14",
    stateDir: "data/ts-state-personal",
    lastSync: "刚刚"
  },
  {
    id: "org-studio",
    name: "GolfJoy Studio",
    account: "ops@golfjoy.io",
    localTailscaleIp: "100.123.88.42",
    stateDir: "data/ts-state-studio",
    lastSync: "1 分钟前"
  }
];

export const INITIAL_EXIT_NODES = [
  {
    id: "ts-us-macmini",
    orgId: "org-studio",
    orgName: "GolfJoy Studio",
    hostname: "us-la-macmini-m2",
    label: "洛杉矶 Mac mini",
    tailscaleIp: "100.123.114.28",
    publicIp: "68.225.41.109",
    region: "US · Los Angeles",
    relayMode: "直连",
    latencyMs: 138,
    online: true,
    healthy: true
  },
  {
    id: "ts-hk-cloud",
    orgId: "org-personal",
    orgName: "Personal Tailnet",
    hostname: "hk-edge-cvm-01",
    label: "香港轻量云",
    tailscaleIp: "100.91.77.63",
    publicIp: "43.153.82.19",
    region: "HK · Hong Kong",
    relayMode: "直连",
    latencyMs: 34,
    online: true,
    healthy: true
  },
  {
    id: "ts-jp-tokyo",
    orgId: "org-personal",
    orgName: "Personal Tailnet",
    hostname: "jp-tyo-router",
    label: "东京软路由",
    tailscaleIp: "100.91.104.12",
    publicIp: "172.104.92.77",
    region: "JP · Tokyo",
    relayMode: "直连",
    latencyMs: 62,
    online: true,
    healthy: true
  },
  {
    id: "ts-sg-relay",
    orgId: "org-studio",
    orgName: "GolfJoy Studio",
    hostname: "sg-build-gateway",
    label: "新加坡网关",
    tailscaleIp: "100.123.45.90",
    publicIp: "18.141.209.54",
    region: "SG · Singapore",
    relayMode: "中继",
    latencyMs: 89,
    online: true,
    healthy: true
  },
  {
    id: "ts-us-broken",
    orgId: "org-personal",
    orgName: "Personal Tailnet",
    hostname: "us-sjc-sandbox",
    label: "圣何塞节点",
    tailscaleIp: "100.91.201.99",
    publicIp: "203.0.113.88",
    region: "US · San Jose",
    relayMode: "中继",
    latencyMs: 285,
    online: true,
    healthy: false
  }
];

// Multi-granularity historical traffic (unit: GB) & per-node distribution (unit: GB)
export const TRAFFIC_PERIODS = {
  live: {
    label: "实时",
    unit: "GB",
    totalDownGB: 16.2,
    totalUpGB: 2.2,
    periodCaption: "今日累计 18.4 GB",
    nodeUsageGB: {
      "ts-us-macmini": 9.4,
      "ts-hk-cloud": 5.3,
      "ts-jp-tokyo": 2.1,
      "ts-sg-relay": 1.6
    }
  },
  day: {
    label: "今日",
    unit: "GB",
    totalDownGB: 16.2,
    totalUpGB: 2.2,
    labels: ["00:00", "02:00", "04:00", "06:00", "08:00", "10:00", "12:00", "14:00", "16:00", "18:00", "20:00", "现在"],
    downSeries: [0.6, 0.3, 0.2, 0.2, 0.9, 2.4, 1.8, 2.1, 2.6, 1.9, 1.8, 1.4],
    upSeries: [0.1, 0.05, 0.03, 0.04, 0.12, 0.35, 0.24, 0.29, 0.36, 0.25, 0.21, 0.16],
    nodeUsageGB: {
      "ts-us-macmini": 9.4,
      "ts-hk-cloud": 5.3,
      "ts-jp-tokyo": 2.1,
      "ts-sg-relay": 1.6
    }
  },
  week: {
    label: "近 7 天",
    unit: "GB",
    totalDownGB: 91.8,
    totalUpGB: 12.4,
    labels: ["11/21", "11/22", "11/23", "11/24", "11/25", "11/26", "今日"],
    downSeries: [11.4, 15.8, 13.2, 10.6, 14.9, 9.7, 16.2],
    upSeries: [1.5, 2.1, 1.8, 1.4, 2.0, 1.4, 2.2],
    nodeUsageGB: {
      "ts-us-macmini": 48.6,
      "ts-hk-cloud": 31.2,
      "ts-jp-tokyo": 14.8,
      "ts-sg-relay": 9.6
    }
  },
  month30: {
    label: "近 30 天",
    unit: "GB",
    totalDownGB: 376.2,
    totalUpGB: 42.4,
    labels: ["10/30", "11/03", "11/07", "11/11", "11/15", "11/19", "11/23", "11/27"],
    downSeries: [38.5, 46.2, 52.0, 41.8, 56.4, 49.1, 44.8, 47.4],
    upSeries: [4.2, 5.1, 5.8, 4.7, 6.3, 5.5, 5.1, 5.7],
    nodeUsageGB: {
      "ts-us-macmini": 212.4,
      "ts-hk-cloud": 118.2,
      "ts-jp-tokyo": 45.6,
      "ts-sg-relay": 42.4
    }
  },
  monthly: {
    label: "按月",
    unit: "GB",
    totalDownGB: 1624.0,
    totalUpGB: 218.5,
    labels: ["12月", "1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月"],
    downSeries: [96, 112, 88, 124, 136, 142, 129, 154, 168, 149, 150, 176],
    upSeries: [12, 15, 11, 17, 18, 19, 17, 21, 23, 20, 21, 24.5],
    nodeUsageGB: {
      "ts-us-macmini": 892.0,
      "ts-hk-cloud": 534.5,
      "ts-jp-tokyo": 246.0,
      "ts-sg-relay": 170.0
    }
  }
};

export const INITIAL_RULES = [
  {
    id: "r-01",
    type: "PROCESS-NAME",
    payload: "tailscaled.exe, verge-mihomo.exe, clash-verge.exe",
    action: "DIRECT",
    hits: 2410,
    enabled: true,
    locked: true
  },
  {
    id: "r-02",
    type: "IP-CIDR",
    payload: "100.64.0.0/10, 192.168.0.0/16, 10.0.0.0/8",
    action: "DIRECT",
    hits: 4219,
    enabled: true,
    locked: true
  },
  {
    id: "r-03",
    type: "PROCESS-NAME",
    payload: "YuanShen.exe, HYP.exe, BetterGI.exe",
    action: "DIRECT",
    hits: 964,
    enabled: true,
    locked: false
  },
  {
    id: "r-04",
    type: "PROCESS-NAME",
    payload: "cloudflared.exe",
    action: "DIRECT",
    hits: 318,
    enabled: true,
    locked: false
  },
  {
    id: "r-05",
    type: "IP-CIDR",
    payload: "8.128.0.0/10",
    action: "DIRECT",
    hits: 612,
    enabled: true,
    locked: false
  },
  {
    id: "r-06",
    type: "DOMAIN-SUFFIX",
    payload: "chatgpt.com, openai.com, claude.ai",
    action: "EXIT",
    hits: 1285,
    enabled: true,
    locked: false
  },
  {
    id: "r-07",
    type: "DOMAIN-SUFFIX",
    payload: "netflix.com, nflxvideo.net",
    action: "FALLBACK_PROXY",
    hits: 142,
    enabled: true,
    locked: false
  },
  {
    id: "r-08",
    type: "RULE-SET",
    payload: "geosite-google",
    action: "EXIT",
    hits: 3504,
    enabled: true,
    locked: false
  },
  {
    id: "r-09",
    type: "RULE-SET",
    payload: "geosite-category-ads-all",
    action: "REJECT",
    hits: 729,
    enabled: true,
    locked: false
  },
  {
    id: "r-10",
    type: "RULE-SET",
    payload: "geosite-cn, geoip-cn",
    action: "DIRECT",
    hits: 29410,
    enabled: true,
    locked: true
  },
  {
    id: "r-11",
    type: "FINAL",
    payload: "默认兜底",
    action: "EXIT",
    hits: 6180,
    enabled: true,
    locked: true
  }
];

export const INITIAL_PROVIDERS = [
  {
    id: "p-geoip-cn",
    tag: "geoip-cn",
    sourceName: "MetaCubeX",
    url: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/cn.srs",
    entryCount: 9420,
    updatedAt: "11-26 14:18"
  },
  {
    id: "p-geosite-cn",
    tag: "geosite-cn",
    sourceName: "SagerNet",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs",
    entryCount: 68510,
    updatedAt: "11-26 14:17"
  },
  {
    id: "p-geosite-noncn",
    tag: "geosite-geolocation-!cn",
    sourceName: "SagerNet",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-geolocation-!cn.srs",
    entryCount: 36920,
    updatedAt: "11-26 14:17"
  },
  {
    id: "p-geosite-openai",
    tag: "geosite-openai",
    sourceName: "SagerNet",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-openai.srs",
    entryCount: 64,
    updatedAt: "11-26 14:17"
  },
  {
    id: "p-geosite-google",
    tag: "geosite-google",
    sourceName: "SagerNet",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-google.srs",
    entryCount: 1490,
    updatedAt: "11-26 14:17"
  },
  {
    id: "p-geosite-github",
    tag: "geosite-github",
    sourceName: "SagerNet",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-github.srs",
    entryCount: 82,
    updatedAt: "11-26 14:17"
  },
  {
    id: "p-geosite-ads",
    tag: "geosite-category-ads-all",
    sourceName: "SagerNet",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs",
    entryCount: 1934,
    updatedAt: "11-26 14:18"
  }
];

export const RUNNING_APPS = [
  { process: "Cursor.exe", path: "C:\\Users\\admin\\AppData\\Local\\Programs\\cursor\\Cursor.exe" },
  { process: "chrome.exe", path: "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" },
  { process: "Code.exe", path: "C:\\Users\\admin\\AppData\\Local\\Programs\\Microsoft VS Code\\Code.exe" },
  { process: "YuanShen.exe", path: "D:\\Games\\Genshin Impact\\Genshin Impact Game\\YuanShen.exe" },
  { process: "steam.exe", path: "C:\\Program Files (x86)\\Steam\\steam.exe" },
  { process: "WeChat.exe", path: "C:\\Program Files\\Tencent\\WeChat\\WeChat.exe" },
  { process: "Telegram.exe", path: "C:\\Users\\admin\\AppData\\Roaming\\Telegram Desktop\\Telegram.exe" }
];

export const INITIAL_CONNECTIONS = [
  {
    id: "c-101",
    process: "Cursor.exe",
    host: "api.anthropic.com",
    dstIp: "160.79.104.25",
    port: 443,
    network: "TCP",
    matchedRule: "FINAL",
    routeAction: "EXIT",
    rttMs: 142,
    downKBps: 420.5,
    totalBytes: "3.8 MB",
    startedAt: "10:56:12"
  },
  {
    id: "c-102",
    process: "chrome.exe",
    host: "github.com",
    dstIp: "140.82.114.4",
    port: 443,
    network: "QUIC",
    matchedRule: "FINAL",
    routeAction: "EXIT",
    rttMs: 138,
    downKBps: 1840.0,
    totalBytes: "14.2 MB",
    startedAt: "10:55:48"
  },
  {
    id: "c-103",
    process: "YuanShen.exe",
    host: "cn-gf01-prd.yuanshen.com",
    dstIp: "8.134.91.50",
    port: 22102,
    network: "UDP",
    matchedRule: "PROCESS · YuanShen.exe",
    routeAction: "DIRECT",
    rttMs: 24,
    downKBps: 28.4,
    totalBytes: "9.1 MB",
    startedAt: "10:31:05"
  },
  {
    id: "c-104",
    process: "chrome.exe",
    host: "www.bilibili.com",
    dstIp: "119.96.135.82",
    port: 443,
    network: "TCP",
    matchedRule: "geosite-cn",
    routeAction: "DIRECT",
    rttMs: 19,
    downKBps: 3120.8,
    totalBytes: "68.4 MB",
    startedAt: "10:52:19"
  },
  {
    id: "c-105",
    process: "Code.exe",
    host: "chatgpt.com",
    dstIp: "172.64.155.209",
    port: 443,
    network: "TCP",
    matchedRule: "chatgpt.com",
    routeAction: "EXIT",
    rttMs: 140,
    downKBps: 196.2,
    totalBytes: "2.1 MB",
    startedAt: "10:54:03"
  },
  {
    id: "c-106",
    process: "verge-mihomo.exe",
    host: "hk-bgp-02.airport-node.net",
    dstIp: "103.214.16.88",
    port: 8443,
    network: "TCP",
    matchedRule: "PROCESS · verge-mihomo.exe",
    routeAction: "DIRECT",
    rttMs: 31,
    downKBps: 2490.0,
    totalBytes: "42.0 MB",
    startedAt: "10:49:33"
  },
  {
    id: "c-107",
    process: "chrome.exe",
    host: "secure-glimr.doubleclick.net",
    dstIp: "0.0.0.0",
    port: 443,
    network: "TCP",
    matchedRule: "geosite-category-ads-all",
    routeAction: "REJECT",
    rttMs: 0,
    downKBps: 0,
    totalBytes: "0 B",
    startedAt: "10:57:02"
  },
  {
    id: "c-108",
    process: "chrome.exe",
    host: "www.netflix.com",
    dstIp: "52.26.71.19",
    port: 443,
    network: "TCP",
    matchedRule: "netflix.com",
    routeAction: "FALLBACK_PROXY",
    rttMs: 56,
    downKBps: 2490.0,
    totalBytes: "42.0 MB",
    startedAt: "10:49:33"
  }
];

export const SAMPLE_IMPORT_RULES_TEXT = `DOMAIN-SUFFIX,anthropic.com,EXIT
DOMAIN-SUFFIX,steamcontent.com,DIRECT
PROCESS-NAME,steam.exe,DIRECT
IP-CIDR,47.96.0.0/11,DIRECT
DOMAIN-KEYWORD,adservice,REJECT`;
