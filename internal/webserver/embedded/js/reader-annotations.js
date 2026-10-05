import { compare } from './foliate-js/epubcfi.js'

const createButton = (label, onClick) => {
    const button = document.createElement('button')
    button.type = 'button'
    button.textContent = label
    button.addEventListener('click', onClick)
    return button
}

export class ReaderAnnotations {
    #view
    #sync
    #translations
    #notify
    #draw
    #url
    #popup
    #anchorRange = null
    #actionButton
    #pending = null
    #saving = false
    #selectionTimeout = null
    #documents = new WeakSet()
    #annotations = new Map()
    #renderErrorShown = false
    #location = null
    #list
    #onNavigate

    constructor({ view, sync, translations, notify, draw, slug, onNavigate }) {
        this.#view = view
        this.#sync = sync
        this.#translations = translations
        this.#notify = notify
        this.#draw = draw
        this.#url = `/documents/${encodeURIComponent(slug)}/annotations`
        this.#list = document.getElementById('annotations-list')
        this.#onNavigate = onNavigate
        if (!sync.isAuthenticated) return

        this.#popup = document.createElement('dialog')
        this.#popup.id = 'annotation-popup'
        this.#actionButton = createButton('', () => this.#submit())
        const cancel = createButton(translations.cancel, () => this.#dismiss())
        this.#popup.append(this.#actionButton, cancel)
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
            this.#show(annotation, range, true)
        })
        window.addEventListener('reader-session-expired', () => {
            this.#hide()
            this.#list?.replaceChildren()
        })
        window.addEventListener('resize', () => {
            if (this.#popup.open) this.#positionPopup()
        })
    }
    async load() {
        if (!this.#sync.isAuthenticated) return
        try {
            const response = await fetch(this.#url)
            if (this.#sessionExpired(response)) return
            if (!response.ok) throw new Error(`Loading annotations failed: HTTP ${response.status}`)
            const annotations = await response.json()
            if (!Array.isArray(annotations) || annotations.some(item =>
                typeof item.cfi !== 'string' || typeof item.content !== 'string')) {
                throw new Error('Invalid annotations response')
            }
            for (const { cfi: value, content } of annotations) {
                this.#annotations.set(value, { value, content })
            }
            this.#renderList()
        } catch (error) {
            console.error('Error loading text annotations:', error)
            this.#notify('warning', this.#translations.annotations_load_failed)
            if (this.#list) this.#list.textContent = this.#translations.annotations_load_failed
        }
    }

    #renderList(focusIndex = null) {
        if (!this.#list) return
        this.#list.replaceChildren()
        if (!this.#annotations.size) {
            const message = document.createElement('p')
            message.textContent = this.#translations.no_annotations
            this.#list.append(message)
            if (focusIndex !== null) {
                document.getElementById('annotations-side-bar-close')?.focus({ preventScroll: true })
            }
            return
        }
        const list = document.createElement('ol')
        const annotations = Array.from(this.#annotations.values())
            .sort((a, b) => compare(a.value, b.value))
        for (const [index, annotation] of annotations.entries()) {
            const item = document.createElement('li')
            const button = createButton('', async () => {
                if (!this.#sync.isAuthenticated || this.#saving) return
                button.disabled = true
                try {
                    const target = await this.#view.goTo(annotation.value)
                    if (!target) throw new Error('Annotation location could not be opened')
                    this.#onNavigate?.()
                } catch (error) {
                    console.error('Error navigating to annotation:', error)
                    this.#notify('warning', this.#translations.annotations_display_failed)
                } finally {
                    button.disabled = false
                }
            })
            const preview = document.createElement('span')
            preview.textContent = annotation.content
            button.append(preview)
            const remove = createButton('\u00d7', () => this.#submit({
                annotation: { ...annotation, remove: true },
                button: remove,
                listIndex: index,
            }))
            remove.className = 'annotation-remove'
            remove.setAttribute('aria-label', this.#translations.remove_annotation)
            remove.title = this.#translations.remove_annotation
            item.append(button, remove)
            list.append(item)
        }
        this.#list.append(list)
        if (focusIndex !== null) {
            list.children[Math.min(focusIndex, annotations.length - 1)].lastElementChild.focus({ preventScroll: true })
        }
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
        let selecting = false
        const schedule = (delay = 0) => {
            clearTimeout(this.#selectionTimeout)
            this.#selectionTimeout = setTimeout(() => {
                if (!selecting) this.#selected(doc, index)
            }, delay)
        }
        const startSelection = () => {
            selecting = true
            this.#hide()
        }
        const cancelPointer = () => {
            selecting = false
            // Native long-press selection can cancel the browser's pointer stream.
            schedule(150)
        }
        doc.addEventListener('selectionchange', () => schedule(150))
        doc.addEventListener('pointerdown', startSelection)
        doc.addEventListener('pointerup', event => {
            selecting = false
            if (event.button === 0) schedule()
        })
        doc.addEventListener('touchstart', startSelection)
        doc.addEventListener('touchend', event => {
            if (event.touches.length === 0) {
                selecting = false
                schedule()
            }
        })
        doc.addEventListener('pointercancel', cancelPointer)
        doc.addEventListener('touchcancel', cancelPointer)
        doc.addEventListener('keyup', event => {
            if (event.key === 'Escape') {
                this.#dismiss()
            }
        })
    }

    #selected(doc, index) {
        if (!this.#sync.isAuthenticated || this.#saving || this.#pending?.remove) return
        const selection = doc.defaultView.getSelection()
        const content = selection?.toString() ?? ''
        if (!selection?.rangeCount || selection.isCollapsed || !content.trim()) {
            this.#hide()
            return
        }
        try {
            const range = selection.getRangeAt(0).cloneRange()
            const value = this.#view.getCFI(index, range)
            if (this.#annotations.has(value)) {
                this.#hide()
                return
            }
            this.#show({ value, content }, range)
        } catch (error) {
            this.#hide()
            console.error('Error preparing text annotation:', error)
            this.#notify('warning', this.#translations.annotation_save_failed)
        }
    }

    #show(annotation, range, remove = false) {
        clearTimeout(this.#selectionTimeout)
        this.#pending = { ...annotation, remove }
        this.#anchorRange = range
        this.#actionButton.textContent = remove
            ? this.#translations.remove_annotation : this.#translations.save_annotation
        this.#popup.setAttribute('aria-label', this.#actionButton.textContent)
        if (!this.#popup.open) {
            this.#popup.style.visibility = 'hidden'
            if (remove) this.#popup.show()
            // Opening without show() preserves focus and the book's native selection.
            else this.#popup.open = true
        }
        this.#positionPopup()
        this.#popup.style.removeProperty('visibility')
    }

    #positionPopup() {
        if (!this.#anchorRange) {
            this.#hide()
            return
        }
        const doc = this.#anchorRange.startContainer.ownerDocument
        const frame = doc.defaultView.frameElement
        const frameRect = frame?.getBoundingClientRect()
        const scaleX = frame ? frameRect.width / frame.offsetWidth : 1
        const scaleY = frame ? frameRect.height / frame.offsetHeight : 1
        const offsetX = frame ? frameRect.left + frame.clientLeft * scaleX : 0
        const offsetY = frame ? frameRect.top + frame.clientTop * scaleY : 0
        const viewportWidth = document.documentElement.clientWidth
        const viewportHeight = document.documentElement.clientHeight
        const gap = 8
        const minY = Math.max(gap, document.getElementById('header-bar')?.getBoundingClientRect().bottom ?? gap)
        const maxY = Math.min(viewportHeight - gap,
            document.getElementById('nav-bar')?.getBoundingClientRect().top ?? viewportHeight - gap)
        const rects = Array.from(this.#anchorRange.getClientRects(), rect => ({
            left: offsetX + rect.left * scaleX,
            right: offsetX + rect.right * scaleX,
            top: offsetY + rect.top * scaleY,
            bottom: offsetY + rect.bottom * scaleY,
        })).filter(rect => rect.right > gap && rect.left < viewportWidth - gap &&
            rect.bottom > minY && rect.top < maxY)
        if (!rects.length) {
            this.#hide()
            return
        }
        const top = Math.max(minY, Math.min(...rects.map(rect => rect.top)))
        const bottom = Math.min(maxY, Math.max(...rects.map(rect => rect.bottom)))
        const left = Math.max(0, Math.min(...rects.map(rect => rect.left)))
        const right = Math.min(viewportWidth, Math.max(...rects.map(rect => rect.right)))
        const { width, height } = this.#popup.getBoundingClientRect()
        const below = maxY - bottom - gap
        const above = top - minY - gap
        const y = below >= height || below >= above ? bottom + gap : top - height - gap
        this.#popup.style.left = `${Math.max(gap, Math.min((left + right - width) / 2, viewportWidth - width - gap))}px`
        this.#popup.style.top = `${Math.max(minY, Math.min(y, maxY - height))}px`
    }

    #hide() {
        clearTimeout(this.#selectionTimeout)
        this.#pending = null
        this.#anchorRange = null
        this.#popup?.close()
    }

    #dismiss() {
        this.#hide()
        this.#view.deselect()
        this.#view.focus({ preventScroll: true })
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

    async #submit({ annotation = this.#pending, button = this.#actionButton, listIndex = null } = {}) {
        if (!annotation || this.#saving || !this.#sync.isAuthenticated) return
        const { value, content, remove } = annotation
        const savedAnnotation = { value, content }
        this.#saving = true
        button.disabled = true
        try {
            const response = await fetch(this.#url, {
                method: remove ? 'DELETE' : 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(remove ? { cfi: value } : { cfi: value, content }),
            })
            if (this.#sessionExpired(response)) return
            if (!response.ok) throw new Error(`Annotation ${remove ? 'deletion' : 'save'} failed: HTTP ${response.status}`)
            if (remove) this.#annotations.delete(value)
            else this.#annotations.set(value, savedAnnotation)
            if (listIndex === null) this.#dismiss()
            else this.#hide()
            this.#renderList(listIndex)
        } catch (error) {
            console.error('Error updating text annotation:', error)
            this.#notify('warning', remove
                ? this.#translations.annotation_remove_failed : this.#translations.annotation_save_failed)
            return
        } finally {
            this.#saving = false
            button.disabled = false
        }
        this.#notify('success', remove
            ? this.#translations.annotation_removed : this.#translations.annotation_saved)
        try {
            if (remove) await this.#view.deleteAnnotation(savedAnnotation)
            else {
                const { index } = await this.#view.resolveNavigation(value)
                await this.#restore(index)
            }
        } catch (error) {
            console.error('Error updating annotation overlay:', error)
            this.#notify('warning', this.#translations.annotations_display_failed)
        }
    }
}
