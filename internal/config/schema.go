package config

type SysConfig struct {
	Agents    AgentConfig      `json:"agents"`
	Embedding EmbeddingConfig  `json:"embedding"`
	Providers []ProviderConfig `json:"providers"`
	Channels  ChannelsConfig   `json:"channels"`
	Gateway   GatewayConfig    `json:"gateway"`
	Tools     []ToolConfig     `json:"tools"`
	MCP       MCPConfig        `json:"mcp"`
	Cron      CronConfig       `json:"cron"`
}

type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

type MCPServerConfig struct {
	Enabled              bool              `json:"enabled"`
	Command              string            `json:"command"`
	Args                 []string          `json:"args,omitempty"`
	Env                  map[string]string `json:"env,omitempty"`
	Cwd                  string            `json:"cwd,omitempty"`
	URL                  string            `json:"url"`
	Headers              map[string]string `json:"headers,omitempty"`
	Timeout              int               `json:"timeout,omitempty"`
	KeepAlive            int               `json:"keepAlive,omitempty"`
	DisableStandaloneSSE bool              `json:"disableStandaloneSSE,omitempty"`
}

type ChannelsConfig struct {
	CLI           CLIChannelConfig    `json:"cli"`
	Feishu        FeishuChannelConfig `json:"feishu"`
	SendProgress  bool                `json:"sendProgress"`
	SendToolHints bool                `json:"sendToolHints"`
}

type ChannelConfig struct {
	Enabled bool `json:"enabled"`
}

type CLIChannelConfig struct {
	ChannelConfig
}

type FeishuChannelConfig struct {
	ChannelConfig
	AppID             string   `json:"appId"`
	AppSecret         string   `json:"appSecret"`
	EncryptKey        string   `json:"encryptKey"`
	VerificationToken string   `json:"verificationToken"`
	AllowFrom         []string `json:"allowFrom"`
	ReactEmoji        string   `json:"reactEmoji"`
}

type AgentConfig struct {
	Profiles map[string]ProfileConfig `json:"profiles"`
}

type EmbeddingConfig struct {
	Profiles  map[string]EmbeddingProfileConfig `json:"profiles"`
	Providers []ProviderConfig                  `json:"providers"`
}

type EmbeddingProfileConfig struct {
	Text  EmbeddingModelConfig `json:"text"`
	Modal EmbeddingModelConfig `json:"modal"`
}

type EmbeddingModelConfig struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	OutputDimension int    `json:"outputDimension"`
}

type ProfileConfig struct {
	Workspace         string  `json:"workspace"`
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	MaxTokens         int     `json:"maxTokens"`
	Temperature       float64 `json:"temperature"`
	MaxToolIterations int     `json:"maxToolIterations"`
	MemoryWindow      int     `json:"memoryWindow"`
	MaxRetryTimes     int     `json:"maxRetryTimes"`
}

type ProviderConfig struct {
	Name    string     `json:"name"`
	Timeout int        `json:"timeout"`
	BaseURL string     `json:"baseURL"`
	Path    string     `json:"path"`
	Auth    AuthConfig `json:"auth"`
}

type AuthConfig struct {
	Token string `json:"token"`
}

type GatewayConfig struct {
	Port      int             `json:"port"`
	Host      string          `json:"host"`
	Heartbeat HeartbeatConfig `json:"heartbeat"`
}
type HeartbeatConfig struct {
	Interval int  `json:"interval"` // seconds
	Enable   bool `json:"enable"`
}

type ToolConfig struct {
	Name    string `json:"name"`
	Timeout int    `json:"timeout"`
}

type CronConfig struct {
	Enabled  bool   `json:"enabled"`
	Timezone string `json:"timezone"`
}

func CreateDefaultConfig() SysConfig {
	return SysConfig{
		Agents: AgentConfig{
			Profiles: map[string]ProfileConfig{
				"default": {
					Workspace:         "",
					Provider:          "",
					Model:             "",
					MaxTokens:         8192,
					Temperature:       0.1,
					MaxToolIterations: 40,
					MemoryWindow:      30,
					MaxRetryTimes:     3,
				},
			},
		},
		Embedding: EmbeddingConfig{
			Profiles: map[string]EmbeddingProfileConfig{
				"default": {
					Text:  EmbeddingModelConfig{},
					Modal: EmbeddingModelConfig{},
				},
			},
			Providers: []ProviderConfig{
				{
					Name:    "voyageai",
					Timeout: 60,
					Auth: AuthConfig{
						Token: "",
					},
				},
			},
		},
		Providers: []ProviderConfig{
			{
				Name:    "openrouter",
				Timeout: 60,
				Auth: AuthConfig{
					Token: "",
				},
			},
			{
				Name:    "codex",
				Timeout: 60,
				Auth: AuthConfig{
					Token: "",
				},
			},
		},
		Channels: ChannelsConfig{
			CLI: CLIChannelConfig{
				ChannelConfig: ChannelConfig{Enabled: true},
			},
			Feishu: FeishuChannelConfig{
				AllowFrom:  []string{"*"},
				ReactEmoji: "THUMBSUP",
			},
			SendProgress:  true,
			SendToolHints: true,
		},
		Gateway: GatewayConfig{
			Port: 8080,
			Host: "127.0.0.1",
			Heartbeat: HeartbeatConfig{
				Enable:   true,
				Interval: 1800,
			},
		},
		Tools: []ToolConfig{},
		MCP: MCPConfig{
			MCPServers: map[string]MCPServerConfig{},
		},
		Cron: CronConfig{
			Enabled:  true,
			Timezone: "Europe/London",
		},
	}
}
