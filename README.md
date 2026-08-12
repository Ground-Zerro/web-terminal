# Web Terminal

[English](#english) | [Русский](#русский)

---

## English

A web terminal with a file browser and performance monitoring, shipped as a
single self-contained binary. The frontend is compiled into the executable —
copy one file to a server, run it, and open the page.

### Features

- Real PTY shell over WebSocket (not an emulation)
- File browser with drag-and-drop upload and download
- Path input with autocomplete — type a path and press Enter to navigate
- Middle-click a file or folder to paste its path into the terminal
- Upload progress for files and nested folders
- Performance widget: CPU graph, RAM, network throughput
- Dark theme, responsive layout
- Brute-force protection with a fail2ban-compatible log
- WebSocket keepalive — the session survives idle periods

### Quick Start

```bash
./webterminal
```

That is the whole setup. The server listens on `0.0.0.0:8089`; log in with
**root** / **admin**.

> **Change the password before putting this on a public network.** The defaults
> give anyone who can reach the port a root shell. The binary prints a warning at
> startup while the default password is in use on a non-loopback address.

#### Build from source

```bash
./build.sh
```

Produces `bin/webterminal`: statically linked for `linux/amd64`, stripped,
with `static/` embedded via `go:embed`. The script refuses to build when an
embedded asset is missing (that would otherwise yield a binary whose interface
404s at runtime), confirms the result is not dynamically linked, and then clears
the leftover build artifacts and the Go build cache. Sources are never touched.

Requires Go 1.22+; set `GO=/path/to/go` if the toolchain is not on `PATH`.
Linux only — metrics are read from `/proc`.

### Keyboard Shortcuts & Mouse Actions

| Action | Shortcut |
|--------|----------|
| Copy selected text | Auto-copies to clipboard on select |
| Paste into terminal | Right-click or Ctrl+Shift+V |
| Paste file/folder path into terminal | Middle-click a file in the browser |
| Navigate to path | Type a path in the address bar, press Enter |
| Autocomplete path | Type part of a path, use Arrow keys / Tab |

### Configuration

Settings are resolved in three layers, each overriding the one before it:

1. Built-in defaults
2. `config.json` **next to the binary** (optional)
3. Command-line options

Anything left unset at every layer keeps its default, so a partial config file
and a single command-line option are both perfectly valid.

#### Generating a config file

```bash
./webterminal --genconfig
```

Writes `config.json` next to the binary with every key at its default value and
mode `0600`. It refuses to overwrite an existing file.

```json
{
    "listen_addr": "0.0.0.0:8089",
    "login": "root",
    "password": "admin",
    "terminal_dir": "/root",
    "fail2ban_log": "/var/log/webterminal-bruteforce.log",
    "max_attempts": 6,
    "ban_duration": 15,
    "session_ttl": 720,
    "read_header_timeout": 15,
    "idle_timeout": 120
}
```

#### Parameters

Every config key has a matching command-line option: replace `_` with `-`.

| Config key | Option | Type | Default | Description |
|---|---|---|---|---|
| `listen_addr` | `--listen-addr` | string | `0.0.0.0:8089` | Address and port to listen on |
| `login` | `--login` | string | `root` | Login name |
| `password` | `--password` | string | `admin` | Password |
| `terminal_dir` | `--terminal-dir` | string | `/root` | Directory the shell starts in |
| `fail2ban_log` | `--fail2ban-log` | string | `/var/log/webterminal-bruteforce.log` | Failed-login log for fail2ban |
| `max_attempts` | `--max-attempts` | int | `6` | Failed logins before an address is banned |
| `ban_duration` | `--ban-duration` | min | `15` | Ban duration |
| `session_ttl` | `--session-ttl` | min | `720` | Idle time before a session expires; every request slides the deadline |
| `read_header_timeout` | `--read-header-timeout` | sec | `15` | Deadline for reading request headers |
| `idle_timeout` | `--idle-timeout` | sec | `120` | Keep-alive idle timeout |

Request and response bodies are deliberately not time-limited, so multi-gigabyte
uploads, ZIP downloads and long-lived WebSocket sessions are never cut off.

#### Examples

```bash
./webterminal --help
./webterminal --login alice --password 'correct horse battery staple'
./webterminal --listen-addr 127.0.0.1:9090      # bind to loopback for nginx
./webterminal --terminal-dir /srv --session-ttl 60
```

Options override the config file for that run only; the file is never modified.

### Running as a Service (optional)

```bash
sudo ./webterminal --service
```

`--service` is a toggle and requires root:

- **No unit yet** → writes `/etc/systemd/system/webterminal.service` with
  `ExecStart` set to wherever this binary currently lives, reloads systemd, then
  enables and starts the service.
- **Unit already there** → stops, disables and deletes it, then reloads systemd.
  This happens regardless of which binary the existing unit points at, so moving
  the binary is a matter of running `--service` twice: once at the old location
  to clear the unit, once at the new one to recreate it.

```bash
sudo install -m 755 webterminal /usr/local/bin/webterminal
sudo /usr/local/bin/webterminal --service
journalctl -u webterminal -f
```

The generated unit:

```ini
[Unit]
Description=Web Terminal
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart="/usr/local/bin/webterminal"
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=webterminal

[Install]
WantedBy=multi-user.target
```

`WorkingDirectory` is not needed: assets are embedded and `config.json` is looked
up next to the executable. The service therefore reads the same config file that
`--genconfig` writes, so configure the service by editing that file and running
`systemctl restart webterminal`.

To uninstall completely: `sudo ./webterminal --service` to drop the unit, then
remove the binary and its `config.json`.

### Behind nginx with TLS (optional)

The binary speaks plain HTTP and has no certificate handling by design. To put it
behind TLS, bind it to loopback and terminate TLS at nginx:

```bash
./webterminal --listen-addr 127.0.0.1:9090
```

```nginx
location /web/ {
    client_max_body_size 10g;
    proxy_pass http://127.0.0.1:9090/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 300s;
    proxy_send_timeout 300s;
}
```

Key settings:
- `proxy_read_timeout 300s` — WebSocket idle timeout (keepalive pings every 30s)
- `proxy_set_header Upgrade/Connection` — required for the WebSocket upgrade
- `X-Real-IP` — the binary trusts proxy headers only from a loopback peer, so
  brute-force bans apply to the real client address

```bash
apt-get install -y certbot python3-certbot-nginx
certbot --nginx -d your-domain.example
nginx -t && systemctl reload nginx
```

### Architecture

```
Browser ──HTTPS──▶ nginx ──HTTP──▶ webterminal
                                   ├── /              (page, embedded)
                                   ├── /static/*      (css/js, embedded)
                                   ├── /api/login
                                   ├── /api/terminal  (WebSocket PTY)
                                   ├── /api/metrics   (CPU/RAM/network)
                                   └── /api/files/*   (file browser)
```

### Project Structure

```
.
├── build.sh                   # static linux/amd64 build into bin/, then cleanup
├── main.go                    # go:embed, flags, routes, graceful shutdown
├── config.go                  # defaults, config file, flags, --help rendering
├── service.go                 # --service: install/remove the systemd unit
├── go.mod / go.sum            # Go module dependencies
├── handlers/
│   ├── http.go                # Response envelope, JSON helpers, method guard
│   ├── auth.go                # Login/logout, sessions, Require middleware
│   ├── terminal.go            # WebSocket PTY bridge, keepalive, resize
│   ├── files.go               # File CRUD, upload, download, path resolution
│   └── metrics.go             # System metrics (buffered /proc reader)
├── middleware/
│   └── bruteforce.go          # In-memory IP rate limiting
└── static/                    # Embedded into the binary at build time
    ├── index.html
    ├── css/style.css
    ├── js/
    │   ├── core.js            # formatBytes, Settings, Session, ApiClient
    │   ├── keyboard.js        # On-screen key bar
    │   ├── terminal.js        # xterm.js + WebSocket client
    │   ├── filelist.js        # File list rendering and sorting
    │   ├── perf.js            # Performance widget (CPU graph)
    │   └── app.js             # Session flow and wiring
    └── vendor/                # xterm.js 5.5.0 + addons (committed, required to build)
```

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/login` | Authenticate, returns a session token |
| POST | `/api/logout` | Invalidate the session |
| GET | `/api/terminal?token=...` | WebSocket terminal connection |
| GET | `/api/metrics` | System metrics (CPU/RAM/network) |
| GET | `/api/files?path=...` | List directory contents |
| POST | `/api/files/upload` | Upload a file |
| GET | `/api/files/download?path=...` | Download a file |
| GET | `/api/files/download-folder?path=...` | Download a directory as ZIP |
| POST | `/api/files/mkdir` | Create a directory |
| POST | `/api/files/delete` | Delete a file or directory |

Every endpoint except `/api/login` requires a session token, normally sent as an
`Authorization` header. `/api/terminal` and the two download endpoints also
accept `?token=...`, because a WebSocket handshake and an `<a download>` link
cannot carry custom headers. Such tokens end up in reverse-proxy access logs.

### Updating the Vendored Frontend

`static/vendor/` is committed because `go:embed` needs it at build time.

```bash
cd static/vendor
curl -LO https://unpkg.com/@xterm/xterm@5.5.0/lib/xterm.min.js
curl -LO https://unpkg.com/@xterm/xterm@5.5.0/css/xterm.min.css
curl -LO https://unpkg.com/@xterm/addon-fit@0.10.0/lib/addon-fit.min.js
curl -LO https://unpkg.com/@xterm/addon-unicode11@0.8.0/lib/addon-unicode11.min.js
curl -LO https://unpkg.com/@xterm/addon-web-links@0.11.0/lib/addon-web-links.min.js
```

Rebuild afterwards so the new files are embedded.

### License

MIT

---

## Русский

Веб-терминал с файловым менеджером и мониторингом производительности в виде
одного самодостаточного бинарника. Фронтенд вкомпилирован в исполняемый файл —
скопируйте один файл на сервер, запустите и откройте страницу.

### Возможности

- Настоящий PTY-шелл через WebSocket (не эмуляция)
- Файловый менеджер с drag-and-drop загрузкой и скачиванием
- Поле ввода пути с автодополнением — введите путь и нажмите Enter
- Средний клик по файлу или папке вставляет путь в терминал
- Прогресс загрузки для файлов и вложенных каталогов
- Виджет производительности: график CPU, RAM, сетевой трафик
- Тёмная тема, адаптивный интерфейс
- Защита от брутфорса с логом в формате для fail2ban
- WebSocket keepalive — сессия переживает простой

### Быстрый старт

```bash
./webterminal
```

Это вся настройка. Сервер слушает `0.0.0.0:8089`, вход — **root** / **admin**.

> **Смените пароль, прежде чем выставлять сервис в публичную сеть.** Умолчания
> дают root-шелл любому, кто дотянется до порта. Пока используется пароль по
> умолчанию и адрес не петлевой, бинарник пишет предупреждение при старте.

#### Сборка из исходников

```bash
./build.sh
```

На выходе `bin/webterminal`: статически слинкованный под `linux/amd64`, без
отладочных символов, со встроенной через `go:embed` папкой `static/`. Скрипт
откажется собирать, если пропал встраиваемый файл (иначе получился бы бинарник,
у которого интерфейс отдаёт 404), проверит, что результат не слинкован
динамически, и уберёт за собой артефакты сборки и кэш Go. Исходники не трогает.

Нужен Go 1.22+; если тулчейн не в `PATH`, укажите `GO=/path/to/go`. Только Linux —
метрики читаются из `/proc`.

### Горячие клавиши и действия мыши

| Действие | Комбинация |
|----------|------------|
| Копирование выделенного текста | Автоматически в буфер обмена |
| Вставка в терминал | Правый клик или Ctrl+Shift+V |
| Вставить путь файла/папки в терминал | Средний клик по файлу в браузере |
| Перейти по пути | Введите путь в адресную строку, нажмите Enter |
| Автодополнение пути | Введите часть пути, используйте стрелки / Tab |

### Конфигурация

Настройки собираются из трёх слоёв, каждый перекрывает предыдущий:

1. Встроенные значения по умолчанию
2. `config.json` **рядом с бинарником** (необязателен)
3. Ключи запуска

Всё, что не задано ни на одном слое, остаётся значением по умолчанию — поэтому
частичный конфиг и одиночный ключ запуска одинаково допустимы.

#### Генерация конфигурационного файла

```bash
./webterminal --genconfig
```

Пишет `config.json` рядом с бинарником со всеми ключами в значениях по умолчанию
и правами `0600`. Существующий файл не перезаписывается.

```json
{
    "listen_addr": "0.0.0.0:8089",
    "login": "root",
    "password": "admin",
    "terminal_dir": "/root",
    "fail2ban_log": "/var/log/webterminal-bruteforce.log",
    "max_attempts": 6,
    "ban_duration": 15,
    "session_ttl": 720,
    "read_header_timeout": 15,
    "idle_timeout": 120
}
```

#### Параметры

У каждого ключа конфига есть парный ключ запуска: замените `_` на `-`.

| Ключ конфига | Ключ запуска | Тип | Умолчание | Описание |
|---|---|---|---|---|
| `listen_addr` | `--listen-addr` | string | `0.0.0.0:8089` | Адрес и порт прослушивания |
| `login` | `--login` | string | `root` | Логин |
| `password` | `--password` | string | `admin` | Пароль |
| `terminal_dir` | `--terminal-dir` | string | `/root` | Начальный каталог шелла |
| `fail2ban_log` | `--fail2ban-log` | string | `/var/log/webterminal-bruteforce.log` | Лог неудачных входов для fail2ban |
| `max_attempts` | `--max-attempts` | int | `6` | Неудачных входов до бана адреса |
| `ban_duration` | `--ban-duration` | мин | `15` | Длительность бана |
| `session_ttl` | `--session-ttl` | мин | `720` | Простой до истечения сессии; каждый запрос сдвигает дедлайн |
| `read_header_timeout` | `--read-header-timeout` | сек | `15` | Дедлайн чтения заголовков запроса |
| `idle_timeout` | `--idle-timeout` | сек | `120` | Таймаут keep-alive |

Тела запросов и ответов намеренно не ограничены по времени, чтобы многогигабайтные
загрузки, скачивание ZIP и долгие WebSocket-сессии не обрывались.

#### Примеры

```bash
./webterminal --help
./webterminal --login alice --password 'correct horse battery staple'
./webterminal --listen-addr 127.0.0.1:9090      # петлевой адрес для nginx
./webterminal --terminal-dir /srv --session-ttl 60
```

Ключи перекрывают конфигурационный файл только для текущего запуска; сам файл не
изменяется.

### Запуск как сервис (опционально)

```bash
sudo ./webterminal --service
```

`--service` работает как переключатель и требует root:

- **Юнита ещё нет** → создаётся `/etc/systemd/system/webterminal.service`, где
  `ExecStart` указывает на текущее расположение этого бинарника; systemd
  перечитывает конфигурацию, служба включается в автозапуск и стартует.
- **Юнит уже есть** → служба останавливается, снимается с автозапуска и юнит
  удаляется, после чего systemd перечитывает конфигурацию. Это происходит
  независимо от того, на какой бинарник указывал существующий юнит, поэтому
  переезд бинарника — это два запуска `--service`: один на старом месте, чтобы
  убрать юнит, второй на новом, чтобы создать заново.

```bash
sudo install -m 755 webterminal /usr/local/bin/webterminal
sudo /usr/local/bin/webterminal --service
journalctl -u webterminal -f
```

Создаваемый юнит:

```ini
[Unit]
Description=Web Terminal
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart="/usr/local/bin/webterminal"
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=webterminal

[Install]
WantedBy=multi-user.target
```

`WorkingDirectory` не нужен: статика встроена, а `config.json` ищется рядом с
исполняемым файлом. Служба читает тот же конфиг, что пишет `--genconfig`, — то
есть настраивается правкой этого файла и командой `systemctl restart webterminal`.

Для полного удаления: `sudo ./webterminal --service` уберёт юнит, затем удалите
бинарник и его `config.json`.

### За nginx с TLS (опционально)

Бинарник намеренно работает по обычному HTTP и не умеет сертификаты. Чтобы
закрыть его TLS, привяжите к петлевому адресу и терминируйте TLS на nginx:

```bash
./webterminal --listen-addr 127.0.0.1:9090
```

```nginx
location /web/ {
    client_max_body_size 10g;
    proxy_pass http://127.0.0.1:9090/;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 300s;
    proxy_send_timeout 300s;
}
```

Ключевые настройки:
- `proxy_read_timeout 300s` — таймаут простоя WebSocket (keepalive-пинг раз в 30 с)
- `proxy_set_header Upgrade/Connection` — обязательно для WebSocket-апгрейда
- `X-Real-IP` — бинарник доверяет заголовкам прокси только от петлевого пира,
  поэтому бан брутфорса применяется к реальному адресу клиента

```bash
apt-get install -y certbot python3-certbot-nginx
certbot --nginx -d your-domain.example
nginx -t && systemctl reload nginx
```

### Архитектура

```
Браузер ──HTTPS──▶ nginx ──HTTP──▶ webterminal
                                   ├── /              (страница, встроена)
                                   ├── /static/*      (css/js, встроены)
                                   ├── /api/login
                                   ├── /api/terminal  (WebSocket PTY)
                                   ├── /api/metrics   (CPU/RAM/сеть)
                                   └── /api/files/*   (файловый менеджер)
```

### Структура проекта

```
.
├── build.sh                   # статическая сборка linux/amd64 в bin/ и уборка
├── main.go                    # go:embed, ключи, маршруты, graceful shutdown
├── config.go                  # умолчания, конфиг, ключи, отрисовка --help
├── service.go                 # --service: установка/удаление systemd-юнита
├── go.mod / go.sum            # Зависимости Go-модуля
├── handlers/
│   ├── http.go                # Конверт ответа, JSON-хелперы, guard по методу
│   ├── auth.go                # Логин/выход, сессии, middleware Require
│   ├── terminal.go            # Мост WebSocket ↔ PTY, keepalive, resize
│   ├── files.go               # Файловые CRUD-операции, разбор путей
│   └── metrics.go             # Системные метрики (буферизованное чтение /proc)
├── middleware/
│   └── bruteforce.go          # Лимитирование IP в памяти
└── static/                    # Встраивается в бинарник при сборке
    ├── index.html
    ├── css/style.css
    ├── js/
    │   ├── core.js            # formatBytes, Settings, Session, ApiClient
    │   ├── keyboard.js        # Экранная панель клавиш
    │   ├── terminal.js        # Клиент xterm.js + WebSocket
    │   ├── filelist.js        # Отрисовка и сортировка списка файлов
    │   ├── perf.js            # Виджет производительности (график CPU)
    │   └── app.js             # Поток сессии и связывание компонентов
    └── vendor/                # xterm.js 5.5.0 + аддоны (в репозитории, нужны для сборки)
```

### API-эндпоинты

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/api/login` | Авторизация, возвращает токен сессии |
| POST | `/api/logout` | Завершение сессии |
| GET | `/api/terminal?token=...` | WebSocket-подключение терминала |
| GET | `/api/metrics` | Системные метрики (CPU/RAM/сеть) |
| GET | `/api/files?path=...` | Список файлов в директории |
| POST | `/api/files/upload` | Загрузка файла |
| GET | `/api/files/download?path=...` | Скачивание файла |
| GET | `/api/files/download-folder?path=...` | Скачивание директории как ZIP |
| POST | `/api/files/mkdir` | Создание директории |
| POST | `/api/files/delete` | Удаление файла или директории |

Все эндпоинты, кроме `/api/login`, требуют токен сессии — обычно он передаётся
в заголовке `Authorization`. `/api/terminal` и два эндпоинта скачивания
дополнительно принимают `?token=...`, поскольку WebSocket-рукопожатие и ссылка
`<a download>` не умеют отправлять свои заголовки. Такие токены попадают
в access-лог обратного прокси.

### Обновление vendor-файлов фронтенда

`static/vendor/` лежит в репозитории, потому что `go:embed` нужен на этапе сборки.

```bash
cd static/vendor
curl -LO https://unpkg.com/@xterm/xterm@5.5.0/lib/xterm.min.js
curl -LO https://unpkg.com/@xterm/xterm@5.5.0/css/xterm.min.css
curl -LO https://unpkg.com/@xterm/addon-fit@0.10.0/lib/addon-fit.min.js
curl -LO https://unpkg.com/@xterm/addon-unicode11@0.8.0/lib/addon-unicode11.min.js
curl -LO https://unpkg.com/@xterm/addon-web-links@0.11.0/lib/addon-web-links.min.js
```

После этого пересоберите бинарник, чтобы новые файлы попали внутрь.

### Лицензия

MIT
