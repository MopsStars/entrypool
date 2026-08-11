# entrypool

Сторож для нескольких российских VPN-входов под **одним** доменом (например `entry.example.com`).

Раз в 30 секунд стучится в порты на каждом IP. Если вход умер — пишет в Telegram.  
Если DNS на Cloudflare — может сам убрать мёртвый IP из A-записей.

Клиентам по-прежнему один сервер в списке. Несколько IP крутятся только в DNS.

---

## Что поставить на сервер (по шагам)

Ставь **не** на единственный RU, который мониторь (лучше второй вход / EU / любая VPS с интернетом).

### 1. Склонировать и установить

```bash
git clone https://github.com/MopsStars/entrypool.git
cd entrypool
sudo sh install.sh
```

Появится бинарник `/usr/local/bin/entrypool` и черновик конфига `/etc/entrypool/config.env`.

### 2. Заполнить конфиг

```bash
sudo nano /etc/entrypool/config.env
```

Минимум так (пока DNS не Cloudflare — только алерты):

```bash
MODE=alert
ENTRY_NAME=entry.example.com
ENTRY_IPS=1.1.1.1,2.2.2.2,3.3.3.3

PROBE_PORTS=8443,2083,25565
PROBE_TIMEOUT=3
PROBE_NEED_OK=1
FAIL_THRESHOLD=3
OK_THRESHOLD=2

TELEGRAM_BOT_TOKEN=123456:ABC...
TELEGRAM_CHAT_ID= твой_chat_id
```

Подставь свои IP входов и порты, которые реально слушаются.  
`ENTRY_NAME` — то имя, на которое смотрят клиенты / Hosts в панели.

### 3. Проверить руками

```bash
sudo entrypool doctor
```

Должно быть `ok` по каждому IP. Если `FAIL` — порт закрыт, неверный IP или файрвол.

### 4. Включить автозапуск

```bash
sudo systemctl enable --now entrypool.timer
sudo systemctl status entrypool.timer
journalctl -u entrypool.service -n 30 --no-pager
```

Готово: каждые ~30 сек идёт проверка. Упал вход → сообщение в Telegram → убери его A у регистратора вручную.

---

## Когда DNS уже на Cloudflare

Тогда чекер может сам править A:

```bash
MODE=pool
PROVIDERS=cloudflare
ENTRY_NAME=entry.example.com
ENTRY_IPS=1.1.1.1,2.2.2.2,3.3.3.3
CF_API_TOKEN=...
CF_ZONE_NAME=example.com
DNS_TTL=60
```

В панели VPN Hosts address оставь **домен** (`entry.example.com`), не голые IP.

---

## Полезные команды

| Команда | Зачем |
|---|---|
| `entrypool doctor` | Проверить входы прямо сейчас |
| `entrypool run` | Один цикл (как timer) |
| `entrypool status` | Что чекер думает про каждый IP |
| `entrypool version` | Версия |

---

## Режимы коротко

| MODE | Что делает |
|---|---|
| `alert` | Только Telegram. DNS правишь сам. **Начни с этого.** |
| `pool` | + сам чистит мёртвые A в Cloudflare |
| `failover` | Пишет в Remnawave IP одного живого входа (обычно не нужно) |

---

## Если что-то не так

```bash
# конфиг читается?
sudo cat /etc/entrypool/config.env

# timer жив?
systemctl status entrypool.timer

# последний прогон
journalctl -u entrypool.service -n 50 --no-pager

# разовый прогон с логом
sudo entrypool run
```
