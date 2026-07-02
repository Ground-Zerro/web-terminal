# Web Terminal

[English](#english) | [Русский](#русский)

---

## English

Web-based terminal with file browser and performance monitoring. Single Go binary, no external runtime dependencies.

### Features

- Real PTY shell via WebSocket (not emulation)
- File browser with drag-and-drop upload/download
- Path input with autocomplete — type a path and press Enter to navigate
- Middle-click on file/folder pastes its path into the terminal
- Upload progress bar for files and nested folders
- Performance widget: CPU graph, RAM, network throughput
- Dark theme, responsive layout
- Brute force protection with fail2ban integration
- WebSocket keepalive — session survives idle

### Keyboard Shortcuts & Mouse Actions

| Action | Shortcut |
|--------|----------|
| Copy selected text | Auto-copies to clipboard on select |
| Paste into terminal | Right-click or Ctrl+Shift+V |
| Paste file/folder path into terminal | Middle-click on file in browser |
| Navigate to path | Type path in address bar, press Enter |
| Autocomplete path | Type partial path, use Arrow keys / Tab |

### Quick Start

#### Prerequisites

- Go 1.21+
- nginx (for reverse proxy + TLS)
- Linux (reads /proc for metrics)

#### One-liner deploy on fresh VPS

```bash
curl -sL https://raw.githubusercontent.com/Ground-Zerro/web-terminal/main/deploy.sh | bash
```

This installs Go, nginx, builds the binary, configures everything, and starts the service.

#### Build

```bash
go build -o webterminal .
```

#### Run (development)

```bash
./webterminal
# Listens on http://127.0.0.1:8081
```

### Production Deployment

#### 1. Systemd Service

Create `/etc/systemd/system/webterminal.service`:

```ini
[Unit]
Description=Web Terminal Server (behind nginx)
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/terminal
ExecStart=/root/terminal/webterminal
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=webterminal

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now webterminal
```

#### 2. Nginx Reverse Proxy

Add to your nginx config (`/etc/nginx/sites-available/your-site`):

```nginx
location /web/ {
    proxy_pass http://127.0.0.1:8081/;
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
- `proxy_read_timeout 300s` — WebSocket idle timeout (keepalive ping runs every 30s)
- `proxy_set_header Upgrade/Connection` — required for WebSocket upgrade

```bash
nginx -t && systemctl reload nginx
```

#### 3. SSL (optional)

For HTTPS, add TLS to the nginx server block. Self-signed or Let's Encrypt — your choice.

### Architecture

```
Browser ──HTTPS──▶ nginx:443 ──HTTP──▶ Go:8081
                        │                  │
                   static files      ├── /api/login
                   (index.html,      ├── /api/terminal  (WebSocket)
                    css/, js/)       ├── /api/metrics   (CPU/RAM/Net)
                                     ├── /api/files/*   (file browser)
                                     └── /              (SPA entry)
```

### Project Structure

```
.
├── main.go                    # Entry point, routes, server
├── config.go                  # Config file loader with defaults
├── config.json.example        # Example configuration file
├── go.mod / go.sum            # Go module dependencies
├── handlers/
│   ├── auth.go                # Login/logout, session management
│   ├── terminal.go            # WebSocket PTY, keepalive ping/pong
│   ├── files.go               # File CRUD, upload, download
│   └── metrics.go             # System metrics (/proc reader)
├── middleware/
│   └── bruteforce.go          # In-memory IP rate limiting
└── static/
    ├── index.html             # SPA entry
    ├── css/style.css          # Dark theme styles
    └── js/
        ├── app.js             # Main app logic
        ├── terminal.js        # xterm.js + WebSocket client
        └── perf.js            # Performance widget (CPU graph)
```

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/login` | Authenticate, returns session token |
| POST | `/api/logout` | Invalidate session |
| GET | `/api/terminal?token=...` | WebSocket terminal connection |
| GET | `/api/metrics` | System metrics (CPU/RAM/network) |
| GET | `/api/files?path=...` | List directory contents |
| POST | `/api/files/upload` | Upload file(s) |
| GET | `/api/files/download?path=...` | Download file |
| GET | `/api/files/download-folder?path=...` | Download directory as ZIP |
| POST | `/api/files/mkdir` | Create directory |
| POST | `/api/files/delete` | Delete file or directory |

### Configuration

The application supports a `config.json` file located next to the binary (in the working directory). If the file is missing or a parameter is absent, built-in defaults are used.

Create `config.json`:

```json
{
    "listen_addr": "127.0.0.1:8081",
    "login": "admin",
    "password": "root",
    "work_dir": "/root/terminal",
    "terminal_dir": "/root",
    "fail2ban_log": "/var/log/webterminal-bruteforce.log",
    "max_attempts": 6,
    "ban_duration": 15,
    "read_timeout": 30,
    "write_timeout": 30,
    "idle_timeout": 120
}
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `listen_addr` | string | `127.0.0.1:8081` | Server bind address |
| `login` | string | `admin` | Login credential |
| `password` | string | `root` | Password credential |
| `work_dir` | string | `/root/terminal` | Application working directory |
| `terminal_dir` | string | `/root` | Terminal shell starting directory |
| `fail2ban_log` | string | `/var/log/webterminal-bruteforce.log` | fail2ban log path |
| `max_attempts` | int | `6` | Max login attempts before ban |
| `ban_duration` | int (min) | `15` | Ban duration in minutes |
| `read_timeout` | int (sec) | `30` | HTTP read timeout |
| `write_timeout` | int (sec) | `30` | HTTP write timeout |
| `idle_timeout` | int (sec) | `120` | HTTP idle timeout |

### Replacing Vendor Files

The `static/vendor/` directory is gitignored. Install manually:

```bash
cd static/vendor
# xterm.js
curl -LO https://unpkg.com/@xterm/xterm@5.5.0/lib/xterm.min.js
curl -LO https://unpkg.com/@xterm/xterm@5.5.0/css/xterm.min.css
# Fit addon
curl -LO https://unpkg.com/@xterm/addon-fit@0.10.0/lib/addon-fit.min.js
# Unicode addon
curl -LO https://unpkg.com/@xterm/addon-unicode11@0.8.0/lib/addon-unicode11.min.js
# Web links addon
curl -LO https://unpkg.com/@xterm/addon-web-links@0.11.0/lib/addon-web-links.min.js
```

### Commands

```bash
# Check status
systemctl status webterminal

# View logs
journalctl -u webterminal -f

# Restart (briefly drops active sessions)
systemctl restart webterminal

# Graceful deploy without dropping sessions
kill $(pgrep -f webterminal) && sleep 1 && nohup ./webterminal > /var/log/webterminal.log 2>&1 &
```

### License

MIT

---

## Русский

Веб-терминал с файловым менеджером и мониторингом производительности. Одиночный Go-бинарник, без внешних рантайм-зависимостей.

### Возможности

- Настоящий PTY-шелл через WebSocket (не эмуляция)
- Файловый менеджер с drag-and-drop загрузкой/скачиванием
- Поле ввода пути с автокомплитом — введите путь и нажмите Enter
- Средний клик по файлу/папке вставляет путь в терминал
- Прогресс-бар при загрузке файлов и вложенных каталогов
- Виджет производительности: график CPU, RAM, сетевой трафик
- Тёмная тема, адаптивный интерфейс
- Защита от брутфорса с интеграцией fail2ban
- WebSocket keepalive — сессия переживает простой

### Горячие клавиши и действия мыши

| Действие | Комбинация |
|----------|------------|
| Копирование выделенного текста | Автоматически в буфер обмена |
| Вставка в терминал | Правый клик или Ctrl+Shift+V |
| Вставить путь файла/папки в терминал | Средний клик по файлу в браузере |
| Перейти по пути | Введите путь в адресную строку, нажмите Enter |
| Автодополнение пути | Введите часть пути, используйте стрелки / Tab |

### Быстрый старт

#### Требования

- Go 1.21+
- nginx (для реверс-прокси + TLS)
- Linux (чтение /proc для метрик)

#### Однострочный деплой на чистый VPS

```bash
curl -sL https://raw.githubusercontent.com/Ground-Zerro/web-terminal/main/deploy.sh | bash
```

Скрипт установит Go, nginx, соберёт бинарник, настроит всё и запустит сервис.

#### Сборка

```bash
go build -o webterminal .
```

#### Запуск (разработка)

```bash
./webterminal
# Слушает на http://127.0.0.1:8081
```

### Продакшн-деплой

#### 1. Systemd-сервис

Создайте `/etc/systemd/system/webterminal.service`:

```ini
[Unit]
Description=Web Terminal Server (behind nginx)
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/terminal
ExecStart=/root/terminal/webterminal
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=webterminal

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now webterminal
```

#### 2. Nginx reverse proxy

Добавьте в конфигурацию nginx (`/etc/nginx/sites-available/your-site`):

```nginx
location /web/ {
    proxy_pass http://127.0.0.1:8081/;
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
- `proxy_read_timeout 300s` — таймаут idle-подключения WebSocket (keepalive-пинг каждые 30 сек)
- `proxy_set_header Upgrade/Connection` — обязательно для WebSocket-апгрейда

```bash
nginx -t && systemctl reload nginx
```

#### 3. SSL (опционально)

Для HTTPS добавьте TLS в server-блок nginx. Самоподписанный или Let's Encrypt — на ваш выбор.

### Архитектура

```
Браузер ──HTTPS──▶ nginx:443 ──HTTP──▶ Go:8081
                        │                  │
                   статические        ├── /api/login
                   файлы              ├── /api/terminal  (WebSocket)
                   (index.html,       ├── /api/metrics   (CPU/RAM/Сеть)
                   css/, js/)         ├── /api/files/*   (файловый менеджер)
                                      └── /              (SPA точка входа)
```

### Структура проекта

```
.
├── main.go                    # Точка входа, маршруты, сервер
├── config.go                  # Загрузчик конфигурации с дефолтами
├── config.json.example        # Пример конфигурационного файла
├── go.mod / go.sum            # Зависимости Go-модуля
├── handlers/
│   ├── auth.go                # Логин/выход, управление сессиями
│   ├── terminal.go            # WebSocket PTY, keepalive ping/pong
│   ├── files.go               # Файловые CRUD-операции, загрузка/скачивание
│   └── metrics.go             # Системные метрики (чтение /proc)
├── middleware/
│   └── bruteforce.go          # Лимитирование IP в памяти
└── static/
    ├── index.html             # Точка входа SPA
    ├── css/style.css          # Стили тёмной темы
    └── js/
        ├── app.js             # Основная логика приложения
        ├── terminal.js        # Клиент xterm.js + WebSocket
        └── perf.js            # Виджет производительности (график CPU)
```

### API-эндпоинты

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/api/login` | Авторизация, возвращает токен сессии |
| POST | `/api/logout` | Завершение сессии |
| GET | `/api/terminal?token=...` | WebSocket-подключение терминала |
| GET | `/api/metrics` | Системные метрики (CPU/RAM/сеть) |
| GET | `/api/files?path=...` | Список файлов в директории |
| POST | `/api/files/upload` | Загрузка файла(ов) |
| GET | `/api/files/download?path=...` | Скачивание файла |
| GET | `/api/files/download-folder?path=...` | Скачивание директории как ZIP |
| POST | `/api/files/mkdir` | Создание директории |
| POST | `/api/files/delete` | Удаление файла или директории |

### Конфигурация

Приложение поддерживает файл `config.json` рядом с бинарником (в рабочей директории). Если файл отсутствует или параметр не указан, используются встроенные дефолты.

Создайте `config.json`:

```json
{
    "listen_addr": "127.0.0.1:8081",
    "login": "admin",
    "password": "root",
    "work_dir": "/root/terminal",
    "terminal_dir": "/root",
    "fail2ban_log": "/var/log/webterminal-bruteforce.log",
    "max_attempts": 6,
    "ban_duration": 15,
    "read_timeout": 30,
    "write_timeout": 30,
    "idle_timeout": 120
}
```

| Параметр | Тип | Дефолт | Описание |
|----------|-----|--------|----------|
| `listen_addr` | string | `127.0.0.1:8081` | Адрес прослушивания сервера |
| `login` | string | `admin` | Логин |
| `password` | string | `root` | Пароль |
| `work_dir` | string | `/root/terminal` | Рабочая директория приложения |
| `terminal_dir` | string | `/root` | Начальная директория терминала |
| `fail2ban_log` | string | `/var/log/webterminal-bruteforce.log` | Путь к логу fail2ban |
| `max_attempts` | int | `6` | Макс. попыток входа до бана |
| `ban_duration` | int (мин) | `15` | Длительность бана в минутах |
| `read_timeout` | int (сек) | `30` | Таймаут чтения HTTP |
| `write_timeout` | int (сек) | `30` | Таймаут записи HTTP |
| `idle_timeout` | int (сек) | `120` | Таймаут idle-подключения |

### Замена vendor-файлов

Директория `static/vendor/` исключена из git. Установите вручную:

```bash
cd static/vendor
# xterm.js
curl -LO https://unpkg.com/@xterm/xterm@5.5.0/lib/xterm.min.js
curl -LO https://unpkg.com/@xterm/xterm@5.5.0/css/xterm.min.css
# Fit addon
curl -LO https://unpkg.com/@xterm/addon-fit@0.10.0/lib/addon-fit.min.js
# Unicode addon
curl -LO https://unpkg.com/@xterm/addon-unicode11@0.8.0/lib/addon-unicode11.min.js
# Web links addon
curl -LO https://unpkg.com/@xterm/addon-web-links@0.11.0/lib/addon-web-links.min.js
```

### Команды

```bash
# Проверить статус
systemctl status webterminal

# Смотреть логи
journalctl -u webterminal -f

# Перезапустить (временно разорвёт активные сессии)
systemctl restart webterminal

# Мягкий деплой без разрыва сессий
kill $(pgrep -f webterminal) && sleep 1 && nohup ./webterminal > /var/log/webterminal.log 2>&1 &
```

### Лицензия

MIT
