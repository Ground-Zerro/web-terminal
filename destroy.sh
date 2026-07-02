#!/bin/bash

# Web Terminal - uninstall script
# Usage: curl -sL https://raw.githubusercontent.com/Ground-Zerro/web-terminal/main/destroy.sh | bash

INSTALL_DIR="/root/terminal"

echo "=== Web Terminal Uninstall ==="
echo ""
echo "This will remove:"
echo "  - Web terminal service (systemd)"
echo "  - Nginx config for /web/"
echo "  - Project files ($INSTALL_DIR)"
echo "  - Logs (/var/log/webterminal*)"
echo "  - Go (if installed by deploy.sh)"
echo ""

read -p "Are you sure? (y/N) " confirm </dev/tty
if [[ ! "$confirm" =~ ^[yY]$ ]]; then
    echo "Aborted."
    exit 0
fi

echo ""

# 1. Stop and remove webterminal service
echo "[1/5] Removing webterminal service..."
if [ -f /etc/systemd/system/webterminal.service ]; then
    systemctl stop webterminal 2>/dev/null || true
    systemctl disable webterminal 2>/dev/null || true
    rm -f /etc/systemd/system/webterminal.service
    systemctl daemon-reload || true
    echo "  Service removed"
else
    echo "  Service not found, skipping"
fi

# 2. Remove nginx config
echo "[2/5] Removing nginx config..."
if [ -L /etc/nginx/sites-enabled/webterminal ]; then
    rm -f /etc/nginx/sites-enabled/webterminal
    echo "  Symlink removed"
fi
if [ -f /etc/nginx/sites-available/webterminal ]; then
    rm -f /etc/nginx/sites-available/webterminal
    echo "  Config removed"
fi
if systemctl is-active --quiet nginx 2>/dev/null; then
    systemctl reload nginx 2>/dev/null || true
    echo "  Nginx reloaded"
fi

# 3. Remove project files
echo "[3/5] Removing project files..."
if [ -d "$INSTALL_DIR" ]; then
    rm -rf "$INSTALL_DIR"
    echo "  $INSTALL_DIR removed"
else
    echo "  $INSTALL_DIR not found, skipping"
fi

# 4. Remove logs
echo "[4/5] Removing logs..."
rm -f /var/log/webterminal*.log 2>/dev/null || true
journalctl --rotate --vacuum-time=1s 2>/dev/null || true
echo "  Logs cleaned"

# 5. Remove Go (only if installed by deploy.sh)
echo "[5/5] Removing Go..."
if [ -d /usr/local/go ]; then
    rm -rf /usr/local/go
    echo "  /usr/local/go removed"
    # Clean PATH entry from .bashrc
    if [ -f /root/.bashrc ]; then
        sed -i '\|/usr/local/go/bin|d' /root/.bashrc
        echo "  PATH entry removed from .bashrc"
    fi
else
    echo "  Go not found at /usr/local/go, skipping"
fi

echo ""
echo "=== Uninstall Complete ==="
echo ""
echo "  Note: System packages (git, nginx, curl, build-essential) were NOT removed."
echo "  If you want to remove them: apt-get remove --purge git nginx curl build-essential"
echo ""
