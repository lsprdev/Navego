package browser

import (
	"context"
	"time"
)

type Element struct {
	Ref       string `json:"ref"`
	Role      string `json:"role"`
	Name      string `json:"name,omitempty"`
	Value     string `json:"value,omitempty"`
	Secret    bool   `json:"secret,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type Snapshot struct {
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Text       string    `json:"text,omitempty"`
	Elements   []Element `json:"elements"`
	Generation uint64    `json:"generation"`
	Backend    string    `json:"backend,omitempty"`
	Metadata   Metadata  `json:"metadata,omitempty"`
}

type Metadata struct {
	Description    string `json:"description,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	ImageAlt       string `json:"image_alt,omitempty"`
	SiteName       string `json:"site_name,omitempty"`
	Type           string `json:"type,omitempty"`
	ArticleSection string `json:"article_section,omitempty"`
}

func (m Metadata) Empty() bool {
	return m == (Metadata{})
}

type FindMatch struct {
	Text string `json:"text"`
	Ref  string `json:"ref,omitempty"`
	Href string `json:"href,omitempty"`
}

type PageLink struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

type FindResult struct {
	Query      string      `json:"query"`
	URL        string      `json:"url"`
	Title      string      `json:"title,omitempty"`
	Backend    string      `json:"backend"`
	Generation uint64      `json:"generation"`
	Matches    []FindMatch `json:"matches"`
	Truncated  bool        `json:"truncated,omitempty"`
}

type WaitCondition struct {
	Text        string
	URLContains string
	Timeout     time.Duration
}

type Tab struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title,omitempty"`
	Active bool   `json:"active"`
}

type TabsResult struct {
	Tabs []Tab `json:"tabs"`
}

type Finder interface {
	Find(context.Context, string, int) (FindResult, error)
}

type Waiter interface {
	Wait(context.Context, WaitCondition) (Snapshot, error)
}

type TabController interface {
	ListTabs(context.Context) (TabsResult, error)
	NewTab(context.Context, string) (Snapshot, error)
	SwitchTab(context.Context, string) (Snapshot, error)
	CloseTab(context.Context, string) (Snapshot, error)
}

type RoutingStatus struct {
	PublicBackend       string     `json:"public_backend"`
	Mode                string     `json:"mode"`
	PinnedChromiumHosts int        `json:"pinned_chromium_hosts,omitempty"`
	CircuitState        string     `json:"circuit_state"`
	ConsecutiveFailure  int        `json:"consecutive_failures"`
	RetryAt             *time.Time `json:"retry_at,omitempty"`
}

type Status struct {
	Connected bool           `json:"connected"`
	URL       string         `json:"url,omitempty"`
	Title     string         `json:"title,omitempty"`
	Backend   string         `json:"backend,omitempty"`
	Routing   *RoutingStatus `json:"routing,omitempty"`
}

type ActionTarget struct {
	Ref        string            `json:"ref"`
	Role       string            `json:"role"`
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Generation uint64            `json:"generation"`
	Fields     map[string]string `json:"fields,omitempty"`
	FieldRefs  map[string]string `json:"-"`
}

type Controller interface {
	Status(context.Context) (Status, error)
	Open(context.Context, string) (Snapshot, error)
	Snapshot(context.Context) (Snapshot, error)
	Click(context.Context, string) (Snapshot, error)
	Type(context.Context, string, string, bool) (Snapshot, error)
	Screenshot(context.Context, bool) ([]byte, string, error)
	PDF(context.Context) ([]byte, string, error)
	DescribeAction(context.Context, string) (ActionTarget, error)
	CommitAction(context.Context, ActionTarget) (Snapshot, error)
	Close() error
}
