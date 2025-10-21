export class ReaderToast {
    #toastEl
    #autoHideTimeout = null
    #sessionExpiredShown = false
    #notLoggedInShown = false

    constructor() {
        this.#toastEl = document.getElementById('reader-toast')
    }

    #setupCloseButton() {
        if (!this.#toastEl) return
        
        const closeBtn = this.#toastEl.querySelector('.toast-close')
        if (closeBtn && !closeBtn.onclick) {
            closeBtn.onclick = () => {
                clearTimeout(this.#autoHideTimeout)
                this.#toastEl.close()
            }
        }
    }

    show(variant, message) {
        if (!this.#toastEl) return

        try {
            // Set up close button on first use
            this.#setupCloseButton()
            
            // Clear any existing auto-hide timeout
            clearTimeout(this.#autoHideTimeout)
            
            // Close if already open to reset animation
            if (this.#toastEl.open) {
                this.#toastEl.close()
            }
            
            // Remove all variant classes
            this.#toastEl.classList.remove('toast-warning', 'toast-success', 'toast-info')
            
            // Add the appropriate variant class
            this.#toastEl.classList.add(`toast-${variant}`)
            
            // Set the message
            const messageEl = this.#toastEl.querySelector('.toast-message')
            if (messageEl) {
                messageEl.innerHTML = message
            }
            
            // Show the toast using native dialog API
            // Use requestAnimationFrame to ensure the close() completes before show()
            requestAnimationFrame(() => {
                try {
                    this.#toastEl.show()
                    
                    // Auto-hide after delay if data-auto-hide is true
                    const autoHide = this.#toastEl.dataset.autoHide === 'true'
                    const delay = parseInt(this.#toastEl.dataset.delay) || 5000
                    
                    if (autoHide) {
                        this.#autoHideTimeout = setTimeout(() => {
                            this.#toastEl.close()
                        }, delay)
                    }
                } catch (error) {
                    console.error('Error showing toast:', error)
                }
            })
        } catch (error) {
            console.error('Error preparing toast:', error)
        }
    }

    showSessionExpired(message) {
        // Only show the notification once
        if (this.#sessionExpiredShown) return
        this.#sessionExpiredShown = true
        
        this.show('warning', message)
    }

    showPositionUpdated(message) {
        this.show('success', message)
    }

    showNotLoggedIn(message) {
        // Only show the notification once
        if (this.#notLoggedInShown) return
        this.#notLoggedInShown = true
        
        this.show('warning', message)
    }
}

