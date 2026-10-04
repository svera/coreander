export class ReaderAnnotations {
    #view
    #sync
    #translations
    #notify
    #draw
    #slug
    #popup
    #preview
    #actionButton
    #title
    #pending = null
    #saving = false
    #selectionTimeout = null
    #documents = new WeakSet()
    #annotations = new Map()
    #renderErrorShown = false
    #location = null

    constructor({ view, sync, translations, notify, draw, slug }) {
        this.#view = view
        this.#sync = sync
        this.#translations = translations
        this.#notify = notify
        this.#draw = draw
        this.#slug = slug
        if (!sync.isAuthenticated) return

        this.#popup = document.createElement('dialog')
        this.#popup.id = 'annotation-popup'
        this.#popup.setAttribute('aria-labelledby', 'annotation-popup-title')
        this.#title = document.createElement('h3')
        this.#title.id = 'annotation-popup-title'
        this.#preview = document.createElement('blockquote')
        const actions = document.createElement('div')
        this.#actionButton = document.createElement('button')
        this.#actionButton.type = 'button'
        this.#actionButton.addEventListener('click', () => this.#submit())
        const cancel = document.createElement('button')
        cancel.type = 'button'
        cancel.textContent = translations.cancel
        cancel.addEventListener('click', () => this.#dismiss())
        actions.append(this.#actionButton, cancel)
        this.#popup.append(this.#title, this.#preview, actions)
        this.#popup.addEventListener('keydown', event => {
            event.stopPropagation()
            if (event.key === 'Escape') {
                event.preventDefault()
                this.#dismiss()
            }
        })
        this.#popup.addEventListener('cancel', event => {
            event.preventDefault()
            this.#dismiss()
        })
        document.body.append(this.#popup)

        view.addEventListener('load', ({ detail }) => this.#bindDocument(detail))
        view.renderer.addEventListener('relocate', ({ detail: { index, fraction } }) => {
            const previous = this.#location
            this.#location = { index, fraction }
            const moved = previous && (previous.index !== index ||
                Math.abs(previous.fraction - fraction) > 0.000001)
            if (!this.#saving && moved) {
                this.#hide()
            }
        })
        view.addEventListener('create-overlay', ({ detail: { index } }) => {
            // Foliate attaches the overlay after dispatching this event.
            queueMicrotask(() => this.#restore(index))
        })
        view.addEventListener('draw-annotation', ({ detail: { draw, annotation } }) => {
            if (this.#annotations.has(annotation.value)) {
                draw(this.#draw, { color: 'yellow' })
            }
        })
        view.addEventListener('show-annotation', ({ detail: { value, range } }) => {
            if (!this.#sync.isAuthenticated || this.#saving) return
            const annotation = this.#annotations.get(value)
            if (!annotation) return
            const selection = range?.startContainer.ownerDocument.defaultView.getSelection()
            if (selection && !selection.isCollapsed) return
            this.#show({ cfi: value, content: annotation.content }, true)
        })
        window.addEventListener('reader-session-expired', () => this.#hide())
    }
    async load() {
        if (!this.#sync.isAuthenticated) return
        try {
            const response = await fetch(this.#url())
            if (this.#sessionExpired(response)) return
            if (!response.ok) throw new Error(`Loading annotations failed: HTTP ${response.status}`)
            const annotations = await response.json()
            if (!Array.isArray(annotations) || annotations.some(item =>
                typeof item.cfi !== 'string' || typeof item.content !== 'string')) {
                throw new Error('Invalid annotations response')
            }
            for (const annotation of annotations) {
                this.#annotations.set(annotation.cfi, { value: annotation.cfi, content: annotation.content })
            }
        } catch (error) {
            console.error('Error loading text annotations:', error)
            this.#notify('warning', this.#translations.annotations_load_failed)
        }
    }

    #url() {
        return `/documents/${encodeURIComponent(this.#slug)}/annotations`
    }

    #sessionExpired(response) {
        if (response.status !== 401 && response.status !== 403) return false
        this.#sync.isAuthenticated = false
        window.dispatchEvent(new CustomEvent('reader-session-expired'))
        return true
    }

    #bindDocument({ doc, index }) {
        if (!doc || this.#documents.has(doc)) return
        this.#documents.add(doc)
        const schedule = () => {
            clearTimeout(this.#selectionTimeout)
            this.#selectionTimeout = setTimeout(() => this.#selected(doc, index), 0)
        }
        doc.addEventListener('pointerdown', () => this.#hide())
        doc.addEventListener('pointerup', event => {
            if (event.button === 0) schedule()
        })
        doc.addEventListener('touchend', event => {
            if (event.touches.length === 0) schedule()
        })
        doc.addEventListener('pointercancel', () => this.#hide())
        doc.addEventListener('touchcancel', () => this.#hide())
        doc.addEventListener('keyup', event => {
            if (event.key === 'Escape') {
                this.#dismiss()
            }
        })
    }

    #selected(doc, index) {
        if (!this.#sync.isAuthenticated || this.#saving || this.#pending?.remove) return
        const selection = doc.defaultView.getSelection()
        if (!selection?.rangeCount || selection.isCollapsed || !selection.toString().trim()) {
            return
        }
        try {
            const range = selection.getRangeAt(0).cloneRange()
            const cfi = this.#view.getCFI(index, range)
            if (this.#annotations.has(cfi)) {
                this.#hide()
                return
            }
            this.#show({ cfi, content: selection.toString() })
        } catch (error) {
            this.#hide()
            console.error('Error preparing text annotation:', error)
            this.#notify('warning', this.#translations.annotation_save_failed)
        }
    }

    #show(annotation, remove = false) {
        clearTimeout(this.#selectionTimeout)
        this.#pending = { ...annotation, remove }
        this.#preview.textContent = annotation.content
        this.#title.textContent = remove
            ? this.#translations.remove_annotation : this.#translations.save_annotation
        this.#actionButton.textContent = remove
            ? this.#translations.remove_annotation : this.#translations.save
        if (!this.#popup.open) {
            if (remove) this.#popup.show()
            // Opening without show() preserves focus and the book's native selection.
            else this.#popup.open = true
        }
    }

    #hide() {
        clearTimeout(this.#selectionTimeout)
        this.#pending = null
        this.#popup?.close()
    }

    #dismiss() {
        this.#hide()
        this.#view.deselect()
        this.#view.focus()
    }

    async #restore(index) {
        if (!this.#sync.isAuthenticated) return
        for (const annotation of this.#annotations.values()) {
            try {
                const target = await this.#view.resolveNavigation(annotation.value)
                if (target.index === index && this.#annotations.has(annotation.value)) {
                    await this.#view.addAnnotation(annotation)
                }
            } catch (error) {
                console.error('Error displaying saved text annotation:', error)
                if (!this.#renderErrorShown) {
                    this.#renderErrorShown = true
                    this.#notify('warning', this.#translations.annotations_display_failed)
                }
            }
        }
    }

    async #submit() {
        if (!this.#pending || this.#saving || !this.#sync.isAuthenticated) return
        const { cfi, content, remove } = this.#pending
        const annotation = { value: cfi, content }
        this.#saving = true
        this.#actionButton.disabled = true
        try {
            const response = await fetch(this.#url(), {
                method: remove ? 'DELETE' : 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(remove ? { cfi } : { cfi, content }),
            })
            if (this.#sessionExpired(response)) return
            if (!response.ok) throw new Error(`Annotation ${remove ? 'deletion' : 'save'} failed: HTTP ${response.status}`)
            if (remove) this.#annotations.delete(cfi)
            else this.#annotations.set(cfi, annotation)
            this.#dismiss()
        } catch (error) {
            console.error('Error updating text annotation:', error)
            this.#notify('warning', remove
                ? this.#translations.annotation_remove_failed : this.#translations.annotation_save_failed)
            return
        } finally {
            this.#saving = false
            this.#actionButton.disabled = false
        }
        this.#notify('success', remove
            ? this.#translations.annotation_removed : this.#translations.annotation_saved)
        try {
            if (remove) await this.#view.deleteAnnotation(annotation)
            else {
                const { index } = await this.#view.resolveNavigation(cfi)
                await this.#restore(index)
            }
        } catch (error) {
            console.error('Error updating annotation overlay:', error)
            this.#notify('warning', this.#translations.annotations_display_failed)
        }
    }
}
