package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/jobs"
)

// AlertInput describes an outgoing alert triggered by Cron, job lifecycle, or hardware watchers.
type AlertInput struct {
	Source   string
	Severity string
	Title    string
	Message  string
	Priority string // ntfy priority: "default" | "high"
	ConvID   string
	JobID    string
	SendNtfy *bool // nil = follow config / topic availability
	SendPush *bool // nil = follow config
}

// DispatchResult reports alert persistence and delivery outcomes per channel.
type DispatchResult struct {
	Event           *db.AlertEvent `json:"event"`
	NtfySent        bool           `json:"ntfy_sent"`
	NtfyError       string         `json:"ntfy_error,omitempty"`
	PushSent        int            `json:"push_sent"`
	PushFailed      int            `json:"push_failed"`
	PushError       string         `json:"push_error,omitempty"`
	DesktopSent     bool           `json:"desktop_sent"`
	DesktopError    string         `json:"desktop_error,omitempty"`
	BrowserLiveSent int            `json:"browser_live_sent"`
}

// Notifier coordinates persisted Gateway alert history, live browser SSE, OS notifications, ntfy, and Web Push.
type Notifier struct {
	store           *db.GatewayDB
	httpClient      *http.Client
	desktopNotifyFn func(ctx context.Context, title, message, severity string) (bool, error)

	subMu sync.Mutex
	subs  map[chan db.AlertEvent]struct{}
}

// NewNotifier creates a unified alert Notifier backed by GatewayDB.
func NewNotifier(store *db.GatewayDB) *Notifier {
	return &Notifier{
		store:           store,
		httpClient:      &http.Client{Timeout: 6 * time.Second},
		desktopNotifyFn: SendOSNotification,
		subs:            map[chan db.AlertEvent]struct{}{},
	}
}

// Subscribe registers a live browser SSE listener channel and returns its unsubscribe cleanup.
func (n *Notifier) Subscribe() (<-chan db.AlertEvent, func()) {
	ch := make(chan db.AlertEvent, 16)
	n.subMu.Lock()
	n.subs[ch] = struct{}{}
	n.subMu.Unlock()

	return ch, func() {
		n.subMu.Lock()
		if _, ok := n.subs[ch]; ok {
			delete(n.subs, ch)
			close(ch)
		}
		n.subMu.Unlock()
	}
}

func (n *Notifier) publishLive(ev db.AlertEvent) int {
	n.subMu.Lock()
	defer n.subMu.Unlock()

	delivered := 0
	for ch := range n.subs {
		select {
		case ch <- ev:
			delivered++
		default:
		}
	}
	return delivered
}

// Dispatch stores an alert in SQLite and delivers it across live browser tabs, host OS, ntfy, and Web Push.
func (n *Notifier) Dispatch(ctx context.Context, in AlertInput) DispatchResult {
	res := DispatchResult{}
	if in.Severity == "" {
		in.Severity = "info"
	}
	if in.Priority == "" {
		if in.Severity == "error" || in.Severity == "warning" {
			in.Priority = "high"
		} else {
			in.Priority = "default"
		}
	}

	ev, _ := n.store.InsertAlertEvent(db.AlertEvent{
		Source:   in.Source,
		Severity: in.Severity,
		Title:    in.Title,
		Message:  in.Message,
		ConvID:   in.ConvID,
		JobID:    in.JobID,
	})
	res.Event = ev
	if ev != nil {
		res.BrowserLiveSent = n.publishLive(*ev)
	}

	cfg, _ := n.store.GetAlertConfig()
	topic := strings.TrimSpace(cfg.NtfyTopic)
	if topic == "" {
		topic = strings.TrimSpace(os.Getenv("NTFY_TOPIC"))
	}
	ntfyURL := strings.TrimRight(cfg.NtfyURL, "/")
	if ntfyURL == "" {
		ntfyURL = strings.TrimRight(os.Getenv("NTFY_URL"), "/")
	}
	if ntfyURL == "" {
		ntfyURL = "https://ntfy.sh"
	}

	allowNtfy := topic != "" && (in.SendNtfy == nil || *in.SendNtfy)
	if allowNtfy {
		if err := n.postNtfy(ctx, ntfyURL, topic, in.Title, in.Message, in.ConvID, in.Priority); err != nil {
			res.NtfyError = err.Error()
		} else {
			res.NtfySent = true
		}
	}

	allowPush := cfg.WebPushEnabled && (in.SendPush == nil || *in.SendPush)
	if allowPush {
		tag := fmt.Sprintf("agy-%s", in.Source)
		if ev != nil && ev.ID > 0 {
			tag = fmt.Sprintf("agy-alert-%d", ev.ID)
		}
		sent, failed, pushErr := n.broadcastWebPush(ctx, in, tag)
		res.PushSent = sent
		res.PushFailed = failed
		res.PushError = pushErr
	}

	if cfg.NotifyDesktop && n.desktopNotifyFn != nil {
		if ok, err := n.desktopNotifyFn(ctx, in.Title, in.Message, in.Severity); err != nil {
			res.DesktopError = err.Error()
			log.Printf("[alerts] desktop notify failed: %v", err)
		} else if ok {
			res.DesktopSent = true
		}
	}
	return res
}

