// Package contract defines shared domain models and DTOs across all SingTail backend modules and the desktop frontend.
package contract

type RoutingMode string

const (
	ModeRule   RoutingMode = "rule"
	ModeGlobal RoutingMode = "global"
	ModeDirect RoutingMode = "direct"
)

type RuleAction string

const (
	ActionExit          RuleAction = "EXIT"
	ActionDirect        RuleAction = "DIRECT"
	ActionFallbackProxy RuleAction = "FALLBACK_PROXY"
	ActionReject        RuleAction = "REJECT"
)

type RuleType string

const (
	RuleDomainSuffix  RuleType = "DOMAIN-SUFFIX"
	RuleDomainKeyword RuleType = "DOMAIN-KEYWORD"
	RuleProcessName   RuleType = "PROCESS-NAME"
	RuleIPCIDR        RuleType = "IP-CIDR"
	RuleSet           RuleType = "RULE-SET"
	RuleFinal         RuleType = "FINAL"
)

type TrafficPeriodKey string

const (
	PeriodLive    TrafficPeriodKey = "live"
	PeriodDay     TrafficPeriodKey = "day"
	PeriodWeek    TrafficPeriodKey = "week"
	PeriodMonth30 TrafficPeriodKey = "month30"
	PeriodMonthly TrafficPeriodKey = "monthly"
)

// Organization represents an authenticated Tailscale tailnet mounted in SingTail.
type Organization struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Account          string `json:"account"`
	LocalTailscaleIP string `json:"localTailscaleIp"`
	NodeHostname     string `json:"nodeHostname,omitempty"`
	ControlURL       string `json:"controlUrl,omitempty"`
	StateDir         string `json:"stateDir,omitempty"`
	LastSync         string `json:"lastSync"`
}

// ExitNode represents a Tailscale peer offering exit node capability.
type ExitNode struct {
	ID          string `json:"id"`
	OrgID       string `json:"orgId"`
	OrgName     string `json:"orgName"`
	Hostname    string `json:"hostname"`
	Label       string `json:"label"`
	TailscaleIP string `json:"tailscaleIp"`
	PublicIP    string `json:"publicIp"`
	Region      string `json:"region"`
	RelayMode   string `json:"relayMode"`
	LatencyMs   int    `json:"latencyMs"`
	Online      bool   `json:"online"`
	Healthy     bool   `json:"healthy"`
}

// OrgLoginSession tracks an interactive tsnet OAuth login in progress.
type OrgLoginSession struct {
	SessionID string `json:"sessionId"`
	State     string `json:"state"` // "starting" | "auth_url" | "authenticated" | "error"
	AuthURL   string `json:"authUrl,omitempty"`
	Error     string `json:"error,omitempty"`
	OrgName   string `json:"orgName,omitempty"`
	NodeCount int    `json:"nodeCount,omitempty"`
}

// Rule represents an ordered traffic routing rule.
type Rule struct {
	ID      string     `json:"id"`
	Type    RuleType   `json:"type"`
	Payload string     `json:"payload"`
	Action  RuleAction `json:"action"`
	Hits    int64      `json:"hits"`
	Enabled bool       `json:"enabled"`
	Locked  bool       `json:"locked"`
}

// RuleProvider represents a binary .srs rule-set subscription.
type RuleProvider struct {
	ID         string `json:"id"`
	Tag        string `json:"tag"`
	SourceName string `json:"sourceName"`
	URL        string `json:"url"`
	EntryCount int    `json:"entryCount"`
	UpdatedAt  string `json:"updatedAt"`
}

// RunningApp represents an active OS process available for one-click rule creation.
type RunningApp struct {
	Process string `json:"process"`
	Path    string `json:"path"`
}

// LiveConnection represents an active network session observed from sing-box Clash API.
type LiveConnection struct {
	ID          string     `json:"id"`
	Process     string     `json:"process"`
	Host        string     `json:"host"`
	DstIP       string     `json:"dstIp"`
	Port        int        `json:"port"`
	Network     string     `json:"network"`
	MatchedRule string     `json:"matchedRule"`
	RouteAction RuleAction `json:"routeAction"`
	RttMs       int        `json:"rttMs"`
	DownKBps    float64    `json:"downKBps"`
	TotalBytes  string     `json:"totalBytes"`
	StartedAt   string     `json:"startedAt"`
}

// TrafficBreakdownItem represents traffic usage share for an exit node or organization.
type TrafficBreakdownItem struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	SubLabel  string  `json:"subLabel"`
	DownBytes int64   `json:"downBytes"`
	UpBytes   int64   `json:"upBytes"`
	TotalGB   float64 `json:"totalGB"`
	Percent   float64 `json:"percent"`
}

