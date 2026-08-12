class PerfWidget {
    static HISTORY = 30;
    static POLL_INTERVAL = 1000;
    static BACKGROUND = '#1a1a1a';
    static LINE = '#4a9eff';
    static FILL_TOP = 'rgba(74, 158, 255, 0.6)';
    static FILL_BOTTOM = 'rgba(74, 158, 255, 0.1)';

    constructor(api, elements) {
        this.api = api;
        this.elements = elements;
        this.context = elements.canvas.getContext('2d');
        this.history = new Float64Array(PerfWidget.HISTORY);
        this.head = 0;
        this.timer = null;
    }

    start() {
        this.stop();
        this.poll();
        this.timer = setInterval(() => this.poll(), PerfWidget.POLL_INTERVAL);
    }

    stop() {
        clearInterval(this.timer);
        this.timer = null;
    }

    async poll() {
        let metrics;
        try {
            metrics = await this.api.get('api/metrics');
        } catch (error) {
            return;
        }

        this.history[this.head] = metrics.cpu.usage_percent;
        this.head = (this.head + 1) % PerfWidget.HISTORY;

        this.draw();
        this.elements.cpu.textContent = `CPU ${metrics.cpu.usage_percent.toFixed(1)}%`;
        this.elements.memory.textContent =
            `MEM ${formatBytes(metrics.memory.used)}/${formatBytes(metrics.memory.total)}`;
        this.elements.network.textContent =
            `↓${formatBytes(metrics.network.rx_bytes_per_sec, '/s')} ` +
            `↑${formatBytes(metrics.network.tx_bytes_per_sec, '/s')}`;
    }

    sampleAt(index) {
        return this.history[(this.head + index) % PerfWidget.HISTORY];
    }

    draw() {
        const ctx = this.context;
        const { width, height } = this.elements.canvas;
        const step = width / (PerfWidget.HISTORY - 1);

        ctx.fillStyle = PerfWidget.BACKGROUND;
        ctx.fillRect(0, 0, width, height);

        ctx.beginPath();
        for (let i = 0; i < PerfWidget.HISTORY; i++) {
            const x = i * step;
            const y = height - (this.sampleAt(i) / 100) * height;
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        }

        ctx.strokeStyle = PerfWidget.LINE;
        ctx.lineWidth = 1.5;
        ctx.stroke();

        ctx.lineTo(width, height);
        ctx.lineTo(0, height);
        ctx.closePath();

        const gradient = ctx.createLinearGradient(0, 0, 0, height);
        gradient.addColorStop(0, PerfWidget.FILL_TOP);
        gradient.addColorStop(1, PerfWidget.FILL_BOTTOM);
        ctx.fillStyle = gradient;
        ctx.fill();
    }
}
