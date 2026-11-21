package botbridge

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/db"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/jobs"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/runner"
	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/wol"
)

// TelegramUpdate models the subset of Telegram Bot API Update payloads handled by the gateway.
type TelegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

// TelegramMessage models an incoming Telegram message (text, photo, document, voice, audio).
type TelegramMessage struct {
	MessageID int64              `json:"message_id"`
	From      *TelegramUser      `json:"from,omitempty"`
	Chat      TelegramChat       `json:"chat"`
	Text      string             `json:"text,omitempty"`
	Caption   string             `json:"caption,omitempty"`
	Photo     []TelegramPhoto    `json:"photo,omitempty"`
	Document  *TelegramDocument  `json:"document,omitempty"`
	Voice     *TelegramAudioFile `json:"voice,omitempty"`
	Audio     *TelegramAudioFile `json:"audio,omitempty"`
}

// TelegramUser holds Telegram sender identity.
type TelegramUser struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username,omitempty"`
}

// TelegramChat holds chat metadata.
type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// TelegramPhoto represents one resolution variant of a Telegram photo.
type TelegramPhoto struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size,omitempty"`
}

// TelegramDocument represents a generic document uploaded to Telegram.
type TelegramDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
}

// TelegramAudioFile represents a Telegram voice note (.ogg) or audio file.
type TelegramAudioFile struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type,omitempty"`
}

// TelegramBot connects Telegram Bot API (long-polling or webhook) directly to jobs.Manager, Runner, and Outbox.
type TelegramBot struct {
	Cfg     config.GatewaySettings
	Store   *db.GatewayDB
	Runner  runner.Runner
	Mgr     *jobs.Manager
	Bridge  *Bridge
	HTTP    *http.Client
	APIBase string
}

// NewTelegramBot creates a TelegramBot connector. APIBase defaults to https://api.telegram.org.
func NewTelegramBot(cfg config.GatewaySettings, store *db.GatewayDB, r runner.Runner, mgr *jobs.Manager, bridge *Bridge) *TelegramBot {
	return &TelegramBot{
		Cfg:     cfg,
		Store:   store,
		Runner:  r,
		Mgr:     mgr,
		Bridge:  bridge,
		HTTP:    &http.Client{Timeout: 45 * time.Second},
		APIBase: "https://api.telegram.org",
	}
}

// VerifyWebhookSecret checks X-Telegram-Bot-Api-Secret-Token when configured.
func (t *TelegramBot) VerifyWebhookSecret(got string) bool {
	expected := strings.TrimSpace(t.Cfg.TelegramWebhookSecret)
	if expected == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(expected)) == 1
}

// IsAllowedChat enforces private/allowlisted chat_id or user_id matching.
func (t *TelegramBot) IsAllowedChat(chatID int64, userID int64) bool {
	if len(t.Cfg.TelegramAllowedChatIDs) == 0 {
		return false
	}
	chatStr := strconv.FormatInt(chatID, 10)
	userStr := strconv.FormatInt(userID, 10)
	for _, allowed := range t.Cfg.TelegramAllowedChatIDs {
		clean := strings.TrimSpace(allowed)
		if clean == chatStr || (userID != 0 && clean == userStr) {
			return true
		}
	}
	return false
}

func (t *TelegramBot) apiURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", strings.TrimRight(t.APIBase, "/"), t.Cfg.TelegramBotToken, method)
}

func (t *TelegramBot) fileURL(filePath string) string {
	return fmt.Sprintf("%s/file/bot%s/%s", strings.TrimRight(t.APIBase, "/"), t.Cfg.TelegramBotToken, strings.TrimLeft(filePath, "/"))
}