func (n *Notifier) postNtfy(ctx context.Context, baseURL, topic, title, message, convID, priority string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s", baseURL, topic), strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
	if pub := strings.TrimRight(os.Getenv("PUBLIC_URL"), "/"); pub != "" && convID != "" {
		req.Header.Set("Click", fmt.Sprintf("%s/?conv=%s", pub, convID))
	}
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy HTTP %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) broadcastWebPush(ctx context.Context, in AlertInput, tag string) (sent, failed int, firstErr string) {
	subsRaw, err := n.store.ListPushSubscriptions()
	if err != nil || len(subsRaw) == 0 {
		return 0, 0, ""
	}
	keys, err := EnsureVAPIDKeys(n.store)
	if err != nil {
		return 0, len(subsRaw), err.Error()
	}
	targetURL := "/"
	if in.ConvID != "" {
		targetURL = "/?conv=" + in.ConvID
	}
	payload, _ := json.Marshal(map[string]any{
		"title":    in.Title,
		"body":     in.Message,
		"tag":      tag,
		"url":      targetURL,
		"severity": in.Severity,
	})

	for _, raw := range subsRaw {
		var sub BrowserPushSubscription
		if json.Unmarshal([]byte(raw), &sub) != nil || sub.Endpoint == "" {
			failed++
			continue
		}
		if err := SendWebPush(ctx, n.httpClient, sub, keys, payload); err != nil {
			if errors.Is(err, ErrSubscriptionGone) {
				_ = n.store.DeletePushSubscription(sub.Endpoint)
			}
			if firstErr == "" {
				firstErr = err.Error()
			}
			log.Printf("[alerts] webpush failed (%s): %v", sub.Endpoint, err)
			failed++
			continue
		}
		sent++
	}
	return sent, failed, firstErr
}

// HandleJobFinish bridges completed or failed Gateway jobs into active notifications.
func (n *Notifier) HandleJobFinish(notice jobs.JobFinishNotice) {
	cfg, _ := n.store.GetAlertConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	elapsedSec := int(notice.Elapsed.Seconds())
	if notice.Status == "error" {
		n.Dispatch(ctx, AlertInput{
			Source:   "job",
			Severity: "error",
			Priority: "high",
			Title:    "AGY: Erro na tarefa",
			Message:  notice.Summary,
			ConvID:   notice.ConversationID,
			JobID:    notice.JobID,
		})
		return
	}

	if !cfg.NotifyJobDone {
		return
	}
	if cfg.NotifyLongJobSec > 0 && elapsedSec < cfg.NotifyLongJobSec {
		return
	}

	msg := fmt.Sprintf("Sessão concluída em %ds (%s).", elapsedSec, notice.Model)
	if notice.Summary != "" {
		msg = fmt.Sprintf("[%ds · %s] %s", elapsedSec, notice.Model, notice.Summary)
	}
	n.Dispatch(ctx, AlertInput{
		Source:   "job",
		Severity: "ok",
		Priority: "default",
		Title:    "AGY: Tarefa longa concluída",
		Message:  msg,
		ConvID:   notice.ConversationID,
		JobID:    notice.JobID,
	})
}
