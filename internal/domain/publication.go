package domain

// 发布请求只包含用户在表单中选择的内容，不保存任何凭据。
type PublishRequest struct {
	Name     string `json:"name"`
	Parent   string `json:"parent"`
	Path     string `json:"path,omitempty"`
	Existing bool   `json:"existing"`
	Private  bool   `json:"private"`
}

// 每个有副作用的阶段之后持久化进度，重试不重复提交或创建仓库。
type PublishState struct {
	Request         PublishRequest `json:"request"`
	Owner           string         `json:"owner"`
	Path            string         `json:"path"`
	Branch          string         `json:"branch,omitempty"`
	Commit          string         `json:"commit,omitempty"`
	LocalReady      bool           `json:"localReady,omitempty"`
	Committed       bool           `json:"committed,omitempty"`
	RemoteAttempted bool           `json:"remoteAttempted,omitempty"`
	RemoteID        int64          `json:"remoteId,omitempty"`
	RemoteURL       string         `json:"remoteUrl,omitempty"`
	Published       bool           `json:"published,omitempty"`
}
