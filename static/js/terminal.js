class TerminalManager {
    static THEME = {
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
    };

    static PING_INTERVAL = 30000;
    static PONG_TIMEOUT = 10000;
    static FIT_DEBOUNCE = 100;
    static INITIAL_FIT_DELAY = 200;

    constructor(session) {
        this.session = session;
        this.terminal = null;
        this.fitAddon = null;
        this.socket = null;
        this.decoder = new TextDecoder('utf-8');
        this.pingInterval = null;
        this.pongTimeout = null;
        this.fitTimeout = null;
        this.observer = null;
    }

    init(container, keysBar) {
        this.terminal = new Terminal({
            cursorBlink: true,
            cursorStyle: 'bar',
            fontSize: 14,
            fontFamily: '"Cascadia Code", Menlo, Monaco, "Courier New", monospace',
            allowProposedApi: true,
            scrollback: 10000,
            tabStopWidth: 8,
            theme: TerminalManager.THEME
        });

        this.fitAddon = new FitAddon.FitAddon();
        this.terminal.loadAddon(this.fitAddon);
        this.terminal.loadAddon(new Unicode11Addon.Unicode11Addon());
        this.terminal.unicode.activeVersion = '11';
        this.terminal.loadAddon(new WebLinksAddon.WebLinksAddon());

        this.terminal.open(container);
        setTimeout(() => this.fit(), TerminalManager.INITIAL_FIT_DELAY);

        this.terminal.onSelectionChange(() => {
            const selection = this.terminal.getSelection();
            if (selection) navigator.clipboard.writeText(selection).catch(() => {});
        });

        this.terminal.attachCustomKeyEventHandler(
            (event) => !(event.ctrlKey && event.shiftKey && event.key === 'V')
        );

        window.addEventListener('resize', () => this.scheduleFit(TerminalManager.FIT_DEBOUNCE));

        this.observer = new MutationObserver(() => {
            if (this.terminal.element?.offsetHeight > 0) this.scheduleFit(50);
        });
        this.observer.observe(container, { attributes: true, attributeFilter: ['style', 'class'] });

        if (keysBar) new KeyboardBar(keysBar, (text) => this.send(text));

        this.connect();
    }

    scheduleFit(delay) {
        clearTimeout(this.fitTimeout);
        this.fitTimeout = setTimeout(() => this.fit(), delay);
    }

    fit() {
        if (!this.fitAddon) return;
        this.fitAddon.fit();
        this.sendResize();
    }

    sendResize() {
        const dimensions = this.fitAddon.proposeDimensions();
        const cols = dimensions?.cols > 0 ? dimensions.cols : this.terminal.cols;
        const rows = dimensions?.rows > 0 ? dimensions.rows : this.terminal.rows;
        this.send(JSON.stringify({ type: 'resize', data: { cols, rows } }));
    }

    connect() {
        const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const token = encodeURIComponent(this.session.token);
        this.socket = new WebSocket(
            `${scheme}//${location.host}${location.pathname}api/terminal?token=${token}`
        );
        this.socket.binaryType = 'arraybuffer';

        this.socket.onopen = () => {
            this.scheduleFit(TerminalManager.FIT_DEBOUNCE);
            this.startPing();
        };

        this.socket.onmessage = (event) => {
            if (typeof event.data === 'string') {
                if (event.data === 'pong') this.clearPongTimer();
                else this.terminal.write(event.data);
                return;
            }
            this.terminal.write(this.decoder.decode(event.data, { stream: true }));
        };

        this.socket.onclose = () => {
            this.stopPing();
            this.terminal.write('\r\n\x1b[31mConnection lost. Please refresh.\x1b[0m\r\n');
        };

        this.socket.onerror = (error) => console.error('WebSocket error:', error);

        this.terminal.onData((data) => this.send(data));

        this.terminal.onBinary((data) => {
            const bytes = new Uint8Array(data.length);
            for (let i = 0; i < data.length; i++) bytes[i] = data.charCodeAt(i) & 0xff;
            this.send(bytes);
        });
    }

    send(payload) {
        if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(payload);
    }

    startPing() {
        this.stopPing();
        this.pingInterval = setInterval(() => {
            if (this.socket?.readyState !== WebSocket.OPEN) return;
            this.socket.send('ping');
            this.pongTimeout = setTimeout(() => {
                console.warn('Ping timeout — closing connection');
                this.socket.close();
            }, TerminalManager.PONG_TIMEOUT);
        }, TerminalManager.PING_INTERVAL);
    }

    stopPing() {
        clearInterval(this.pingInterval);
        this.pingInterval = null;
        this.clearPongTimer();
    }

    clearPongTimer() {
        clearTimeout(this.pongTimeout);
        this.pongTimeout = null;
    }

    clear() {
        this.terminal?.clear();
    }

    disconnect() {
        this.stopPing();
        clearTimeout(this.fitTimeout);
        this.observer?.disconnect();
        this.socket?.close();
        this.terminal?.dispose();
    }
}
