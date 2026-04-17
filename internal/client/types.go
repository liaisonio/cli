package client

import (
	"fmt"
	"strconv"
)

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

// FlexUint is a uint64 that accepts either a JSON number or a JSON string.
//
// The liaison-cloud REST API is code-generated from Kratos/proto, and proto
// uint64 fields are serialized as JSON STRINGS (e.g. `"id": "100044"`) to
// avoid losing precision in JS clients. Go's standard uint64 unmarshalling
// only accepts a bare number, so we need a thin wrapper that tolerates both.
//
// Marshalling emits a bare number — we only consume these types, we don't
// post them back, so string-encoding on the outbound side isn't required.
type FlexUint uint64

func (f *FlexUint) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	// Strip surrounding quotes if the server sent a string.
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" {
		return nil
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("parse FlexUint %q: %w", s, err)
	}
	*f = FlexUint(n)
	return nil
}

func (f FlexUint) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatUint(uint64(f), 10)), nil
}

func (f FlexUint) Uint64() uint64 { return uint64(f) }

// ─── IAM ─────────────────────────────────────────────────────────────────────

type CurrentUser struct {
	ID          FlexUint `json:"id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Phone       string   `json:"phone,omitempty"`
	DisplayName string   `json:"display_name,omitempty"`
	Avatar      string   `json:"avatar,omitempty"`
}

// ─── Edge (Connector) ────────────────────────────────────────────────────────

type Edge struct {
	ID               FlexUint `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Status           int      `json:"status"` // 1=running, 2=stopped
	Online           int      `json:"online"` // 1=online,  2=offline
	CreatedAt        string   `json:"created_at,omitempty"`
	UpdatedAt        string   `json:"updated_at,omitempty"`
	ApplicationCount int      `json:"application_count,omitempty"`
	Device           any      `json:"device,omitempty"`
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
	ID          FlexUint `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Protocol    string   `json:"protocol,omitempty"`
	Port        int      `json:"port,omitempty"`
	Domain      string   `json:"domain,omitempty"`
	Status      string   `json:"status,omitempty"` // "running" or "stopped"
	EdgeID      FlexUint `json:"edge_id,omitempty"`
	AppID       FlexUint `json:"application_id,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

type ProxyList struct {
	Total   int32    `json:"total"`
	Proxies []*Proxy `json:"proxies"`
}

// ─── Application ─────────────────────────────────────────────────────────────

type Application struct {
	ID              FlexUint `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	ApplicationType string   `json:"application_type,omitempty"`
	IP              string   `json:"ip,omitempty"`
	Port            int      `json:"port,omitempty"`
	EdgeID          FlexUint `json:"edge_id,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
}

type ApplicationList struct {
	Total        int32          `json:"total"`
	Applications []*Application `json:"applications"`
}

// ─── Device ──────────────────────────────────────────────────────────────────

type Device struct {
	ID        FlexUint `json:"id"`
	Name      string   `json:"name"`
	OS        string   `json:"os,omitempty"`
	Arch      string   `json:"arch,omitempty"`
	Online    int      `json:"online,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type DeviceList struct {
	Total   int32     `json:"total"`
	Devices []*Device `json:"devices"`
}
