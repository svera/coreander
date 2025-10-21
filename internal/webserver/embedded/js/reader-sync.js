export class ReaderSync {
    #updatePositionTimeout = null
    #syncFromServerTimeout = null
    #pendingPosition = null
    #pendingSlug = null
    #isAuthenticated = false
    #view = null

    constructor(isAuthenticated) {
        this.#isAuthenticated = isAuthenticated
    }

    setView(view) {
        this.#view = view
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
                // Session expired, mark as unauthenticated and dispatch event
                this.#isAuthenticated = false
                window.dispatchEvent(new CustomEvent('reader-session-expired'))
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
                // Session expired, mark as unauthenticated and dispatch event
                this.#isAuthenticated = false
                window.dispatchEvent(new CustomEvent('reader-session-expired'))
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
        if (!this.#isAuthenticated || !this.#view) {
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
                    await this.#view.goTo(serverData.position)
                    // Dispatch event only if position actually changed
                    if (positionChanged) {
                        window.dispatchEvent(new CustomEvent('reader-position-updated'))
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
}

