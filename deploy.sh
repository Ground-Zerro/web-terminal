#!/bin/bash

# Web Terminal - one-shot deployment script for fresh VPS
# Usage: curl -sL https://raw.githubusercontent.com/Ground-Zerro/web-terminal/main/deploy.sh | bash

REPO_URL="https://github.com/Ground-Zerro/web-terminal.git"
INSTALL_DIR="/root/terminal"
GO_VERSION="1.22.4"
XTERM_VERSION="5.3.0"

echo "=== Web Terminal Deployment ==="
echo ""

# 1. System update
echo "[1/10] Updating system..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq || echo "  WARNING: apt-get update failed, continuing anyway"
apt-get upgrade -y -qq || echo "  WARNING: apt-get upgrade failed, continuing anyway"

# 2. Install dependencies
echo "[2/10] Installing dependencies..."
apt-get install -y -qq git nginx curl build-essential || {
    echo "  ERROR: Failed to install dependencies"
    exit 1
}

# 3. Install Go
echo "[3/10] Installing Go ${GO_VERSION}..."
if command -v go &>/dev/null; then
    echo "  Go already installed: $(go version)"
else
    curl -sL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xzf - || {
        echo "  ERROR: Failed to download/install Go"
        exit 1
    }
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /root/.bashrc
    export PATH=$PATH:/usr/local/go/bin
    echo "  Installed: $(go version)"
fi

# 4. Clone fresh from repository
echo "[4/10] Getting source code..."
rm -rf "$INSTALL_DIR"
if git clone -q "$REPO_URL" "$INSTALL_DIR" 2>/dev/null; then
    cd "$INSTALL_DIR"
    echo "  Cloned from GitHub"
else
    echo "  ERROR: Cannot get source code."
    echo "  For private repos, copy source to $INSTALL_DIR first, then re-run."
    exit 1
fi

# 5. Initialize Go module
echo "[5/10] Initializing Go module..."
/usr/local/go/bin/go mod init webterminal || echo "  WARNING: go mod init failed"
/usr/local/go/bin/go mod tidy || echo "  WARNING: go mod tidy failed"

# 6. Build
echo "[6/10] Building binary..."
/usr/local/go/bin/go build -o webterminal . || {
    echo "  ERROR: Build failed"
    exit 1
}
echo "  Built: $(ls -la webterminal | awk '{print $5}') bytes"

# 7. Install vendor files (xterm.js)
echo "[7/10] Installing frontend vendor files..."
mkdir -p static/vendor
cd static/vendor

# Download xterm.js and addons from jsdelivr
for file in \
    "xterm@${XTERM_VERSION}/lib/xterm.min.js" \
    "xterm@${XTERM_VERSION}/css/xterm.min.css" \
    "@xterm/addon-fit@0.10.0/lib/addon-fit.min.js" \
    "@xterm/addon-unicode11@0.8.0/lib/addon-unicode11.min.js" \
    "@xterm/addon-web-links@0.11.0/lib/addon-web-links.min.js"; do
    filename=$(basename "$file")
    if ! curl -sL -o "$filename" "https://cdn.jsdelivr.net/npm/$file"; then
        echo "  WARNING: Failed to download $filename"
    fi
done

# Verify downloads
TOTAL_SIZE=$(wc -c *.js *.css 2>/dev/null | tail -1 | awk '{print $1}')
if [ "$TOTAL_SIZE" -lt 10000 ]; then
    echo "  WARNING: Vendor files may not have downloaded correctly (total: ${TOTAL_SIZE} bytes)"
else
    echo "  Downloaded vendor files (${TOTAL_SIZE} bytes total)"
fi

cd "$INSTALL_DIR"

# 8. Create config.json (if not exists)
if [ ! -f config.json ]; then
    echo "[8/10] Creating config.json..."
    cat > config.json <<'CONF'
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
CONF
    echo "  Created with default credentials: admin / root"
else
    echo "[8/10] config.json already exists, skipping"
fi

# 9. Setup systemd service
echo "[9/10] Setting up systemd service..."
cat > /etc/systemd/system/webterminal.service <<'SVC'
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
SVC

systemctl daemon-reload || echo "  WARNING: systemctl daemon-reload failed"
systemctl enable --now webterminal || echo "  WARNING: systemctl enable failed"
sleep 1

if systemctl is-active --quiet webterminal; then
    echo "  Web terminal service: running"
else
    echo "  WARNING: Web terminal may not be running. Check with: systemctl status webterminal"
fi

# 10. Configure nginx
echo "[10/10] Configuring nginx..."
cat > /etc/nginx/sites-available/webterminal <<'NGX'
server {
    listen 80;
    server_name _;

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
}
NGX

rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/webterminal /etc/nginx/sites-enabled/webterminal

if nginx -t 2>/dev/null; then
    systemctl enable --now nginx || echo "  WARNING: nginx enable failed"
    systemctl restart nginx || echo "  WARNING: nginx restart failed"
    echo "  Nginx: running"
else
    echo "  WARNING: Nginx config test failed, check with: nginx -t"
fi

# Restart webterminal to pick up new binary/config
echo "  Restarting webterminal..."
systemctl restart webterminal || echo "  WARNING: webterminal restart failed"
sleep 1

# Verify
echo ""
echo "=== Verifying deployment ==="
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1/web/ 2>/dev/null || echo "000")
if [ "$HTTP_CODE" = "200" ]; then
    echo "  HTTP check: OK (200)"
else
    echo "  HTTP check: FAIL ($HTTP_CODE)"
fi

SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || curl -s icanhazip.com 2>/dev/null || hostname -I | awk '{print $1}')

echo ""
echo "=== Deployment Complete ==="
echo ""
echo "  URL:      http://${SERVER_IP}/web/"
echo "  Login:    admin"
echo "  Password: root"
echo ""
echo "  Service:  systemctl status webterminal"
echo "  Logs:     journalctl -u webterminal -f"
echo ""
