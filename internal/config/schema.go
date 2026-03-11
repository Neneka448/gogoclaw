package config

type SysConfig struct {
	Agents    AgentConfig       `json:"agents"`
	Providers []ProviderConfig  `json:"providers"`
	Channels  map[string]string `json:"channels"`
	Gateway   GatewayConfig     `json:"gateway"`
	Tools     []ToolConfig      `json:"tools"`
}

type AgentConfig struct {
	Profiles map[string]ProfileConfig `json:"profiles"`
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
		Channels: map[string]string{},
		Gateway: GatewayConfig{
			Port: 8080,
			Host: "127.0.0.1",
			Heartbeat: HeartbeatConfig{
				Enable:   true,
				Interval: 1800,
			},
		},
		Tools: []ToolConfig{},
	}
}
