// Desensitized realistic dataset for SingTail Desktop Prototype

export const INITIAL_ORGS = [
  {
    id: "org-personal",
    name: "Personal Tailnet",
    account: "alex.dev@gmail.com",
    tailnetDomain: "tail8f2a1.ts.net",
    localNodeName: "singtail-win-workstation",
    localTailscaleIp: "100.98.210.14",
    stateDir: "data/ts-state-personal",
    authStatus: "authenticated",
    lastSync: "刚刚"
  },
  {
    id: "org-studio",
    name: "GolfJoy Studio",
    account: "ops@golfjoy.io",
    tailnetDomain: "golfjoy-mesh.ts.net",
    localNodeName: "singtail-studio-win11",
    localTailscaleIp: "100.123.88.42",
    stateDir: "data/ts-state-studio",
    authStatus: "authenticated",
    lastSync: "1 分钟前"
  }
];

export const INITIAL_EXIT_NODES = [
  {
    id: "ts-us-macmini",
    orgId: "org-studio",
    orgName: "GolfJoy Studio",
    hostname: "us-la-macmini-m2",
    label: "洛杉矶 Mac mini (家宽原生)",
    tailscaleIp: "100.123.114.28",
    publicIp: "68.225.41.109",
    region: "US · Los Angeles",
    ispType: "Residential ISP",
    relayMode: "P2P 直连",
    latencyMs: 138,
    online: true,
    healthy: true
  },
  {
    id: "ts-hk-cloud",
    orgId: "org-personal",
    orgName: "Personal Tailnet",
    hostname: "hk-edge-cvm-01",
    label: "香港轻量云出口",
    tailscaleIp: "100.91.77.63",
    publicIp: "43.153.82.19",
    region: "HK · Hong Kong",
    ispType: "Cloud BGP",
    relayMode: "P2P 直连",
    latencyMs: 34,
    online: true,
    healthy: true
  },
  {
    id: "ts-jp-tokyo",
    orgId: "org-personal",
    orgName: "Personal Tailnet",
    hostname: "jp-tyo-router",
    label: "东京软路由出口",
    tailscaleIp: "100.91.104.12",
    publicIp: "172.104.92.77",
    region: "JP · Tokyo",
    ispType: "Linode",
    relayMode: "P2P 直连",
    latencyMs: 62,
    online: true,
    healthy: true
  },
  {
    id: "ts-sg-relay",
    orgId: "org-studio",
    orgName: "GolfJoy Studio",
    hostname: "sg-build-gateway",
    label: "新加坡编译网关",
    tailscaleIp: "100.123.45.90",
    publicIp: "18.141.209.54",
    region: "SG · Singapore",
    ispType: "AWS EC2",
    relayMode: "DERP 中继 (hkg)",
    latencyMs: 89,
    online: true,
    healthy: true
  },
  {
    id: "ts-us-broken",
    orgId: "org-personal",
    orgName: "Personal Tailnet",
    hostname: "us-sjc-sandbox",
    label: "圣何塞测试节点 (模拟异常回滚)",
    tailscaleIp: "100.91.201.99",
    publicIp: "203.0.113.88",
    region: "US · San Jose",
    ispType: "VPS",
    relayMode: "DERP 中继 (sfo)",
    latencyMs: 285,
    online: true,
    healthy: false
  }
];

