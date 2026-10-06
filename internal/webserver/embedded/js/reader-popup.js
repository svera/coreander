export class ReaderPopup {
    #element
    #mode
    #onDismiss
    #onPosition
    #focusTarget
    #focusTimeout = null
    closeButton

    constructor({ element, mode, closeLabel, closeButton, onDismiss, onPosition, focusTarget }) {
        if (mode !== 'modal' && mode !== 'anchored') throw new Error(`Invalid reader popup mode: ${mode}`)
        this.#element = element
        this.#mode = mode
        this.#onDismiss = onDismiss
        this.#onPosition = onPosition
        this.#focusTarget = focusTarget
        element.className = `${element.className ?? ''} reader-popup reader-popup--${mode}`.trim()
        this.closeButton = closeButton ?? document.createElement('button')
        this.closeButton.type = 'button'
        this.closeButton.className = 'reader-popup-close'
        this.closeButton.textContent = '\u00d7'
        this.closeButton.setAttribute('aria-label', closeLabel)
        this.closeButton.title = closeLabel
        this.closeButton.addEventListener('click', () => this.dismiss())
        element.addEventListener('keydown', event => {
            event.stopPropagation()
            if (event.key === 'Escape' && !event.defaultPrevented) {
                event.preventDefault()
                this.dismiss()
            }
        })
        if (mode === 'modal') {
            element.addEventListener('cancel', event => {
                event.preventDefault()
                this.dismiss()
            })
            element.addEventListener('click', event => {
                if (event.target === element) this.dismiss()
            })
            element.addEventListener('close', () => this.#clearFocusTimeout())
        } else {
            element.hidden = true
            element.setAttribute('role', 'dialog')
            document.addEventListener('keydown', event => {
                if (this.isOpen && !event.defaultPrevented && event.key === 'Escape' &&
                    !document.querySelector?.('dialog[open]')) {
                    event.preventDefault()
                    event.stopPropagation()
                    this.dismiss()
                }
            })
            const reposition = () => {
                if (this.isOpen) this.#onPosition?.()
            }
            window.addEventListener('resize', reposition)
            window.visualViewport?.addEventListener('resize', reposition)
            window.visualViewport?.addEventListener('scroll', reposition)
        }
    }

    get isOpen() {
        return this.#mode === 'modal' ? this.#element.open : !this.#element.hidden
    }

    show({ label, compact = false } = {}) {
        if (label !== undefined) this.#element.setAttribute('aria-label', label)
        this.#element.setAttribute('data-compact', String(compact))
        if (this.#mode === 'modal') {
            if (!this.isOpen) this.#element.showModal()
            this.#clearFocusTimeout()
            this.#focusTimeout = setTimeout(() => {
                this.#focusTimeout = null
                if (this.isOpen) this.#focusTarget?.focus()
            }, 0)
        } else {
            if (!this.isOpen) {
                this.#element.style.visibility = 'hidden'
                this.#element.hidden = false
            }
            this.#onPosition?.()
            this.#element.style.removeProperty('visibility')
        }
    }

    hide() {
        this.#clearFocusTimeout()
        if (this.#mode === 'modal') {
            if (this.isOpen) this.#element.close()
        } else this.#element.hidden = true
    }

    dismiss() {
        if (!this.isOpen) return
        this.hide()
        this.#onDismiss?.()
    }

    #clearFocusTimeout() {
        clearTimeout(this.#focusTimeout)
        this.#focusTimeout = null
    }
}
