export class ReaderToast {
    #toastEl
    #autoHideTimeout = null

    constructor() {
        this.#toastEl = document.getElementById('reader-toast')
        if (!this.#toastEl) return
        this.#toastEl.querySelector('.toast-close')?.addEventListener('click', () => this.#hide())
        const reposition = () => {
            if (!this.#toastEl.hidden) this.#positionToast()
        }
        window.addEventListener('resize', reposition)
        window.visualViewport?.addEventListener('resize', reposition)
        window.visualViewport?.addEventListener('scroll', reposition)
    }

    #positionToast() {
        const viewport = window.visualViewport
        const width = viewport?.width ?? document.documentElement.clientWidth
        const gap = 16
        this.#toastEl.style.left = `${(viewport?.offsetLeft ?? 0) + gap}px`
        this.#toastEl.style.top = `${(viewport?.offsetTop ?? 0) + gap}px`
        this.#toastEl.style.width = `${Math.max(0, width - 2 * gap)}px`
    }

    #hide() {
        clearTimeout(this.#autoHideTimeout)
        this.#toastEl.hidden = true
    }

    show(variant, message) {
        if (!this.#toastEl) return

        try {
            this.#hide()
            this.#toastEl.classList.remove('toast-warning', 'toast-success', 'toast-info')
            this.#toastEl.classList.add(`toast-${variant}`)
            const messageEl = this.#toastEl.querySelector('.toast-message')
            if (messageEl) messageEl.innerHTML = message

            requestAnimationFrame(() => {
                try {
                    this.#positionToast()
                    this.#toastEl.hidden = false
                    if (this.#toastEl.dataset.autoHide === 'true') {
                        this.#autoHideTimeout = setTimeout(() => this.#hide(),
                            parseInt(this.#toastEl.dataset.delay) || 5000)
                    }
                } catch (error) {
                    console.error('Error showing toast:', error)
                }
            })
        } catch (error) {
            console.error('Error preparing toast:', error)
        }
    }
}
