import { compare } from './foliate-js/epubcfi.js'

const SELECTION_PREVIEW = 'reader-annotation-selection'

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
    #selectionOverlayer = null
    #actionButton
    #commentLabel
    #commentInput
    #commentText
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

        this.#popup = document.createElement('div')
        this.#popup.id = 'annotation-popup'
        this.#popup.hidden = true
        this.#popup.setAttribute('role', 'dialog')
        this.#actionButton = createButton('', () => this.#submit())
        const cancel = createButton(translations.cancel, () => this.#dismiss())
        this.#commentLabel = document.createElement('label')
        this.#commentLabel.textContent = translations.comment
        this.#commentInput = document.createElement('textarea')
        this.#commentInput.rows = 3
        this.#commentInput.addEventListener('focus', () => this.#previewSelection())
        this.#commentInput.addEventListener('keydown', event => {
            if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) &&
                !event.isComposing && !event.repeat &&
                !event.defaultPrevented && !this.#pending?.remove) {
                event.preventDefault()
                this.#submit()
            }
        })
        this.#commentLabel.append(this.#commentInput)
        this.#commentText = document.createElement('p')
        const actions = document.createElement('div')
        actions.className = 'annotation-actions'
        actions.append(this.#actionButton, cancel)
        this.#popup.append(this.#commentLabel, this.#commentText, actions)
        this.#popup.addEventListener('keydown', event => {
            event.stopPropagation()
            if (event.key === 'Escape') {
                event.preventDefault()
                this.#dismiss()
            }
        })
        document.addEventListener('keydown', event => {
            if (!this.#popup.hidden && !event.defaultPrevented && event.key === 'Escape') {
                event.preventDefault()
                event.stopPropagation()
                this.#dismiss()
            }
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
        const reposition = () => {
            if (!this.#popup.hidden) this.#positionPopup()
        }
        window.addEventListener('resize', reposition)
        window.visualViewport?.addEventListener('resize', reposition)
        window.visualViewport?.addEventListener('scroll', reposition)
    }
    async load() {
        if (!this.#sync.isAuthenticated) return
        try {
            const response = await fetch(this.#url)
            if (this.#sessionExpired(response)) return
            if (!response.ok) throw new Error(`Loading annotations failed: HTTP ${response.status}`)
            const annotations = await response.json()
            if (!Array.isArray(annotations) || annotations.some(item =>
                typeof item.cfi !== 'string' || typeof item.content !== 'string' ||
                (item.comment !== undefined && typeof item.comment !== 'string'))) {
                throw new Error('Invalid annotations response')
            }
            for (const { cfi: value, content, comment = '' } of annotations) {
                this.#annotations.set(value, { value, content, comment })
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
        // Wait until dragging ends; debounce late native long-press/handle updates.
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
            // Editing a comment can clear the book's selection; retain the captured range.
            if (this.#popup.contains(document.activeElement)) return
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
        this.#clearSelectionPreview()
        if (this.#pending?.value !== annotation.value || remove) {
            this.#commentInput.value = annotation.comment ?? ''
        }
        this.#pending = { ...annotation, remove }
        this.#anchorRange = range
        this.#commentLabel.hidden = remove
        this.#commentText.textContent = annotation.comment ?? ''
        this.#commentText.hidden = !remove || !annotation.comment
        this.#actionButton.textContent = remove
            ? this.#translations.remove_annotation : this.#translations.save_annotation
        this.#popup.setAttribute('aria-label', this.#actionButton.textContent)
        if (this.#popup.hidden) {
            this.#popup.style.visibility = 'hidden'
            this.#popup.hidden = false
        }
        this.#positionPopup()
        this.#popup.style.removeProperty('visibility')
        if (!remove && document.activeElement === this.#commentInput) this.#previewSelection()
    }

    #previewSelection() {
        if (!this.#anchorRange || this.#pending?.remove || this.#popup.hidden) return
        try {
            const doc = this.#anchorRange.startContainer.ownerDocument
            const overlayer = this.#view.renderer.getContents().find(item => item.doc === doc)?.overlayer
            if (!overlayer) throw new Error('The selected passage has no annotation overlay')
            this.#clearSelectionPreview()
            this.#selectionOverlayer = overlayer
            overlayer.add(SELECTION_PREVIEW, this.#anchorRange, this.#draw, { color: 'yellow' })
        } catch (error) {
            console.error('Error previewing the selected passage:', error)
            this.#notify('warning', this.#translations.annotations_display_failed)
        }
    }

    #clearSelectionPreview() {
        this.#selectionOverlayer?.remove(SELECTION_PREVIEW)
        this.#selectionOverlayer = null
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
        const viewport = window.visualViewport
        const viewportLeft = viewport?.offsetLeft ?? 0
        const viewportTop = viewport?.offsetTop ?? 0
        const viewportWidth = viewport?.width ?? document.documentElement.clientWidth
        const viewportHeight = viewport?.height ?? document.documentElement.clientHeight
        const viewportRight = viewportLeft + viewportWidth
        const viewportBottom = viewportTop + viewportHeight
        const gap = 8
        const minX = viewportLeft + gap
        const maxX = viewportRight - gap
        const minY = Math.max(viewportTop + gap,
            document.getElementById('header-bar')?.getBoundingClientRect().bottom ?? viewportTop + gap)
        const maxY = Math.min(viewportBottom - gap,
            document.getElementById('nav-bar')?.getBoundingClientRect().top ?? viewportBottom - gap)
        this.#popup.style.maxWidth = `${Math.max(0, maxX - minX)}px`
        this.#popup.style.maxHeight = `${Math.max(0, maxY - minY)}px`
        const rects = Array.from(this.#anchorRange.getClientRects(), rect => ({
            left: offsetX + rect.left * scaleX,
            right: offsetX + rect.right * scaleX,
            top: offsetY + rect.top * scaleY,
            bottom: offsetY + rect.bottom * scaleY,
        })).filter(rect => rect.right > minX && rect.left < maxX &&
            rect.bottom > minY && rect.top < maxY)
        if (!rects.length) {
            this.#hide()
            return
        }
        const top = Math.max(minY, Math.min(...rects.map(rect => rect.top)))
        const bottom = Math.min(maxY, Math.max(...rects.map(rect => rect.bottom)))
        const left = Math.max(minX, Math.min(...rects.map(rect => rect.left)))
        const right = Math.min(maxX, Math.max(...rects.map(rect => rect.right)))
        const { width, height } = this.#popup.getBoundingClientRect()
        const below = maxY - bottom - gap
        const above = top - minY - gap
        const y = below >= height || below >= above ? bottom + gap : top - height - gap
        this.#popup.style.left = `${Math.max(minX, Math.min((left + right - width) / 2, maxX - width))}px`
        this.#popup.style.top = `${Math.max(minY, Math.min(y, maxY - height))}px`
    }

    #hide() {
        clearTimeout(this.#selectionTimeout)
        this.#clearSelectionPreview()
        this.#pending = null
        this.#anchorRange = null
        if (this.#popup) this.#popup.hidden = true
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
        const comment = this.#commentInput.value
        if (!remove && Array.from(comment).length > 65536) {
            this.#notify('warning', this.#translations.comment_limit)
            return
        }
        const savedAnnotation = remove ? { value, content } : { value, content, comment }
        this.#saving = true
        button.disabled = true
        this.#commentInput.disabled = true
        try {
            const response = await fetch(this.#url, {
                method: remove ? 'DELETE' : 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(remove ? { cfi: value } : { cfi: value, content, comment }),
            })
            if (this.#sessionExpired(response)) return
            if (!response.ok) throw new Error(`Annotation ${remove ? 'deletion' : 'save'} failed: HTTP ${response.status}`)
            if (remove) this.#annotations.delete(value)
            else this.#annotations.set(value, savedAnnotation)
            if (listIndex === null) this.#dismiss()
            else this.#hide()
            this.#renderList(listIndex)
            // Persistence is authoritative; overlay failures must not undo a committed save/delete.
            try {
                if (remove) await this.#view.deleteAnnotation(savedAnnotation)
                else await this.#view.addAnnotation(savedAnnotation)
            } catch (error) {
                console.error('Error updating annotation overlay:', error)
                this.#notify('warning', this.#translations.annotations_display_failed)
                return
            }
            this.#notify('success', remove
                ? this.#translations.annotation_removed : this.#translations.annotation_saved)
        } catch (error) {
            console.error('Error updating text annotation:', error)
            this.#notify('warning', remove
                ? this.#translations.annotation_remove_failed : this.#translations.annotation_save_failed)
            return
        } finally {
            this.#saving = false
            button.disabled = false
            this.#commentInput.disabled = false
            if (!this.#sync.isAuthenticated) this.#hide()
        }
    }
}