// TrafficReport holds time-series and org/node distribution metrics for a chosen granularity.
type TrafficReport struct {
	Period          TrafficPeriodKey       `json:"period"`
	Label           string                 `json:"label"`
	TotalDownGB     float64                `json:"totalDownGB"`
	TotalUpGB       float64                `json:"totalUpGB"`
	TotalGB         float64                `json:"totalGB"`
	Labels          []string               `json:"labels"`
	DownSeries      []float64              `json:"downSeries"`
	UpSeries        []float64              `json:"upSeries"`
	LiveDownHistory []float64              `json:"liveDownHistory"`
	LiveUpHistory   []float64              `json:"liveUpHistory"`
	ByNode          []TrafficBreakdownItem `json:"byNode"`
	ByOrg           []TrafficBreakdownItem `json:"byOrg"`
}

// BannerNotice surfaces active verification, runtime error, or rollback alerts in the UI.
type BannerNotice struct {
	Kind string `json:"kind"` // "verifying" | "rollback"
	Text string `json:"text"`
}

// SystemSettings stores user-configurable daemon and proxy forwarding preferences.
type SystemSettings struct {
	AutoStartService     bool   `json:"autoStartService"`
	AutoVerifyIP         bool   `json:"autoVerifyIp"`
	FallbackProxyEnabled bool   `json:"fallbackProxyEnabled"`
	FallbackProxyAddr    string `json:"fallbackProxyAddr"`
	AutoUpdateRulesDays  int    `json:"autoUpdateRulesDays"`
}

// DashboardSnapshot returns the full reactive state required by the desktop UI.
type DashboardSnapshot struct {
	TunEnabled      bool             `json:"tunEnabled"`
	Privileged      bool             `json:"privileged"`
	CoreAvailable   bool             `json:"coreAvailable"`
	CoreVersion     string           `json:"coreVersion,omitempty"`
	RoutingMode     RoutingMode      `json:"routingMode"`
	ActiveExitID    string           `json:"activeExitId"`
	VerifyingExitID string           `json:"verifyingExitId,omitempty"`
	BannerNotice    *BannerNotice    `json:"bannerNotice,omitempty"`
	Orgs            []Organization   `json:"orgs"`
	ExitNodes       []ExitNode       `json:"exitNodes"`
	Rules           []Rule           `json:"rules"`
	Providers       []RuleProvider   `json:"providers"`
	Connections     []LiveConnection `json:"connections"`
	RunningApps     []RunningApp     `json:"runningApps"`
	Settings        SystemSettings   `json:"settings"`
	Traffic         TrafficReport    `json:"traffic"`
}

// SanitizedRule represents a custom rule stripped of runtime counters for safe sharing.
type SanitizedRule struct {
	Type    RuleType   `json:"type"`
	Payload string     `json:"payload"`
	Action  RuleAction `json:"action"`
	Enabled bool       `json:"enabled"`
}

// SanitizedProvider represents a rule provider stripped of local metadata.
type SanitizedProvider struct {
	Tag        string `json:"tag"`
	SourceName string `json:"sourceName"`
	URL        string `json:"url"`
}

// SanitizedExport is guaranteed to exclude all ts-state private keys and auth tokens.
type SanitizedExport struct {
	Version     string              `json:"version"`
	ExportedAt  string              `json:"exportedAt"`
	CustomRules []SanitizedRule     `json:"customRules"`
	Providers   []SanitizedProvider `json:"providers"`
	Settings    SystemSettings      `json:"settings"`
}

// API Request DTOs

type SetTunRequest struct {
	Enabled bool `json:"enabled"`
}

type SetModeRequest struct {
	Mode RoutingMode `json:"mode"`
}

type SwitchExitRequest struct {
	ExitID string `json:"exitId"`
}

type AddRuleRequest struct {
	Type    RuleType   `json:"type"`
	Payload string     `json:"payload"`
	Action  RuleAction `json:"action"`
}

type MoveRuleRequest struct {
	RuleID    string `json:"ruleId"`
	Direction int    `json:"direction"` // -1 up, +1 down
}

type ToggleRuleRequest struct {
	RuleID string `json:"ruleId"`
}

type ImportRulesRequest struct {
	RawText string `json:"rawText"`
}

type QuickRouteRequest struct {
	ConnID    string     `json:"connId"`
	MatchKind string     `json:"matchKind"` // "domain" | "process"
	Action    RuleAction `json:"action"`
}

type AddProviderRequest struct {
	Tag        string `json:"tag"`
	SourceName string `json:"sourceName"`
	URL        string `json:"url"`
}

type StartOrgLoginRequest struct {
	Alias      string `json:"alias"`
	ControlURL string `json:"controlUrl,omitempty"`
}

type ImportOrgStateDirRequest struct {
	SourceDir string `json:"sourceDir"`
	Alias     string `json:"alias,omitempty"`
}

// AddOrgRequest is retained for manual/headless org registration in automated tests.
type AddOrgRequest struct {
	Name         string `json:"name"`
	Account      string `json:"account"`
	NodeHostname string `json:"nodeHostname"`
	PublicIP     string `json:"publicIp"`
	Region       string `json:"region"`
}
