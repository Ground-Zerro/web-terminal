// Terminal Manager
class TerminalManager {
    constructor() {
        this.terminal = null;
        this.fitAddon = null;
        this.unicodeAddon = null;
        this.socket = null;
        this.token = null;
        this.decoder = null;
        this.pingInterval = null;
        this.pingTimeout = null;
    }

    init(token) {
        this.token = token;

        // Persistent UTF-8 decoder — keeps state across WebSocket frames
        // so multi-byte chars split across frames are reassembled correctly
        this.decoder = new TextDecoder('utf-8');

        // Terminal settings
        this.terminal = new Terminal({
            cursorBlink: true,
            cursorStyle: 'bar',
            fontSize: 14,
            fontFamily: '"Cascadia Code", Menlo, Monaco, "Courier New", monospace',
            allowProposedApi: true,
            scrollback: 10000,
            tabStopWidth: 8,
            theme: {
                background: '#1a1a1a',
                foreground: '#ffffff',
                cursor: '#4a9eff',
                cursorAccent: '#1a1a1a',
                selectionBackground: '#4a9eff44',
                black: '#000000',
                red: '#ff4444',
                green: '#44ff44',
                yellow: '#ffff44',
                blue: '#4a9eff',
                magenta: '#ff44ff',
                cyan: '#44ffff',
                white: '#ffffff',
                brightBlack: '#555555',
                brightRed: '#ff7777',
                brightGreen: '#77ff77',
                brightYellow: '#ffff77',
                brightBlue: '#77aaff',
                brightMagenta: '#ff77ff',
                brightCyan: '#77ffff',
                brightWhite: '#ffffff'
            }
        });

        // Load addons
        this.fitAddon = new FitAddon.FitAddon();
        this.terminal.loadAddon(this.fitAddon);

        this.unicodeAddon = new Unicode11Addon.Unicode11Addon();
        this.terminal.loadAddon(this.unicodeAddon);
        this.terminal.unicode.activeVersion = '11';

        this.terminal.loadAddon(new WebLinksAddon.WebLinksAddon());

        // Open terminal in DOM
        this.terminal.open(document.getElementById('terminal'));

        // Initial fit after DOM settles
        this._fitDelayed(200);

        // Copy on select
        this.terminal.onSelectionChange(() => {
            const sel = this.terminal.getSelection();
            if (sel) {
                navigator.clipboard.writeText(sel).catch(() => {});
            }
        });

        // Ctrl+Shift+V paste
        this.terminal.attachCustomKeyEventHandler((event) => {
            if (event.ctrlKey && event.shiftKey && event.key === 'V') {
                if (event.type === 'keydown') this.pasteFromClipboard();
                return false;
            }
            return true;
        });

        // Right-click paste: capture clipboard on mousedown (before contextmenu)
        // Clipboard API works reliably from mousedown but not from contextmenu
        this.terminal.element.addEventListener('mousedown', (e) => {
            if (e.button === 2) {
                this._pendingPaste = navigator.clipboard.readText().catch(() => null);
            }
        });

        this.terminal.element.addEventListener('contextmenu', (e) => {
            e.preventDefault();
            if (this._pendingPaste) {
                this._pendingPaste.then(text => {
                    if (text) this.sendText(text);
                    this._pendingPaste = null;
                });
            }
        });

        // Handle paste from Ctrl+V / browser paste menu
        this.terminal.element.addEventListener('paste', (e) => {
            const text = (e.clipboardData || window.clipboardData).getData('text');
            if (text) {
                e.preventDefault();
                this.sendText(text);
            }
        });

        // Window resize
        let rt;
        window.addEventListener('resize', () => {
            clearTimeout(rt);
            rt = setTimeout(() => this._fitNow(), 100);
        });

        // Container size changes
        const obs = new MutationObserver(() => {
            if (this.terminal.element && this.terminal.element.offsetHeight > 0) {
                clearTimeout(rt);
                rt = setTimeout(() => this._fitNow(), 50);
            }
        });
        obs.observe(document.getElementById('terminal'), {
            attributes: true,
            attributeFilter: ['style', 'class']
        });

        this.connect();
    }

