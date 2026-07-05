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

        // Mobile modifier keys state (unused now, kept for compat)
        this.lockedMods = { shift: false, ctrl: false, alt: false };

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

        // Ctrl+Shift+V — let browser handle paste natively (paste event on textarea catches it)
        this.terminal.attachCustomKeyEventHandler((event) => {
            if (event.ctrlKey && event.shiftKey && event.key === 'V') {
                return false; // let browser fire paste event on textarea
            }

            // Apply locked modifiers from mobile toolbar
            if (event.type === 'keydown' || event.type === 'keypress') {
                if (this.lockedMods.ctrl && !event.ctrlKey) {
                    event.preventDefault();
                    this._sendWithMods(event.key, true, this.lockedMods.shift, this.lockedMods.alt);
                    return false;
                }
                if (this.lockedMods.alt && !event.altKey) {
                    event.preventDefault();
                    this._sendWithMods(event.key, this.lockedMods.ctrl, this.lockedMods.shift, true);
                    return false;
                }
            }
            return true;
        });

        // Right-click: show context menu (browser "Paste" option triggers paste event above)
        // Clipboard API is blocked on HTTP pages, so we can't read clipboard from JS directly.
        // The browser's native "Paste" menu item fires a paste DOM event with clipboardData.
        this.terminal.element.addEventListener('contextmenu', (e) => {
            // Allow native context menu so user can click "Paste"
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
        this.setupMobileKeys();
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

    _sendWithMods(key, ctrl, shift, alt) {
        if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;
        let code = '';
        const k = key.length === 1 ? key.toLowerCase() : key;

        if (ctrl) {
            if (k >= 'a' && k <= 'z') {
                code = String.fromCharCode(k.charCodeAt(0) - 96);
            } else if (k === '[') code = '\x1b';
            else if (k === '\\') code = '\x1c';
            else if (k === ']') code = '\x1d';
            else if (k === '^') code = '\x1e';
            else if (k === '_') code = '\x1f';
            else if (k === '?') code = '\x7f';
            else code = key;
        } else {
            code = key;
        }

        if (alt) code = '\x1b' + code;
        if (shift && !ctrl) code = key.length === 1 ? key : code;

        this.socket.send(code);
    }

    setupMobileKeys() {
        const keysBar = document.getElementById('terminal-keys');
        if (!keysBar) return;

        // Arrow key buttons
        keysBar.querySelectorAll('.key-arrow').forEach(btn => {
            const handler = (e) => {
                e.preventDefault();
                e.stopPropagation();
                const dir = btn.dataset.arrow;
                this.sendText('\x1b[' + dir);
            };
            btn.addEventListener('touchstart', handler, { passive: false });
            btn.addEventListener('click', handler);
        });

        // Shortcuts dropdown
        const toggle = document.getElementById('key-shortcuts-toggle');
        const menu = document.getElementById('key-shortcuts-menu');
        if (toggle && menu) {
            const toggleHandler = (e) => {
                e.preventDefault();
                e.stopPropagation();
                menu.classList.toggle('open');
            };
            toggle.addEventListener('touchstart', toggleHandler, { passive: false });
            toggle.addEventListener('click', toggleHandler);
            document.addEventListener('touchstart', (e) => {
                if (!e.target.closest('.key-shortcuts-wrap')) menu.classList.remove('open');
            });
            document.addEventListener('click', () => menu.classList.remove('open'));
        }

        // Shortcut buttons
        keysBar.querySelectorAll('.key-shortcut').forEach(btn => {
            const handler = (e) => {
                e.preventDefault();
                e.stopPropagation();
                const seq = btn.dataset.seq;
                const useCtrl = btn.dataset.ctrl === '1';
                if (useCtrl) {
                    this._sendWithMods(seq, true, false, false);
                } else if (seq === 'enter') {
                    this.sendText('\r');
                } else if (seq === '\t') {
                    this.sendText('\t');
                } else {
                    this.sendText('\x1b' + seq);
                }
                menu.classList.remove('open');
            };
            btn.addEventListener('touchstart', handler, { passive: false });
            btn.addEventListener('click', handler);
        });
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
