package whatsapp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite" // Pure-Go SQLite driver (CGO-free, perfect for STB/ARM)
)

// WhatsAppClient wraps whatsmeow.Client and implements domain.WhatsAppSender
// as well as incoming message handling.
type WhatsAppClient struct {
	client     *whatsmeow.Client
	sentMsgIDs sync.Map // cache of recently sent message IDs to prevent reply loops
}

// NewWhatsAppClient initializes the whatsmeow SQLite store and connects to WhatsApp.
func NewWhatsAppClient(ctx context.Context) (*WhatsAppClient, error) {
	dbLog := waLog.Stdout("Database", "INFO", false)
	container, err := sqlstore.New(ctx, "sqlite", "file:whatsapp.db?_pragma=foreign_keys(1)", dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to open whatsmeow session store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get first device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, waLog.Noop)

	if client.Store.ID == nil {
		// Belum login → perlu scan QR
		qrChan, _ := client.GetQRChannel(ctx)
		if err := client.Connect(); err != nil {
			return nil, fmt.Errorf("failed to connect whatsmeow: %w", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Sudah pernah login → connect langsung
		if err := client.Connect(); err != nil {
			return nil, fmt.Errorf("failed to connect whatsmeow: %w", err)
		}
	}

	wac := &WhatsAppClient{
		client: client,
	}

	// Background routine to purge sentMsgIDs older than 10 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			now := time.Now()
			wac.sentMsgIDs.Range(func(key, val interface{}) bool {
				if t, ok := val.(time.Time); ok && now.Sub(t) > 10*time.Minute {
					wac.sentMsgIDs.Delete(key)
				}
				return true
			})
		}
	}()

	return wac, nil
}

// SendMessage sends a text message to a WhatsApp user or JID.
func (s *WhatsAppClient) SendMessage(ctx context.Context, to string, text string) error {
	to = strings.TrimSpace(to)
	var jid types.JID
	if strings.Contains(to, "@") {
		var err error
		jid, err = types.ParseJID(to)
		if err != nil {
			return fmt.Errorf("invalid JID %s: %w", to, err)
		}
	} else {
		jid = types.NewJID(to, types.DefaultUserServer)
	}

	resp, err := s.client.SendMessage(ctx, jid, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return fmt.Errorf("gagal kirim pesan ke %s: %w", to, err)
	}

	// Record sent ID to prevent processing self-replies
	s.sentMsgIDs.Store(resp.ID, time.Now())
	return nil
}

