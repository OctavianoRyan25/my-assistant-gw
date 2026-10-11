# Deploy ke VPS

Pastikan VPS sudah memasang Docker Engine dan Docker Compose plugin.

1. Clone repository, lalu masuk ke direktorinya.
2. Buat file konfigurasi dan isi kredensial/API key:

   ```sh
   cp .env.example .env
   ```

   Atur `DB_HOST=mysql`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `MYSQL_ROOT_PASSWORD`, `APP_PORT`, `LLM_PROVIDER`, API key LLM, dan `WA_PHONE_NUMBER`. Gunakan password database yang kuat dan berbeda untuk `DB_PASSWORD` dan `MYSQL_ROOT_PASSWORD`.

3. Jalankan aplikasi:

   ```sh
   docker compose up -d --build
   ```

   Migrasi di direktori `migrations` otomatis dijalankan MySQL saat pertama kali membuat volume database.

4. Lihat log untuk memindai QR WhatsApp saat login pertama:

   ```sh
   docker compose logs -f app
   ```

Data MySQL dan sesi WhatsApp disimpan di Docker volumes. Untuk menghentikan aplikasi tanpa menghapus data, jalankan `docker compose down`. Jangan gunakan `docker compose down -v` kecuali memang ingin menghapus seluruh data persisten.

Untuk build dan menjalankan versi terbaru setelah perubahan kode:

```sh
docker compose up -d --build
```
