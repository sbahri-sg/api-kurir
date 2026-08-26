# Security

- Jangan menyimpan API key, token, file `.env`, private key, atau data customer
  di package ini.
- Credential diterima hanya melalui header request HTTPS dan tidak dicatat.
- Response memakai `Cache-Control: no-store`.
- Request body dibatasi 64 KB dan upstream timeout dibatasi.
- Redirect upstream ditolak untuk menghindari pengiriman credential ke host lain.
- Endpoint transaksi hanya diuji dengan credential sandbox dan persetujuan admin.
- Rotasi atau pencabutan credential dilakukan melalui Partner Portal.

Laporkan insiden melalui kanal keamanan internal Emisell dan segera cabut
credential yang terindikasi bocor.