// RegisterMessageHandler listens for incoming WhatsApp messages and delegates them to ChatUsecase.
// It filters incoming messages to only process messages from the authorized user (PRD: single-user).
func (s *WhatsAppClient) RegisterMessageHandler(chatUC domain.ChatUsecase, allowedPhone string, log *zap.Logger) {
	cleanAllowed := cleanNumber(allowedPhone)

	s.client.AddEventHandler(func(rawEvt interface{}) {
		evt, ok := rawEvt.(*events.Message)
		if !ok {
			return
		}

		// Unwrap raw message (ephemeral, view-once, device-sent, etc.)
		unwrapped := evt.UnwrapRaw()
		if unwrapped != nil {
			evt = unwrapped
		}

		log.Info("📩 WhatsApp event diterima",
			zap.String("sender", evt.Info.Sender.String()),
			zap.String("chat", evt.Info.Chat.String()),
			zap.Bool("isFromMe", evt.Info.IsFromMe),
			zap.Bool("isGroup", evt.Info.IsGroup),
		)

		// 1. Abaikan pesan lama jika timestamp lebih dari 10 menit
		if time.Since(evt.Info.Timestamp) > 10*time.Minute {
			log.Info("mengabaikan pesan kadaluarsa (>10m)", zap.Time("timestamp", evt.Info.Timestamp))
			return
		}

		// 2. Abaikan pesan grup (V1 adalah personal assistant single-user)
		if evt.Info.IsGroup {
			log.Debug("mengabaikan pesan dari grup")
			return
		}

		// 3. Abaikan broadcast status / newsletter
		if evt.Info.Chat.Server == types.NewsletterServer || evt.Info.Chat.Server == types.BroadcastServer {
			return
		}

		// 4. Abaikan pesan yang baru saja dikirim oleh bot kita sendiri
		if _, isOwn := s.sentMsgIDs.Load(evt.Info.ID); isOwn {
			log.Debug("mengabaikan pesan balasan bot sendiri", zap.String("id", string(evt.Info.ID)))
			return
		}

		// 5. Cek otorisasi pengirim: hanya layani nomor owner (Octaviano)
		senderUser := cleanNumber(evt.Info.Sender.User)
		chatUser := cleanNumber(evt.Info.Chat.User)

		// Jika nomor berupa LID (Linked Identity), coba resolve ke phone number
		if evt.Info.Sender.Server == types.HiddenUserServer && s.client.Store != nil {
			if pn, err := s.client.Store.GetAltJID(context.Background(), evt.Info.Sender); err == nil && !pn.IsEmpty() {
				senderUser = cleanNumber(pn.User)
			}
		}
		if evt.Info.Chat.Server == types.HiddenUserServer && s.client.Store != nil {
			if pn, err := s.client.Store.GetAltJID(context.Background(), evt.Info.Chat); err == nil && !pn.IsEmpty() {
				chatUser = cleanNumber(pn.User)
			}
		}

		if cleanAllowed != "" {
			isAllowed := (senderUser == cleanAllowed || chatUser == cleanAllowed)

			// Khusus skenario self-chat (user login dengan WA sendiri dan kirim pesan ke dirinya sendiri):
			if !isAllowed && s.client.Store.ID != nil && cleanNumber(s.client.Store.ID.User) == cleanAllowed {
				if evt.Info.IsFromMe && (cleanNumber(evt.Info.Chat.User) == cleanAllowed || evt.Info.Chat.Server == types.HiddenUserServer) {
					isAllowed = true
				}
			}

			if !isAllowed {
				log.Info("⚠️ Pesan dari nomor tidak dikenal diabaikan",
					zap.String("senderUser", senderUser),
					zap.String("chatUser", chatUser),
					zap.String("allowedUser", cleanAllowed),
				)
				return
			}
		}

		// 6. Ekstrak isi teks pesan
		text := extractMessageText(evt.Message)
		if strings.TrimSpace(text) == "" {
			log.Info("pesan diterima tanpa teks yang bisa diproses (media/stiker)")
			return
		}

		log.Info("💬 Memproses pesan WhatsApp",
			zap.String("from", evt.Info.Sender.User),
			zap.String("message", text),
		)

		// 7. Tandai pesan sudah dibaca (centang biru) & kirim status 'sedang mengetik'
		go func() {
			_ = s.client.MarkRead(context.Background(), []types.MessageID{evt.Info.ID}, evt.Info.Timestamp, evt.Info.Chat, evt.Info.Sender)
			_ = s.client.SendChatPresence(context.Background(), evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		}()

		// 8. Proses pesan lewat AI ChatUsecase (Intent router: reminder, expense, search, chat)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			reply, err := chatUC.HandleMessage(ctx, text)
			// Hentikan status mengetik
			_ = s.client.SendChatPresence(ctx, evt.Info.Chat, types.ChatPresencePaused, types.ChatPresenceMediaText)

			if err != nil {
				log.Error("gagal memproses pesan AI", zap.Error(err))
				reply = "Waduh wir, ada kendala pas proses pesan kamu: " + err.Error()
			}

			if strings.TrimSpace(reply) == "" {
				return
			}

			// 9. Kirim balasan ke chat asal
			targetJID := evt.Info.Chat.String()
			if err := s.SendMessage(ctx, targetJID, reply); err != nil {
				log.Error("gagal mengirim balasan WhatsApp", zap.Error(err), zap.String("to", targetJID))
			} else {
				log.Info("✅ Berhasil mengirim balasan WhatsApp", zap.String("to", targetJID))
			}
		}()
	})

	log.Info("WhatsApp incoming message listener berhasil dipasang", zap.String("allowedUser", cleanAllowed))
}

// Close disconnects the whatsmeow client.
func (s *WhatsAppClient) Close() {
	s.client.Disconnect()
}

// extractMessageText extracts plain text from various WhatsApp message structures.
func extractMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.GetConversation() != "" {
		return msg.GetConversation()
	}
	if msg.GetExtendedTextMessage().GetText() != "" {
		return msg.GetExtendedTextMessage().GetText()
	}
	if msg.GetImageMessage().GetCaption() != "" {
		return msg.GetImageMessage().GetCaption()
	}
	if msg.GetDocumentMessage().GetCaption() != "" {
		return msg.GetDocumentMessage().GetCaption()
	}
	if msg.GetVideoMessage().GetCaption() != "" {
		return msg.GetVideoMessage().GetCaption()
	}
	if msg.GetDeviceSentMessage().GetMessage() != nil {
		return extractMessageText(msg.GetDeviceSentMessage().GetMessage())
	}
	if msg.GetEphemeralMessage().GetMessage() != nil {
		return extractMessageText(msg.GetEphemeralMessage().GetMessage())
	}
	if msg.GetViewOnceMessage().GetMessage() != nil {
		return extractMessageText(msg.GetViewOnceMessage().GetMessage())
	}
	if msg.GetViewOnceMessageV2().GetMessage() != nil {
		return extractMessageText(msg.GetViewOnceMessageV2().GetMessage())
	}
	if msg.GetDocumentWithCaptionMessage().GetMessage() != nil {
		return extractMessageText(msg.GetDocumentWithCaptionMessage().GetMessage())
	}
	return ""
}

// cleanNumber removes +, @s.whatsapp.net, spaces, and hyphens leaving only digits.
// If it starts with '0' (e.g., 0812...), it automatically normalizes to '62812...'.
func cleanNumber(n string) string {
	n = strings.TrimPrefix(n, "+")
	if idx := strings.Index(n, "@"); idx != -1 {
		n = n[:idx]
	}
	var sb strings.Builder
	for _, r := range n {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	res := sb.String()
	if strings.HasPrefix(res, "0") && len(res) >= 10 {
		res = "62" + res[1:]
	}
	return res
}
