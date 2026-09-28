# PRD: Personal AI Assistant via WhatsApp

**Owner:** Octaviano **Status:** Draft v1 **Platform:** Go backend + WhatsApp (whatsmeow)

---

## 1. Latar Belakang

Kebutuhan akan asisten pribadi yang bisa diakses langsung lewat WhatsApp, tanpa perlu install aplikasi tambahan. Dibangun dengan Go, dihosting di STB (spesifikasi terbatas), sehingga desain sistem harus ringan dan mengandalkan API LLM gratis (bukan model lokal).

## 2. Tujuan Produk

- Menyediakan satu titik akses (WhatsApp) untuk mengelola reminder, pengeluaran, pencarian informasi, dan diskusi santai.
- Meminimalkan friksi: semua interaksi lewat chat natural language, tanpa command yang kaku.
- Berjalan stabil di infrastruktur ringan (STB) dengan konsumsi resource rendah.

## 3. Target Pengguna

Single-user (personal use) — Octaviano sebagai satu-satunya pengguna di fase awal. Tidak perlu sistem multi-tenant/auth kompleks di V1.

## 4. Lingkup (Scope)

### Masuk lingkup V1

1. Reminder berbasis jadwal (dibuat via chat)
2. Pencatatan pengeluaran + laporan bulanan otomatis (tanggal 30)
3. Web search / RAG untuk menjawab pertanyaan dengan info terkini
4. Brainstorming / diskusi ringan (general chat dengan LLM)

### Di luar lingkup V1 (backlog)

- Multi-user / sharing akses
- Voice note transcription
- Habit tracker (gym/jogging/gaming log) — dari diskusi sebelumnya, ditunda ke V2
- Dashboard web terpisah
- Multi-modal (gambar/struk)

---

## 5. Rincian Fitur

### 5.1 Reminder Berdasarkan Jadwal

**User story:** Sebagai user, saya ingin bilang ke asisten "ingatkan aku besok jam 8 pagi meeting sama klien" dan asisten akan mengirim pesan WA pada waktu tersebut.

**Alur:**

1. User kirim pesan natural language berisi intent reminder.
2. LLM mem-parse jadi structured data: `{judul, datetime, recurring?}`.
3. Sistem simpan ke tabel `reminders` dan daftarkan job di job queue (scheduler).
4. Pada waktu yang ditentukan, worker mengirim pesan WA ke user.

**Kebutuhan fungsional:**

- Bisa membuat reminder sekali jalan maupun berulang (harian/mingguan/bulanan).
- Bisa melihat daftar reminder aktif ("reminder apa aja yang aktif?").
- Bisa membatalkan/mengubah reminder via chat ("batalkan reminder meeting besok").
- Konfirmasi balik setelah reminder berhasil dibuat, termasuk parsing waktu yang benar (perlu timezone eksplisit, misal WIB).

**Data model (sketsa):**

```
reminders
- id
- title
- scheduled_at (timestamp)
- recurrence (nullable: daily/weekly/monthly/cron expr)
- status (active/done/cancelled)
- created_at
```

**Komponen teknis:** Go scheduler (misal `asynq` + Redis, atau cron sederhana `robfig/cron` kalau mau lebih ringan tanpa Redis).

**Acceptance criteria:**

- Reminder terkirim maksimal 1 menit dari waktu yang dijadwalkan.
- Parsing waktu relatif ("besok", "3 jam lagi", "tiap Senin jam 9") berhasil ≥90% dari uji coba manual.

---

### 5.2 Pencatatan Pengeluaran + Laporan Bulanan

**User story:** Sebagai user, saya ingin mencatat pengeluaran lewat chat ("keluar 50rb buat makan siang") dan setiap tanggal 30 menerima rekap total otomatis.

**Alur:**

1. User kirim pesan berisi nominal + deskripsi pengeluaran.
2. LLM ekstrak: `{nominal, kategori (opsional), deskripsi, tanggal}`.
3. Simpan ke tabel `expenses`.
4. Scheduled job jalan tiap tanggal 30 (atau akhir bulan jika bulan \<30 hari), menghitung total dan mengirim rekap ke WA.

**Kebutuhan fungsional:**

- Bisa mencatat pengeluaran dengan atau tanpa kategori eksplisit (kategori bisa di-infer LLM: makan, transport, hiburan, dll).
- Bisa query manual kapan saja ("total pengeluaran bulan ini berapa?", "pengeluaran kategori makan minggu ini?").
- Laporan otomatis tanggal 30 berisi: total keseluruhan + breakdown per kategori.
- Handle bulan dengan \<30 hari (Februari) — jalankan di hari terakhir bulan tersebut.

**Data model (sketsa):**

```
expenses
- id
- amount
- category
- description
- occurred_at (timestamp)
- created_at
```

**Acceptance criteria:**

- Pesan pengeluaran berhasil dicatat dengan ekstraksi nominal yang akurat.
- Laporan bulanan terkirim otomatis tanpa intervensi manual.

---

### 5.3 Web Search / RAG

**User story:** Sebagai user, saya ingin bertanya hal-hal yang butuh info terkini ("berita AI minggu ini apa?") dan asisten menjawab berdasarkan hasil pencarian, bukan cuma dari training data LLM.

**Alur:**