func (t *TelegramBot) sendChatAction(ctx context.Context, chatID int64, action string) {
	payload, _ := json.Marshal(map[string]any{"chat_id": chatID, "action": action})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiURL("sendChatAction"), bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

// SendText sends a potentially long reply split into 4000-rune Telegram chunks.
func (t *TelegramBot) SendText(ctx context.Context, chatID int64, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, chunk := range SplitChunks(text, 4000) {
		payload, _ := json.Marshal(map[string]any{
			"chat_id": chatID,
			"text":    chunk,
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiURL("sendMessage"), bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := t.HTTP.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			return fmt.Errorf("telegram sendMessage %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		_ = resp.Body.Close()
	}
	return nil
}

func isTelegramImage(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}

// SendOutboxFile uploads an Outbox file directly to Telegram via sendPhoto or sendDocument.
func (t *TelegramBot) SendOutboxFile(ctx context.Context, chatID int64, filename string) error {
	rc, closeFn, err := t.Runner.OpenOutboxFile(ctx, filename)
	if err != nil {
		return err
	}
	defer func() { _ = closeFn() }()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	method := "sendDocument"
	field := "document"
	if isTelegramImage(filename) {
		method = "sendPhoto"
		field = "photo"
	}
	fw, err := mw.CreateFormFile(field, filepath.Base(filename))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, io.LimitReader(rc, 45<<20)); err != nil {
		return err
	}
	_ = mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiURL(method), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram %s %d: %s", method, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (t *TelegramBot) downloadFileToTemp(ctx context.Context, fileID, fallbackName string) (tmpPath string, cleanName string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.apiURL("getFile")+"?file_id="+fileID, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var gf struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gf); err != nil || !gf.OK || gf.Result.FilePath == "" {
		return "", "", fmt.Errorf("telegram getFile falhou para %s", fileID)
	}

	base := filepath.Base(gf.Result.FilePath)
	if fallbackName != "" {
		base = filepath.Base(fallbackName)
	}
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, base)
	if base == "" || base == "." {
		base = "telegram_file.bin"
	}

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, t.fileURL(gf.Result.FilePath), nil)
	if err != nil {
		return "", "", err
	}
	dlResp, err := t.HTTP.Do(dlReq)
	if err != nil {
		return "", "", err
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode >= 400 {
		return "", "", fmt.Errorf("download telegram HTTP %d", dlResp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "tg-*_"+base)
	if err != nil {
		return "", "", err
	}
	tmpPath = tmpFile.Name()
	if _, err := io.Copy(tmpFile, io.LimitReader(dlResp.Body, 45<<20)); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	_ = tmpFile.Close()
	return tmpPath, fmt.Sprintf("tg_%s_%s", time.Now().Format("20060102-150405"), base), nil
}

func (t *TelegramBot) transcribeAudioFile(ctx context.Context, audioPath, fileName string) (string, error) {
	if enabled, _ := t.Cfg.Mods["whisper"].(bool); !enabled || t.Cfg.WhisperURL == "" {
		return "", fmt.Errorf("whisper desativado")
	}
	f, err := os.Open(audioPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", err
	}
	_ = mw.WriteField("model", "Systran/faster-whisper-small")
	_ = mw.Close()

	endpoint := strings.TrimRight(t.Cfg.WhisperURL, "/") + "/v1/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("whisper HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	return strings.TrimSpace(parsed.Text), nil
}

// HandleUpdate processes one Telegram Update (text, photo, document, audio/voice, slash commands).
func (t *TelegramBot) HandleUpdate(ctx context.Context, upd TelegramUpdate) (accepted bool, info string) {
	if upd.Message == nil {
		return false, "no message"
	}
	msg := upd.Message
	if msg.From != nil && msg.From.IsBot {
		return false, "bot ignored"
	}
	var userID int64
	if msg.From != nil {
		userID = msg.From.ID
	}
	if !t.IsAllowedChat(msg.Chat.ID, userID) {
		return false, "forbidden"
	}

	sessionKey := fmt.Sprintf("telegram:%d", msg.Chat.ID)
	rawText := strings.TrimSpace(msg.Text)
	caption := strings.TrimSpace(msg.Caption)

	// Built-in Fast-Path Commands
	switch strings.ToLower(strings.Fields(rawText + " ")[0]) {
	case "/start", "/help":
		hostName := t.Runner.HostName(ctx)
		if hostName == "" {
			hostName = t.Cfg.TargetHost
		}
		help := fmt.Sprintf(
			"AGY Mobile Gateway (%s)\n\nEnvie texto, foto, documento ou áudio para conversar com o agente.\n\nComandos:\n/new - Iniciar nova conversa\n/status - Status do host ativo\n/wol - Enviar Wake-on-LAN\n/outbox - Listar arquivos do Outbox",
			hostName,
		)
		_ = t.SendText(ctx, msg.Chat.ID, help)
		return true, "help"
	case "/new":
		_ = t.Store.ClearBotSession(sessionKey)
		_ = t.SendText(ctx, msg.Chat.ID, "Nova sessão iniciada. Pode enviar o próximo prompt!")
		return true, "new"
	case "/status":
		online := t.Runner.CheckOnline(ctx, 1500*time.Millisecond)
		hostName := t.Runner.HostName(ctx)
		if hostName == "" {
			hostName = t.Cfg.TargetHost
		}
		state := "Offline"
		if online {
			state = fmt.Sprintf("Online (%s)", t.Runner.ActiveOS(ctx))
		}
		_ = t.SendText(ctx, msg.Chat.ID, fmt.Sprintf("Host: %s\nStatus: %s", hostName, state))
		return true, "status"
	case "/wol":
		mac := t.Cfg.WolMAC
		if h, err := t.Store.GetActiveHostProfile(); err == nil && h != nil && h.WolMAC != "" {
			mac = h.WolMAC
		}
		if mac == "" {
			_ = t.SendText(ctx, msg.Chat.ID, "Nenhum MAC Wake-on-LAN configurado para o host ativo.")
			return true, "wol_missing"
		}
		if err := wol.SendMagicPacket(mac); err != nil {
			_ = t.SendText(ctx, msg.Chat.ID, fmt.Sprintf("Erro ao enviar WoL: %v", err))
			return true, "wol_err"
		}
		_ = t.SendText(ctx, msg.Chat.ID, fmt.Sprintf("Magic Packet enviado para %s.", mac))
		return true, "wol"
	case "/outbox":
		if !t.Runner.CheckOnline(ctx, 1500*time.Millisecond) {
			_ = t.SendText(ctx, msg.Chat.ID, "Host offline no momento.")
			return true, "offline"
		}
		files, err := t.Runner.ListOutbox(ctx)
		if err != nil || len(files) == 0 {
			_ = t.SendText(ctx, msg.Chat.ID, "Outbox vazio.")
			return true, "outbox_empty"
		}
		var lines []string
		for i, f := range files {
			if i >= 10 {
				break
			}
			lines = append(lines, fmt.Sprintf("- %s (%d B)", f.Name, f.Size))
		}
		_ = t.SendText(ctx, msg.Chat.ID, "Arquivos no Outbox:\n"+strings.Join(lines, "\n"))
		return true, "outbox"
	}

	if !t.Runner.CheckOnline(ctx, 1500*time.Millisecond) {
		_ = t.SendText(ctx, msg.Chat.ID, "Host offline no momento. Use /wol para acordar.")
		return true, "offline"
	}

	prompt := rawText
	var attached []string

	// Handle Photo (pick highest resolution) or Document
	var dlFileID, dlFallback string
	if len(msg.Photo) > 0 {
		dlFileID = msg.Photo[len(msg.Photo)-1].FileID
		dlFallback = "photo.jpg"
	} else if msg.Document != nil {
		dlFileID = msg.Document.FileID
		dlFallback = msg.Document.FileName
	}

	if dlFileID != "" {
		tmpPath, cleanName, err := t.downloadFileToTemp(ctx, dlFileID, dlFallback)
		if err != nil {
			_ = t.SendText(ctx, msg.Chat.ID, fmt.Sprintf("Falha ao baixar anexo do Telegram: %v", err))
			return true, "dl_err"
		}
		destPath, saveErr := t.Runner.SaveToInbox(ctx, tmpPath, cleanName)
		_ = os.Remove(tmpPath)
		if saveErr != nil {
			_ = t.SendText(ctx, msg.Chat.ID, fmt.Sprintf("Falha ao salvar anexo no Inbox: %v", saveErr))
			return true, "save_err"
		}
		attached = append(attached, destPath)
		if prompt == "" {
			prompt = caption
		}
		if prompt == "" {
			prompt = "Analise o arquivo enviado em anexo."
		}
	}

	// Handle Voice / Audio
	var audioID, audioFallback string
	if msg.Voice != nil {
		audioID = msg.Voice.FileID
		audioFallback = "voice.ogg"
	} else if msg.Audio != nil {
		audioID = msg.Audio.FileID
		audioFallback = "audio.mp3"
	}
	if audioID != "" {
		tmpPath, cleanName, err := t.downloadFileToTemp(ctx, audioID, audioFallback)
		if err != nil {
			_ = t.SendText(ctx, msg.Chat.ID, fmt.Sprintf("Falha ao baixar áudio do Telegram: %v", err))
			return true, "audio_dl_err"
		}
		transcript, trErr := t.transcribeAudioFile(ctx, tmpPath, cleanName)
		if trErr == nil && transcript != "" {
			_ = os.Remove(tmpPath)
			if prompt != "" {
				prompt = prompt + "\n" + transcript
			} else {
				prompt = transcript
			}
		} else {
			destPath, saveErr := t.Runner.SaveToInbox(ctx, tmpPath, cleanName)
			_ = os.Remove(tmpPath)
			if saveErr == nil {
				attached = append(attached, destPath)
				if prompt == "" {
					prompt = caption
				}
				if prompt == "" {
					prompt = "O usuário enviou um áudio (Whisper indisponível). Informe ou processe conforme possível."
				}
			}
		}
	}

	if strings.TrimSpace(prompt) == "" && len(attached) == 0 {
		return false, "empty"
	}

	var convPtr *string
	if existingConv, err := t.Store.GetBotSession(sessionKey); err == nil && existingConv != "" {
		convPtr = &existingConv
	}

	autoModel, copilotPerm := "auto", "copilot"
	started, err := t.Mgr.StartJob(ctx, jobs.CreateRequest{
		Prompt:         prompt,
		ConversationID: convPtr,
		Model:          &autoModel,
		PermissionMode: &copilotPerm,
		AttachedFiles:  attached,
	})
	if err != nil {
		_ = t.SendText(ctx, msg.Chat.ID, fmt.Sprintf("Erro ao iniciar job: %v", err))
		return true, "start_err"
	}
	jobID, _ := started["job_id"].(string)

	go func(chatID int64, jID, sKey string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		t.sendChatAction(bgCtx, chatID, "typing")
		typingStop := make(chan struct{})
		go func() {
			tk := time.NewTicker(4 * time.Second)
			defer tk.Stop()
			for {
				select {
				case <-typingStop:
					return
				case <-bgCtx.Done():
					return
				case <-tk.C:
					t.sendChatAction(bgCtx, chatID, "typing")
				}
			}
		}()

		res, waitErr := t.Bridge.WaitForJobResult(bgCtx, jID, 3*time.Minute+30*time.Second)
		close(typingStop)
		if waitErr != nil {
			_ = t.SendText(bgCtx, chatID, fmt.Sprintf("Erro ao aguardar resposta: %v", waitErr))
			return
		}
		if res.ConversationID != "" {
			_ = t.Store.SetBotSession(sKey, res.ConversationID)
		}
		if strings.TrimSpace(res.Reply) != "" {
			_ = t.SendText(bgCtx, chatID, res.Reply)
		} else if res.Error != "" {
			_ = t.SendText(bgCtx, chatID, "Erro: "+res.Error)
		}
		for _, outName := range res.OutboxFiles {
			if err := t.SendOutboxFile(bgCtx, chatID, outName); err != nil {
				log.Printf("telegram outbox send failed (%s): %v", outName, err)
			}
		}
	}(msg.Chat.ID, jobID, sessionKey)

	return true, jobID
}

// StartPolling runs a background Telegram getUpdates long-polling loop until ctx is cancelled.
func (t *TelegramBot) StartPolling(ctx context.Context) {
	if strings.TrimSpace(t.Cfg.TelegramBotToken) == "" {
		return
	}
	log.Println("Telegram bot long-polling ativo")
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		pollURL := fmt.Sprintf("%s?timeout=25&offset=%d", t.apiURL("getUpdates"), offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		resp, err := t.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(3 * time.Second)
			continue
		}
		var data struct {
			OK     bool             `json:"ok"`
			Result []TelegramUpdate `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && data.OK {
			for _, upd := range data.Result {
				if upd.UpdateID >= offset {
					offset = upd.UpdateID + 1
				}
				t.HandleUpdate(ctx, upd)
			}
		}
		_ = resp.Body.Close()
	}
}
