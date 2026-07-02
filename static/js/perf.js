// Performance Widget
class PerfWidget {
    constructor() {
        this.cpuHistory = new Array(30).fill(0);
        this.canvas = null;
        this.ctx = null;
        this.pollInterval = null;
        this.token = null;
    }

    start(token) {
        this.token = token;
        this.canvas = document.getElementById('cpu-graph');
        this.ctx = this.canvas.getContext('2d');
        this.poll();
        this.pollInterval = setInterval(() => this.poll(), 1000);
    }

    stop() {
        if (this.pollInterval) {
            clearInterval(this.pollInterval);
            this.pollInterval = null;
        }
    }

    async poll() {
        try {
            const resp = await fetch('api/metrics', {
                headers: { 'Authorization': this.token }
            });
            if (!resp.ok) return;
            const data = await resp.json();

            // CPU
            this.cpuHistory.push(data.cpu.usage_percent);
            if (this.cpuHistory.length > 30) this.cpuHistory.shift();
            this.drawCPU(data.cpu.usage_percent);

            // Memory
            const memUsed = this.formatBytes(data.memory.used);
            const memTotal = this.formatBytes(data.memory.total);
            document.getElementById('mem-text').textContent = `MEM ${memUsed}/${memTotal}`;

            // Network
            const rx = this.formatSpeed(data.network.rx_bytes_per_sec);
            const tx = this.formatSpeed(data.network.tx_bytes_per_sec);
            document.getElementById('net-text').innerHTML = `&darr;${rx} &uarr;${tx}`;
        } catch (e) {
            // silent — metrics endpoint may not be available yet
        }
    }

    drawCPU(currentPercent) {
        const ctx = this.ctx;
        const w = this.canvas.width;
        const h = this.canvas.height;

        ctx.clearRect(0, 0, w, h);

        // Background
        ctx.fillStyle = '#1a1a1a';
        ctx.fillRect(0, 0, w, h);

        // Draw history as filled area
        const step = w / (this.cpuHistory.length - 1);
        ctx.beginPath();
        ctx.moveTo(0, h);

        for (let i = 0; i < this.cpuHistory.length; i++) {
            const x = i * step;
            const y = h - (this.cpuHistory[i] / 100) * h;
            if (i === 0) {
                ctx.lineTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        }

        ctx.lineTo(w, h);
        ctx.closePath();

        // Gradient fill
        const grad = ctx.createLinearGradient(0, 0, 0, h);
        grad.addColorStop(0, 'rgba(74, 158, 255, 0.6)');
        grad.addColorStop(1, 'rgba(74, 158, 255, 0.1)');
        ctx.fillStyle = grad;
        ctx.fill();

        // Top line
        ctx.beginPath();
        for (let i = 0; i < this.cpuHistory.length; i++) {
            const x = i * step;
            const y = h - (this.cpuHistory[i] / 100) * h;
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        }
        ctx.strokeStyle = '#4a9eff';
        ctx.lineWidth = 1.5;
        ctx.stroke();

        // Update text
        document.getElementById('cpu-text').textContent = `CPU ${currentPercent.toFixed(1)}%`;
    }

    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        const val = (bytes / Math.pow(k, i)).toFixed(i > 1 ? 1 : 0);
        return val + sizes[i];
    }

    formatSpeed(bytesPerSec) {
        if (bytesPerSec === 0) return '0 B/s';
        const k = 1024;
        const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
        const i = Math.floor(Math.log(bytesPerSec) / Math.log(k));
        const val = (bytesPerSec / Math.pow(k, i)).toFixed(i > 0 ? 1 : 0);
        return val + ' ' + sizes[i];
    }
}

window.perfWidget = new PerfWidget();