1. Sistem mendeteksi intent yang butuh informasi real-time (berbeda dari general chat).
2. Panggil web search API (misal Serper, Tavily, atau SearchAPI — banyak yang punya free tier).
3. Hasil pencarian diringkas dan dimasukkan sebagai context ke prompt LLM.
4. LLM menjawab berdasarkan context tersebut, dengan sitasi sumber jika relevan.

**Kebutuhan fungsional:**

- Deteksi kapan perlu search vs kapan cukup jawab langsung dari LLM (hemat API call).
- Batasi jumlah hasil pencarian yang di-fetch (misal top 3-5) untuk menjaga context tetap ringkas.
- Tampilkan sumber/link di jawaban jika relevan.

**Catatan implementasi RAG sederhana:**

- V1 tidak perlu vector database — cukup "search-augmented generation" (ambil hasil search, masukkan ke prompt).
- Vector DB (misal untuk RAG atas dokumen pribadi/notes) bisa jadi fitur V2 kalau dibutuhkan.

**Acceptance criteria:**

- Pertanyaan yang butuh info terkini terjawab dengan data yang relevan dan up-to-date.
- Response time tetap wajar (\<10 detik) meski ada langkah search tambahan.

---

### 5.4 Brainstorming / Diskusi Ringan

**User story:** Sebagai user, saya ingin ngobrol santai atau brainstorming ide dengan asisten, dengan asisten mengingat konteks percakapan sebelumnya dalam sesi yang sama.

**Alur:**

1. Pesan yang tidak match ke intent lain (reminder, pengeluaran, search) dianggap general chat.
2. Sistem ambil histori percakapan terakhir (context window terbatas, misal 10-20 pesan terakhir) sebagai memory.
3. Kirim ke LLM dengan system prompt kepribadian asisten.
4. Balas ke user.

**Kebutuhan fungsional:**

- Context percakapan tersimpan per sesi (bisa reset dengan command seperti "mulai obrolan baru").
- Gaya bahasa bisa disesuaikan (santai/informal, mengikuti gaya user).

**Data model (sketsa):**

```
chat_history
- id
- role (user/assistant)
- message
- created_at
```

**Acceptance criteria:**

- Asisten mampu mempertahankan konteks minimal 5-10 giliran percakapan.
- Respons terasa natural, tidak kaku seperti bot command-based.

---

## 6. Intent Routing (Lintas Fitur)

Karena semua fitur diakses lewat chat natural language yang sama, dibutuhkan lapisan **intent classifier** di awal pipeline:

```
Pesan masuk → Intent Classifier (LLM-based atau rule+LLM hybrid) →
  ├─ reminder
  ├─ expense
  ├─ search/RAG
  └─ general chat (default fallback)
```

Bisa pakai LLM call kecil untuk klasifikasi (prompt terpisah dengan output JSON `{intent, confidence}`), atau kombinasi keyword matching (untuk kasus jelas) + LLM fallback (untuk kasus ambigu) supaya hemat API call.

## 7. Arsitektur Teknis (Ringkas)

```
WhatsApp (whatsmeow) 
    → Go service (HTTP/webhook handler)
        → Intent Router
        → LLM Client (Gemini/Groq/OpenRouter — sesuai kebutuhan)
        → Web Search Client (untuk fitur RAG)
        → MySQL (reminders, expenses, chat_history)
        → Scheduler (asynq / robfig-cron) untuk reminder & laporan bulanan
```

**Pertimbangan resource (karena hosting di STB):**

- Gunakan MySQL ringan (atau SQLite kalau load rendah dan single-user).
- Scheduler berbasis cron sederhana lebih hemat resource dibanding job queue penuh (Redis+asynq) kalau volume reminder tidak besar.
- LLM & search sepenuhnya via API eksternal (tidak ada inference lokal).

## 8. Non-Functional Requirements

- **Reliability:** Reminder harus tetap terkirim meski service sempat restart (persist job state, bukan in-memory saja).
- **Cost control:** Rate limit pemanggilan LLM API gratis dipantau agar tidak kena throttle/429.
- **Privacy:** Data pengeluaran & chat pribadi tidak boleh bocor — API key dan koneksi WA harus aman.
- **Maintainability:** Kode modular per fitur (reminder, expense, search, chat) supaya gampang ditambah fitur baru di V2.

## 9. Metrik Keberhasilan

- Reminder terkirim tepat waktu (>95% akurasi).
- Laporan pengeluaran bulanan terkirim otomatis setiap bulan tanpa gagal.
- Waktu respons rata-rata untuk chat \< 5 detik (di luar fitur search).
- Penggunaan harian aktif oleh user (self-tracking, minimal dipakai untuk minimal 2 dari 4 fitur per minggu).

## 10. Roadmap Singkat

| Fase | Fitur |
| --- | --- |
| V1 (fokus PRD ini) | Reminder, Pencatatan pengeluaran, Web search/RAG, Brainstorming |
| V2 | Habit tracker (gym/jogging/gaming), voice note, multi-modal |
| V3 | RAG atas dokumen/notes pribadi (vector DB), dashboard web |

## 11. Pertanyaan Terbuka

- Timezone default untuk parsing waktu reminder — WIB tetap atau perlu dikonfigurasi?
- Kategori pengeluaran: predefined list atau full LLM-inferred/freeform?
- Provider LLM utama: Gemini, Groq, atau OpenRouter — atau fallback berlapis?
- Perlu command eksplisit (misal `/reset`) untuk reset context chat, atau full natural language?