export const INITIAL_RULES = [
  {
    id: "r-01",
    type: "PROCESS-NAME",
    payload: "tailscaled.exe, tailscale-ipn.exe",
    action: "DIRECT",
    note: "本机原生 Tailscale 客户端直连避让",
    hits: 1842,
    enabled: true,
    locked: true
  },
  {
    id: "r-02",
    type: "IP-CIDR",
    payload: "100.64.0.0/10, 192.168.0.0/16, 10.0.0.0/8",
    action: "DIRECT",
    note: "Tailnet 内网与局域网直连",
    hits: 4219,
    enabled: true,
    locked: true
  },
  {
    id: "r-03",
    type: "PROCESS-NAME",
    payload: "YuanShen.exe, HYP.exe, BetterGI.exe",
    action: "DIRECT",
    note: "国服游戏强制直连防绕境外",
    hits: 964,
    enabled: true,
    locked: false
  },
  {
    id: "r-04",
    type: "PROCESS-NAME",
    payload: "cloudflared.exe",
    action: "DIRECT",
    note: "Cloudflare 边缘隧道直连",
    hits: 318,
    enabled: true,
    locked: false
  },
  {
    id: "r-05",
    type: "IP-CIDR",
    payload: "8.128.0.0/10",
    action: "DIRECT",
    note: "阿里云国内网段修正（防海外库误判）",
    hits: 612,
    enabled: true,
    locked: false
  },
  {
    id: "r-06",
    type: "DOMAIN-SUFFIX",
    payload: "chatgpt.com, openai.com, claude.ai",
    action: "EXIT",
    note: "AI 服务走当前出口",
    hits: 1285,
    enabled: true,
    locked: false
  },
  {
    id: "r-07",
    type: "DOMAIN-SUFFIX",
    payload: "netflix.com, nflxvideo.net",
    action: "FALLBACK_PROXY",
    note: "流媒体走外部兜底代理端口",
    hits: 142,
    enabled: true,
    locked: false
  },
  {
    id: "r-08",
    type: "RULE-SET",
    payload: "geosite-google",
    action: "EXIT",
    note: "Google 全域走隧道（消除 gvt1 5秒卡顿）",
    hits: 3504,
    enabled: true,
    locked: false
  },
  {
    id: "r-09",
    type: "RULE-SET",
    payload: "geosite-category-ads-all",
    action: "REJECT",
    note: "广告追踪域名拦截",
    hits: 729,
    enabled: true,
    locked: false
  },
  {
    id: "r-10",
    type: "RULE-SET",
    payload: "geosite-cn, geoip-cn",
    action: "DIRECT",
    note: "国内域名与国内 IP 兜底直连",
    hits: 29410,
    enabled: true,
    locked: true
  },
  {
    id: "r-11",
    type: "FINAL",
    payload: "其余未命中流量",
    action: "EXIT",
    note: "默认兜底走当前选中出口",
    hits: 6180,
    enabled: true,
    locked: true
  }
];

export const INITIAL_PROVIDERS = [
  {
    id: "p-geoip-cn",
    tag: "geoip-cn",
    format: "SRS Binary",
    sourceName: "MetaCubeX / meta-rules-dat",
    url: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/cn.srs",
    entryCount: 9420,
    updatedAt: "2026-11-26 14:18",
    badge: "含阿里云 8.128.0.0/10",
    verified: true
  },
  {
    id: "p-geosite-cn",
    tag: "geosite-cn",
    format: "SRS Binary",
    sourceName: "SagerNet / sing-geosite",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs",
    entryCount: 68510,
    updatedAt: "2026-11-26 14:17",
    badge: "国内直连主表",
    verified: true
  },
  {
    id: "p-geosite-noncn",
    tag: "geosite-geolocation-!cn",
    format: "SRS Binary",
    sourceName: "SagerNet / sing-geosite",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-geolocation-!cn.srs",
    entryCount: 36920,
    updatedAt: "2026-11-26 14:17",
    badge: "境外加密解析表",
    verified: true
  },
  {
    id: "p-geosite-openai",
    tag: "geosite-openai",
    format: "SRS Binary",
    sourceName: "SagerNet / sing-geosite",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-openai.srs",
    entryCount: 64,
    updatedAt: "2026-11-26 14:17",
    badge: "AI 服务",
    verified: true
  },
  {
    id: "p-geosite-google",
    tag: "geosite-google",
    format: "SRS Binary",
    sourceName: "SagerNet / sing-geosite",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-google.srs",
    entryCount: 1490,
    updatedAt: "2026-11-26 14:17",
    badge: "Google 全家桶",
    verified: true
  },
  {
    id: "p-geosite-github",
    tag: "geosite-github",
    format: "SRS Binary",
    sourceName: "SagerNet / sing-geosite",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-github.srs",
    entryCount: 82,
    updatedAt: "2026-11-26 14:17",
    badge: "代码托管",
    verified: true
  },
  {
    id: "p-geosite-ads",
    tag: "geosite-category-ads-all",
    format: "SRS Binary",
    sourceName: "SagerNet / sing-geosite",
    url: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs",
    entryCount: 1934,
    updatedAt: "2026-11-26 14:18",
    badge: "广告净化",
    verified: true
  }
];

export const RUNNING_APPS = [
  { process: "Cursor.exe", path: "C:\\Users\\admin\\AppData\\Local\\Programs\\cursor\\Cursor.exe", activeConns: 9 },
  { process: "chrome.exe", path: "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe", activeConns: 14 },
  { process: "Code.exe", path: "C:\\Users\\admin\\AppData\\Local\\Programs\\Microsoft VS Code\\Code.exe", activeConns: 6 },
  { process: "YuanShen.exe", path: "D:\\Games\\Genshin Impact\\Genshin Impact Game\\YuanShen.exe", activeConns: 2 },
  { process: "steam.exe", path: "C:\\Program Files (x86)\\Steam\\steam.exe", activeConns: 5 },
  { process: "WeChat.exe", path: "C:\\Program Files\\Tencent\\WeChat\\WeChat.exe", activeConns: 4 },
  { process: "Telegram.exe", path: "C:\\Users\\admin\\AppData\\Roaming\\Telegram Desktop\\Telegram.exe", activeConns: 3 }
];

