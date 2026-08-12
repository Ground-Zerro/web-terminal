class KeyboardBar {
    static SEQUENCES = {
        enter: '\r',
        tab: '\t',
        'shift-tab': '\x1b[Z'
    };

    constructor(root, send) {
        this.root = root;
        this.send = send;
        this.menu = root.querySelector('.key-shortcuts-menu');

        this.bind('.key-arrow', (button) => this.send(`\x1b[${button.dataset.arrow}`));
        this.bind('.key-shortcut', (button) => this.press(button));

        const toggle = root.querySelector('.key-shortcuts-toggle');
        if (toggle && this.menu) {
            this.tap(toggle, () => this.menu.classList.toggle('open'));
            document.addEventListener('pointerdown', (event) => {
                if (!event.target.closest('.key-shortcuts-wrap')) this.close();
            });
        }
    }

    bind(selector, action) {
        for (const button of this.root.querySelectorAll(selector)) {
            this.tap(button, () => action(button));
        }
    }

    tap(element, action) {
        element.addEventListener('pointerdown', (event) => {
            event.preventDefault();
            event.stopPropagation();
            action();
        });
    }

    press(button) {
        const { ctrl, key } = button.dataset;
        if (ctrl) {
            this.send(KeyboardBar.control(ctrl));
        } else {
            this.send(KeyboardBar.SEQUENCES[key] || '');
        }
        this.close();
    }

    close() {
        if (this.menu) this.menu.classList.remove('open');
    }

    static control(letter) {
        return String.fromCharCode(letter.toLowerCase().charCodeAt(0) - 96);
    }
}
