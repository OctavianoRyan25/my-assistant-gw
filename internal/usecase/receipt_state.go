package usecase

import (
	"fmt"
	"sync"
	"time"

	"github.com/OctavianoRyan25/my-assistant-gw/internal/domain"
)

// pendingReceipt holds a scanned receipt that is awaiting user confirmation or correction.
type pendingReceipt struct {
	receipt   *domain.ReceiptParsed
	expiresAt time.Time
}

// receiptStateStore is a simple in-memory store for pending receipt confirmations.
// Since this is a single-user personal assistant, a simple mutex map is sufficient.
type receiptStateStore struct {
	mu      sync.Mutex
	pending map[string]*pendingReceipt // key: user JID or constant "self"
}

func newReceiptStateStore() *receiptStateStore {
	s := &receiptStateStore{
		pending: make(map[string]*pendingReceipt),
	}
	go s.cleanupLoop()
	return s
}

// set stores a pending receipt for a user. Expires after 10 minutes.
func (s *receiptStateStore) set(userKey string, r *domain.ReceiptParsed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[userKey] = &pendingReceipt{
		receipt:   r,
		expiresAt: time.Now().Add(10 * time.Minute),
	}
}

// get retrieves the pending receipt for a user (if still valid).
func (s *receiptStateStore) get(userKey string) (*domain.ReceiptParsed, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[userKey]
	if !ok || time.Now().After(p.expiresAt) {
		delete(s.pending, userKey)
		return nil, false
	}
	return p.receipt, true
}

// clear removes any pending receipt for the user.
func (s *receiptStateStore) clear(userKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, userKey)
}

// cleanupLoop periodically evicts expired entries.
func (s *receiptStateStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.pending {
			if now.After(v.expiresAt) {
				delete(s.pending, k)
			}
		}
		s.mu.Unlock()
	}
}

// ─── Formatting helpers ────────────────────────────────────────────────────────

// FormatReceiptPreview formats a parsed receipt for user confirmation.
func FormatReceiptPreview(r *domain.ReceiptParsed) string {
	if r == nil || len(r.Items) == 0 {
		return "❌ Tidak ada item yang berhasil terbaca dari struk ini."
	}

	var sb fmt.Stringer
	_ = sb
	out := "🧾 *Hasil Scan Struk*\n"
	if r.StoreName != "" {
		out += fmt.Sprintf("🏪 *Toko:* %s\n", r.StoreName)
	}
	out += "\n*Item:*\n"
	for i, item := range r.Items {
		out += fmt.Sprintf("%d. %s — Rp %s (%s)\n", i+1, item.Description, formatRupiah(item.Amount), item.Category)
	}
	if r.Total > 0 {
		out += fmt.Sprintf("\n💰 *Total:* Rp %s", formatRupiah(r.Total))
	}
	out += "\n\n✅ Ketik *ya* atau *simpan* untuk menyimpan semua item ke pengeluaran.\n"
	out += "✏️ Untuk koreksi ketik contoh:\n"
	out += "   • _koreksi 1 nama=Nasi Goreng jumlah=25000 kategori=makan_\n"
	out += "   • _hapus 2_ (hapus item nomor 2)\n"
	out += "❌ Ketik *batal* untuk membatalkan."
	return out
}

// FormatReceiptSaved formats a success message after saving receipt items.
func FormatReceiptSaved(items []domain.ReceiptItem, storeName string) string {
	if len(items) == 0 {
		return "Tidak ada item yang disimpan wir."
	}
	store := ""
	if storeName != "" {
		store = " dari " + storeName
	}
	msg := fmt.Sprintf("✅ *%d pengeluaran%s berhasil dicatat!* 😼\n\n", len(items), store)
	total := 0.0
	for _, item := range items {
		msg += fmt.Sprintf("• %s — Rp %s (%s)\n", item.Description, formatRupiah(item.Amount), item.Category)
		total += item.Amount
	}
	msg += fmt.Sprintf("\n💰 Total: Rp %s\nJangan boros-boros ya wir! 🐱", formatRupiah(total))
	return msg
}