    _fitNow() {
        if (this.fitAddon) {
            this.fitAddon.fit();
            this.sendResize();
        }
    }

    _fitDelayed(ms) {
        setTimeout(() => this._fitNow(), ms);
    }

    async pasteFromClipboard() {
        try {
            const text = await navigator.clipboard.readText();
            if (text && this.socket && this.socket.readyState === WebSocket.OPEN) {
                this.socket.send(text);
            }
        } catch (e) { /* ignore */ }
    }

    sendResize() {
        if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;
        const dims = this.fitAddon.proposeDimensions();
        const cols = (dims && dims.cols > 0) ? dims.cols : this.terminal.cols;
        const rows = (dims && dims.rows > 0) ? dims.rows : this.terminal.rows;
        this.socket.send(JSON.stringify({ type: 'resize', data: { cols, rows } }));
    }

    connect() {
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const url = `${proto}//${location.host}${location.pathname}api/terminal?token=${this.token}`;

        this.socket = new WebSocket(url);
        this.socket.binaryType = 'arraybuffer';

        this.socket.onopen = () => {
            // Re-fit and send dimensions once the connection is live
            setTimeout(() => this._fitNow(), 100);
            this.startPing();
        };

        this.socket.onmessage = (event) => {
            // Handle pong response to keepalive ping
            if (typeof event.data === 'string' && event.data === 'pong') {
                this.onPong();
                return;
            }

            if (event.data instanceof ArrayBuffer) {
                // Use persistent decoder — incomplete multi-byte sequences
                // at frame boundaries are carried over to the next call
                const text = this.decoder.decode(event.data, { stream: true });
                this.terminal.write(text);
            } else {
                this.terminal.write(event.data);
            }
        };

        this.socket.onclose = () => {
            this.stopPing();
            this.terminal.write('\r\n\x1b[31mConnection lost. Please refresh.\x1b[0m\r\n');
        };

        this.socket.onerror = (err) => {
            console.error('WebSocket error:', err);
        };

        // Terminal input → server
        this.terminal.onData((data) => {
            if (this.socket && this.socket.readyState === WebSocket.OPEN) {
                this.socket.send(data);
            }
        });

        // Binary protocol messages (OSC 52 etc.)
        this.terminal.onBinary((data) => {
            if (this.socket && this.socket.readyState === WebSocket.OPEN) {
                const buf = new Uint8Array(data.length);
                for (let i = 0; i < data.length; i++) {
                    buf[i] = data.charCodeAt(i) & 0xff;
                }
                this.socket.send(buf);
            }
        });
    }

    startPing() {
        this.stopPing();
        this.pingInterval = setInterval(() => {
            if (this.socket && this.socket.readyState === WebSocket.OPEN) {
                this.socket.send('ping');
                // If no pong within 10s, connection is dead
                this.pingTimeout = setTimeout(() => {
                    console.warn('Ping timeout — closing connection');
                    this.socket.close();
                }, 10000);
            }
        }, 30000);
    }

    stopPing() {
        if (this.pingInterval) {
            clearInterval(this.pingInterval);
            this.pingInterval = null;
        }
        if (this.pingTimeout) {
            clearTimeout(this.pingTimeout);
            this.pingTimeout = null;
        }
    }

    onPong() {
        if (this.pingTimeout) {
            clearTimeout(this.pingTimeout);
            this.pingTimeout = null;
        }
    }

    clear() {
        if (this.terminal) this.terminal.clear();
    }

    sendText(text) {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(text);
        }
    }

    disconnect() {
        this.stopPing();
        if (this.socket) this.socket.close();
        if (this.terminal) this.terminal.dispose();
    }
}

window.TerminalManager = TerminalManager;
