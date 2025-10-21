export class ReaderSync {
    #reader
    #updatePositionTimeout = null
    #syncFromServerTimeout = null
    #pendingPosition = null
    #pendingSlug = null
    #isAuthenticated = false
    #sessionExpiredNotificationShown = false
    #notLoggedInNotificationShown = false
    #toastAutoHideTimeout = null

    constructor(reader, isAuthenticated) {
        this.#reader = reader
        this.#isAuthenticated = isAuthenticated
        
        // Show notification if not logged in
        if (!isAuthenticated) {
            this.showNotLoggedInNotification()
        }
    }

    #setupToastCloseButton(toastEl) {
        const closeBtn = toastEl.querySelector('.toast-close')
        if (closeBtn && !closeBtn.onclick) {
            closeBtn.onclick = () => {
                clearTimeout(this.#toastAutoHideTimeout)
                toastEl.close()
            }
        }
    }

    #showToast(toastEl, variant, message) {
        try {
            // Set up close button on first use
            this.#setupToastCloseButton(toastEl)
            
            // Clear any existing auto-hide timeout
            clearTimeout(this.#toastAutoHideTimeout)
            
            // Close if already open to reset animation
            if (toastEl.open) {
                toastEl.close()
            }
            
            // Remove all variant classes
            toastEl.classList.remove('toast-warning', 'toast-success', 'toast-info')
            
            // Add the appropriate variant class
            toastEl.classList.add(`toast-${variant}`)
            
            // Set the message
            const messageEl = toastEl.querySelector('.toast-message')
            if (messageEl) {
                messageEl.innerHTML = message
            }
            
            // Show the toast using native dialog API
            // Use requestAnimationFrame to ensure the close() completes before show()
            requestAnimationFrame(() => {
                try {
                    toastEl.show()
                    
                    // Auto-hide after delay if data-auto-hide is true
                    const autoHide = toastEl.dataset.autoHide === 'true'
                    const delay = parseInt(toastEl.dataset.delay) || 5000
                    
                    if (autoHide) {
                        this.#toastAutoHideTimeout = setTimeout(() => {
                            toastEl.close()
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

    get isAuthenticated() {
        return this.#isAuthenticated
    }

    set isAuthenticated(value) {
        this.#isAuthenticated = value
    }

    getLocalPosition(slug) {
        const storage = window.localStorage
        const saved = storage.getItem(slug)
        
        if (!saved) {
            return { position: null, updated: null }
        }
        
        try {
            const parsed = JSON.parse(saved)
            return {
                position: parsed.position || null,
                updated: parsed.updated || null
            }
        } catch {
            // Old format: plain string (position only)
            return { position: saved, updated: null }
        }
    }

    async getServerPosition(slug) {
        try {
            const response = await fetch(`/documents/${slug}/position`, {
                method: 'GET',
                headers: {
                    'Content-Type': 'application/json',
                }
            })
            
            if (response.status === 403) {
                // Session expired, mark as unauthenticated and show notification
                this.#isAuthenticated = false
                this.showSessionExpiredNotification()
                return { position: '', updated: '' }
            }
            
            if (response.ok) {
                return await response.json()
            }
            
            return { position: '', updated: '' }
        } catch (error) {
            console.error('Error fetching position from server:', error)
            return { position: '', updated: '' }
        }
    }

    async syncPositionToServer(slug, position) {
        try {
            const response = await fetch(`/documents/${slug}/position`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ position })
            })
            
            if (response.status === 403) {
                // Session expired, mark as unauthenticated and show notification
                this.#isAuthenticated = false
                this.showSessionExpiredNotification()
                return
            }
            
            if (!response.ok && response.status !== 204) {
                console.error('Failed to sync position to server:', response.statusText)
            }
        } catch (error) {
            console.error('Error syncing position to server:', error)
        }
    }

    flushPositionUpdate() {
        // If there's a pending position update, send it immediately
        if (this.#pendingPosition && this.#pendingSlug) {
            clearTimeout(this.#updatePositionTimeout)
            this.syncPositionToServer(this.#pendingSlug, this.#pendingPosition)
            this.#pendingPosition = null
            this.#pendingSlug = null
        }
    }

    debouncedSyncPositionFromServer() {
        // Debounce sync calls to prevent redundant requests when multiple events fire
        clearTimeout(this.#syncFromServerTimeout)
        this.#syncFromServerTimeout = setTimeout(() => {
            this.syncPositionFromServer()
        }, 100) // Wait 100ms for other events
    }

    async syncPositionFromServer() {
        // Only sync if authenticated and view is initialized
        if (!this.#isAuthenticated || !this.#reader.view) {
            return
        }
        
        const slug = document.getElementById('slug')?.value
        if (!slug) {
            return
        }
        
        const storage = window.localStorage
        const localData = this.getLocalPosition(slug)
        const serverData = await this.getServerPosition(slug)
        
        // If server has a position and it's newer than local, update
        if (serverData.position && serverData.updated) {
            if (!localData.updated || new Date(serverData.updated) > new Date(localData.updated)) {
                // Check if position actually changed
                const positionChanged = localData.position !== serverData.position
                
                // Server position is newer, update localStorage
                storage.setItem(slug, JSON.stringify({
                    position: serverData.position,
                    updated: serverData.updated
                }))
                
                // Navigate to the new position
                try {
                    await this.#reader.view.goTo(serverData.position)
                    // Show notification only if position actually changed
                    if (positionChanged) {
                        this.showPositionUpdatedNotification()
                    }
                } catch (error) {
                    console.error('Error navigating to synced position:', error)
                }
            }
        }
    }

    schedulePositionUpdate(slug, position) {
        // Save pending position for immediate flush if needed
        this.#pendingPosition = position
        this.#pendingSlug = slug
        
        clearTimeout(this.#updatePositionTimeout)
        this.#updatePositionTimeout = setTimeout(() => {
            this.syncPositionToServer(slug, position)
            this.#pendingPosition = null
            this.#pendingSlug = null
        }, 1000) // Wait 1 second after last position change
    }

    showSessionExpiredNotification() {
        // Only show the notification once
        if (this.#sessionExpiredNotificationShown) return
        this.#sessionExpiredNotificationShown = true
        
        const toastEl = document.getElementById('reader-toast')
        if (!toastEl) return
        
        this.#showToast(toastEl, 'warning', this.#reader.translations.session_expired_reading)
    }

    showPositionUpdatedNotification() {
        const toastEl = document.getElementById('reader-toast')
        if (!toastEl) return
        
        this.#showToast(toastEl, 'success', this.#reader.translations.position_updated_from_server)
    }

    showNotLoggedInNotification() {
        // Only show the notification once
        if (this.#notLoggedInNotificationShown) return
        this.#notLoggedInNotificationShown = true
        
        const toastEl = document.getElementById('reader-toast')
        if (!toastEl) return
        
        this.#showToast(toastEl, 'warning', this.#reader.translations.not_logged_in_reading)
    }
}

