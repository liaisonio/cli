package client

// APIResponse is the envelope liaison-cloud wraps every /api/v1 response in.
// Data is left as a raw []byte so callers can decode into concrete types.
type APIResponse struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Reason   string `json:"reason,omitempty"`
	Metadata any    `json:"metadata,omitempty"`
	// Data is populated by the client after unmarshalling the envelope.
	Data []byte `json:"-"`
}

// ─── IAM ─────────────────────────────────────────────────────────────────────

type CurrentUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	Phone       string `json:"phone,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

// ─── Edge (Connector) ────────────────────────────────────────────────────────

type Edge struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Status           int    `json:"status"` // 1=running, 2=stopped
	Online           int    `json:"online"` // 1=online,  2=offline
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
	ApplicationCount int    `json:"application_count,omitempty"`
	Device           any    `json:"device,omitempty"`
}

type EdgeList struct {
	Total int32   `json:"total"`
	Edges []*Edge `json:"edges"`
}

type EdgeCreateResult struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Command   string `json:"command,omitempty"`
}

// ─── Proxy (Entry) ───────────────────────────────────────────────────────────

type Proxy struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	Port        int    `json:"port,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Status      string `json:"status,omitempty"` // "running" or "stopped"
	EdgeID      uint64 `json:"edge_id,omitempty"`
	AppID       uint64 `json:"application_id,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type ProxyList struct {
	Total   int32    `json:"total"`
	Proxies []*Proxy `json:"proxies"`
}

// ─── Application ─────────────────────────────────────────────────────────────

type Application struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	IP          string `json:"ip,omitempty"`
	Port        int    `json:"port,omitempty"`
	EdgeID      uint64 `json:"edge_id,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type ApplicationList struct {
	Total        int32          `json:"total"`
	Applications []*Application `json:"applications"`
}

// ─── Device ──────────────────────────────────────────────────────────────────

type Device struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	OS        string `json:"os,omitempty"`
	Arch      string `json:"arch,omitempty"`
	Online    int    `json:"online,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type DeviceList struct {
	Total   int32     `json:"total"`
	Devices []*Device `json:"devices"`
}