export const INITIAL_CONNECTIONS = [
  {
    id: "c-101",
    process: "Cursor.exe",
    host: "api.anthropic.com",
    dstIp: "160.79.104.25",
    port: 443,
    network: "TCP",
    protocol: "TLS · HTTP/2",
    matchedRule: "FINAL → 默认出口",
    routeAction: "EXIT",
    dnsServer: "1.1.1.1 (DoH 经出口)",
    dnsMs: 41,
    rttMs: 142,
    downKBps: 420.5,
    upKBps: 38.2,
    totalBytes: "3.8 MB",
    startedAt: "10:56:12"
  },
  {
    id: "c-102",
    process: "chrome.exe",
    host: "github.com",
    dstIp: "140.82.114.4",
    port: 443,
    network: "UDP",
    protocol: "QUIC · HTTP/3",
    matchedRule: "FINAL → 默认出口",
    routeAction: "EXIT",
    dnsServer: "1.1.1.1 (DoH 经出口)",
    dnsMs: 36,
    rttMs: 138,
    downKBps: 1840.0,
    upKBps: 64.1,
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
    protocol: "Game KCP",
    matchedRule: "PROCESS-NAME · YuanShen.exe",
    routeAction: "DIRECT",
    dnsServer: "223.5.5.5 (直连)",
    dnsMs: 9,
    rttMs: 24,
    downKBps: 28.4,
    upKBps: 14.9,
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
    protocol: "TLS · HTTP/2",
    matchedRule: "RULE-SET · geosite-cn",
    routeAction: "DIRECT",
    dnsServer: "223.5.5.5 (直连)",
    dnsMs: 8,
    rttMs: 19,
    downKBps: 3120.8,
    upKBps: 42.0,
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
    protocol: "TLS · WebSocket",
    matchedRule: "DOMAIN-SUFFIX · chatgpt.com",
    routeAction: "EXIT",
    dnsServer: "1.1.1.1 (DoH 经出口)",
    dnsMs: 39,
    rttMs: 140,
    downKBps: 196.2,
    upKBps: 22.7,
    totalBytes: "2.1 MB",
    startedAt: "10:54:03"
  },
  {
    id: "c-106",
    process: "tailscaled.exe",
    host: "controlplane.tailscale.com",
    dstIp: "192.200.0.108",
    port: 443,
    network: "TCP",
    protocol: "Noise / HTTP2",
    matchedRule: "PROCESS-NAME · tailscaled.exe",
    routeAction: "DIRECT",
    dnsServer: "223.5.5.5 (直连)",
    dnsMs: 12,
    rttMs: 168,
    downKBps: 1.8,
    upKBps: 1.2,
    totalBytes: "412 KB",
    startedAt: "09:12:00"
  },
  {
    id: "c-107",
    process: "chrome.exe",
    host: "secure-glimr.doubleclick.net",
    dstIp: "0.0.0.0",
    port: 443,
    network: "TCP",
    protocol: "TLS",
    matchedRule: "RULE-SET · geosite-category-ads-all",
    routeAction: "REJECT",
    dnsServer: "已拦截",
    dnsMs: 0,
    rttMs: 0,
    downKBps: 0,
    upKBps: 0,
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
    protocol: "TLS · HTTP/2",
    matchedRule: "DOMAIN-SUFFIX · netflix.com",
    routeAction: "FALLBACK_PROXY",
    dnsServer: "外部代理远端解析",
    dnsMs: 28,
    rttMs: 56,
    downKBps: 2490.0,
    upKBps: 31.5,
    totalBytes: "42.0 MB",
    startedAt: "10:49:33"
  }
];

export const SAMPLE_IMPORT_RULES_TEXT = `# 支持一行一条规则（Clash / Surge / JSON 数组均支持）
DOMAIN-SUFFIX,anthropic.com,EXIT
DOMAIN-SUFFIX,steamcontent.com,DIRECT
PROCESS-NAME,steam.exe,DIRECT
IP-CIDR,47.96.0.0/11,DIRECT
DOMAIN-KEYWORD,adservice,REJECT`;
