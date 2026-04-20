package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the Auralens HTTP API client.
type Client struct {
	baseURL    string
	agentName  string
	agentToken string
	http       *http.Client
}

// New creates an authenticated API client.
func New(baseURL, agentName, agentToken string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		agentName:  agentName,
		agentToken: agentToken,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

// NewAnon creates an unauthenticated client (for registration).
func NewAnon(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// ── Auth ────────────────────────────────────────────────────────────────────

type RegisterRequest struct {
	Name       string `json:"name"`
	InviteCode string `json:"inviteCode"`
}

type RegisterResponse struct {
	Success bool `json:"success"`
	Agent   struct {
		ID   string `json:"_id"`
		Name string `json:"name"`
		Kind string `json:"kind"`
		Role string `json:"role"`
	} `json:"agent"`
	Token   string `json:"token"`
	Message string `json:"message"`
}

// Register calls POST /api/agents/register with an invite code.
func (c *Client) Register(name, inviteCode string) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := c.doJSON("POST", "/api/agents/register", RegisterRequest{Name: name, InviteCode: inviteCode}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── Research ────────────────────────────────────────────────────────────────

// ResearchItem is a summary item as returned by the list endpoint.
type ResearchItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	RoundCount  int    `json:"roundCount"`
	CurrentRound *struct {
		Status      string `json:"status"`
		RoundNumber int    `json:"roundNumber"`
		Notes       string `json:"notes"`
	} `json:"currentRound"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Attachment is a file in the research pool or a round's outputs.
type Attachment struct {
	ID          string `json:"id"`
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	ContentType string `json:"contentType"`
	SignedURL   string `json:"signedUrl"`
	DisplayURL  string `json:"displayUrl"`
}

// Round is a single research round.
type Round struct {
	ID            string       `json:"id"`
	RoundNumber   int          `json:"roundNumber"`
	Status        string       `json:"status"`
	Notes         string       `json:"notes"`
	AttachmentIDs []string     `json:"attachmentIds"`
	Attachments   []Attachment `json:"attachments"`
	Outputs       []Attachment `json:"outputs"`
	Result        string       `json:"result"`
}

// ResearchDetail is the full research document returned by the detail endpoint.
type ResearchDetail struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	Status       string       `json:"status"`
	Notes        string       `json:"notes"`
	Attachments  []Attachment `json:"attachments"`
	Rounds       []Round      `json:"rounds"`
	CurrentRound *Round       `json:"currentRound"`
	CreatedAt    string       `json:"createdAt"`
	UpdatedAt    string       `json:"updatedAt"`
}

type listResearchResponse struct {
	Success    bool           `json:"success"`
	Data       []ResearchItem `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		Total      int `json:"total"`
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
}

// ListResearchParams configures the research list query.
type ListResearchParams struct {
	Status    string // draft | active | archived
	HasResult string // true | false | ""
	Page      int
	PageSize  int
}

// ListResearch fetches paginated research items.
func (c *Client) ListResearch(p ListResearchParams) ([]ResearchItem, int, error) {
	q := url.Values{}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.HasResult != "" {
		q.Set("hasResult", p.HasResult)
	}
	if p.Page > 0 {
		q.Set("page", fmt.Sprintf("%d", p.Page))
	}
	if p.PageSize > 0 {
		q.Set("pageSize", fmt.Sprintf("%d", p.PageSize))
	}

	path := "/api/research"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	var resp listResearchResponse
	if err := c.doJSON("GET", path, nil, &resp); err != nil {
		return nil, 0, err
	}
	return resp.Data, resp.Pagination.Total, nil
}

type getResearchResponse struct {
	Success bool           `json:"success"`
	Data    ResearchDetail `json:"data"`
}

// GetResearch fetches the full detail of a single research item.
func (c *Client) GetResearch(id string) (*ResearchDetail, error) {
	var resp getResearchResponse
	if err := c.doJSON("GET", "/api/research/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

type submitResultRequest struct {
	Content string `json:"content,omitempty"`
}

type submitResultResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		CurrentRound *Round `json:"currentRound"`
	} `json:"data"`
}

// SubmitResult posts a result and archives the current round.
// content is optional (may be empty).
func (c *Client) SubmitResult(id, content string) (*submitResultResponse, error) {
	var resp submitResultResponse
	if err := c.doJSON("POST", "/api/research/"+id+"/result", submitResultRequest{Content: content}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── internal helpers ────────────────────────────────────────────────────────

// doJSON performs a JSON request. body may be nil for GET/DELETE.
func (c *Client) doJSON(method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.agentName != "" {
		req.Header.Set("X-Agent-Name", c.agentName)
	}
	if c.agentToken != "" {
		req.Header.Set("X-Agent-Token", c.agentToken)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Try to extract a message field from the JSON error body.
		var errBody struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &errBody)
		msg := errBody.Error
		if msg == "" {
			msg = errBody.Message
		}
		if msg == "" {
			msg = string(respBody)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
