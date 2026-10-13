# Role pengguna

Migrasi `000011_add_user_role` menambahkan kolom `users.role` dengan nilai `User` atau `Admin`. Seluruh akun lama dan akun yang dibuat lewat registrasi publik mendapat `User` secara otomatis. Registrasi publik tidak menerima pilihan role.

Untuk mengaktifkan Admin pertama kali, jalankan migrasi lalu promosikan satu akun yang sudah terdaftar melalui akses database yang tepercaya:

```sql
UPDATE public.users SET role = 'Admin', updated_at = NOW()
WHERE email = 'alamat-admin@example.com';
```

Setelah itu, masuk kembali dengan akun tersebut. Admin dapat membuka **Kelola Pengguna** untuk melihat akun dan mengubah role akun lain. Perubahan hak akses API berlaku pada permintaan berikutnya; pengguna yang dipromosikan perlu masuk kembali agar menu Admin muncul.
