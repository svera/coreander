import './foliate-js/view.js'
import { createTOCView } from './foliate-js/ui/tree.js'
import { createMenu } from './menu.js'
import { Overlayer } from './foliate-js/overlayer.js'
import { ReaderSync } from './reader-sync.js'
import { ReaderToast } from './reader-toast.js'

const getCSS = ({ spacing, justify, hyphenate, theme, fontSize, fontFamily }) => `
    @namespace epub "http://www.idpf.org/2007/ops";
    html {
        color-scheme: ${theme === 'auto' ? 'light dark' : theme};
        font-size: ${fontSize}% !important;
    }
    /* https://github.com/whatwg/html/issues/5426 */
    @media (prefers-color-scheme: dark) {
        a:link {
            color: lightblue;
        }
    }
    body {
        font-family: ${fontFamily === 'sans-serif' ? 'sans-serif' : 'serif'} !important;
    }
    p, li, blockquote, dd {
        line-height: ${spacing};
        text-align: ${justify ? 'justify' : 'start'};
        -webkit-hyphens: ${hyphenate ? 'auto' : 'manual'};
        hyphens: ${hyphenate ? 'auto' : 'manual'};
        -webkit-hyphenate-limit-before: 3;
        -webkit-hyphenate-limit-after: 2;
        -webkit-hyphenate-limit-lines: 2;
        hanging-punctuation: allow-end last;
        widows: 2;
    }
    /* prevent the above from overriding the align attribute */
    [align="left"] { text-align: left; }
    [align="right"] { text-align: right; }
    [align="center"] { text-align: center; }
    [align="justify"] { text-align: justify; }

    pre {
        white-space: pre-wrap !important;
    }
    aside[epub|type~="endnote"],
    aside[epub|type~="footnote"],
    aside[epub|type~="note"],
    aside[epub|type~="rearnote"] {
        display: none;
    }
`

const $ = document.querySelector.bind(document)

const locales = 'en'
const percentFormat = new Intl.NumberFormat(locales, { style: 'percent' })
const listFormat = new Intl.ListFormat(locales, { style: 'short', type: 'conjunction' })

const formatLanguageMap = x => {
    if (!x) return ''
    if (typeof x === 'string') return x
    const keys = Object.keys(x)
    return x[keys[0]]
}

const formatOneContributor = contributor => typeof contributor === 'string'
    ? contributor : formatLanguageMap(contributor?.name)

const formatContributor = contributor => Array.isArray(contributor)
    ? listFormat.format(contributor.map(formatOneContributor))
    : formatOneContributor(contributor)

class Reader {
    #tocView
    #footnoteModal
    #footnoteContent
    #toast
    #sessionExpiredShown = false
    #notLoggedInShown = false
    #sidebarOpening = false
    #skipNextPush = false
    sync = null
    view = null
    translations = null
    style = {
        spacing: 1.4, // Line height
        justify: true,
        hyphenate: true,
        theme: 'auto',
        fontSize: 100, // Percentage: 100 = 100%
        fontFamily: 'serif', // 'serif' or 'sans-serif'
    }
    #defaultFontSize = 100
    #minFontSize = 75
    #maxFontSize = 200
    #fontSizeStep = 10
    annotations = new Map()
    annotationsByValue = new Map()
    closeSideBar() {
        $('#dimming-overlay').classList.remove('show')
        $('#side-bar').classList.remove('show')
        // Refocus the view so keyboard navigation works
        if (this.view) {
            this.view.focus()
        }
    }
    #increaseFontSize() {
        if (this.style.fontSize < this.#maxFontSize) {
            this.style.fontSize += this.#fontSizeStep
            this.#applyFontSize()
        }
    }
    #decreaseFontSize() {
        if (this.style.fontSize > this.#minFontSize) {
            this.style.fontSize -= this.#fontSizeStep
            this.#applyFontSize()
        }
    }
    #resetFontSize() {
        this.style.fontSize = this.#defaultFontSize
        this.#applyFontSize()
    }
    #applyFontSize() {
        window.localStorage.setItem('reader-fontSize', this.style.fontSize)
        if (this.view?.renderer) {
            this.view.renderer.setStyles?.(getCSS(this.style))
        }
        // Also apply font size to footnote modal
        const footnoteModal = document.getElementById('footnote-modal')
        if (footnoteModal) {
            footnoteModal.style.fontSize = `${this.style.fontSize}%`
        }
        this.#updateFontSizeButtons()
    }
    #updateFontSizeButtons() {
        const decreaseBtn = $('#decrease-font')
        const increaseBtn = $('#increase-font')

        if (decreaseBtn) {
            decreaseBtn.disabled = this.style.fontSize <= this.#minFontSize
        }
        if (increaseBtn) {
            increaseBtn.disabled = this.style.fontSize >= this.#maxFontSize
        }
    }
    #setLineHeight(value) {
        this.style.spacing = value
        window.localStorage.setItem('reader-lineHeight', value.toString())
        if (this.view?.renderer) {
            this.view.renderer.setStyles?.(getCSS(this.style))
        }
        this.#updateLineHeightButtons()
    }
    #updateLineHeightButtons() {
        // Early return if buttons aren't initialized
        if (!this.lineHeightButtons) return

        const regularLineBtn = this.lineHeightButtons.regularLineBtn
        const mediumLineBtn = this.lineHeightButtons.mediumLineBtn
        const largeLineBtn = this.lineHeightButtons.largeLineBtn

        // Return if any button is missing
        if (!regularLineBtn || !mediumLineBtn || !largeLineBtn) return

        const current = this.style.spacing

        // Remove active class from all buttons
        regularLineBtn.classList.remove('active')
        mediumLineBtn.classList.remove('active')
        largeLineBtn.classList.remove('active')

        // Add active class to current selection (CSS handles styling)
        if (Math.abs(current - 1.4) < 0.01) {
            regularLineBtn.classList.add('active')
        } else if (Math.abs(current - 1.6) < 0.01) {
            mediumLineBtn.classList.add('active')
        } else if (Math.abs(current - 2.0) < 0.01) {
            largeLineBtn.classList.add('active')
        }
    }
    #setFontFamily(value) {
        this.style.fontFamily = value
        window.localStorage.setItem('reader-fontFamily', value)
        if (this.view?.renderer) {
            this.view.renderer.setStyles?.(getCSS(this.style))
        }
        this.#updateFontFamilyButtons()
    }
    #updateFontFamilyButtons() {
        // Early return if buttons aren't initialized
        if (!this.fontFamilyButtons) return

        const serifBtn = this.fontFamilyButtons.serifBtn
        const sansSerifBtn = this.fontFamilyButtons.sansSerifBtn

        // Return if any button is missing
        if (!serifBtn || !sansSerifBtn) return

        const current = this.style.fontFamily

        // Remove active class from all buttons
        serifBtn.classList.remove('active')
        sansSerifBtn.classList.remove('active')

        // Add active class to current selection (CSS handles styling)
        if (current === 'serif') {
            serifBtn.classList.add('active')
        } else if (current === 'sans-serif') {
            sansSerifBtn.classList.add('active')
        }
    }
    #applyTheme(theme) {
        // Save theme preference to localStorage
        window.localStorage.setItem('reader-theme', theme)

        // Apply theme to the main document using system color-scheme
        const html = document.documentElement
        html.dataset.theme = theme
        html.style.colorScheme = 'light dark'
        if (theme === 'dark') {
            html.style.colorScheme = 'dark'
        } else if (theme === 'light') {
            html.style.colorScheme = 'light'
        }
        // Remove any hardcoded colors to let system color-scheme apply
        document.body.style.removeProperty('background')
        document.body.style.removeProperty('color')
    }
    #setupFootnoteModal() {
        this.#footnoteModal = $('#footnote-modal')
        this.#footnoteContent = $('#footnote-content')

        if (!this.#footnoteModal || !this.#footnoteContent) return

        // Apply current font size to modal
        this.#footnoteModal.style.fontSize = `${this.style.fontSize}%`

        // Set up close button
        const closeBtn = $('#footnote-close')
        if (closeBtn) {
            closeBtn.onclick = () => this.#footnoteModal.close()
        }

        // Close on backdrop click
        this.#footnoteModal.onclick = (e) => {
            if (e.target === this.#footnoteModal) {
                this.#footnoteModal.close()
            }
        }

        // Close on Escape key
        this.#footnoteModal.onkeydown = (e) => {
            if (e.key === 'Escape') {
                this.#footnoteModal.close()
            }
        }
    }
    constructor() {
        // Check if user is authenticated
        const isAuthenticated = document.getElementById('authenticated')?.value === 'true'

        // Load translations
        this.translations = JSON.parse(document.getElementById('i18n').textContent).i18n

        // Initialize toast
        this.#toast = new ReaderToast()

        // Initialize sync helper
        this.sync = new ReaderSync(isAuthenticated)

        // Listen for sync events
        window.addEventListener('reader-session-expired', () => this.showSessionExpired())
        window.addEventListener('reader-position-updated', () => this.showPositionUpdated())

        // Show not logged in notification if needed
        if (!isAuthenticated) {
            this.showNotLoggedIn()
        }

        $('#side-bar-button').addEventListener('click', () => {
            this.#sidebarOpening = true
            this.#skipNextPush = true
            $('#dimming-overlay').classList.add('show')
            $('#side-bar').classList.add('show')
            // Clear the flags after a short delay to allow normal syncing to resume
            setTimeout(() => {
                this.#sidebarOpening = false
                this.#skipNextPush = false
            }, 500)
        })
        $('#dimming-overlay').addEventListener('click', () => this.closeSideBar())
        $('#side-bar-close').addEventListener('click', () => this.closeSideBar())

       const t = this.translations;

       // Create font size controls
       const fontSizeControls = document.createElement('div')
       fontSizeControls.id = 'font-size-controls'

       const decreaseBtn = document.createElement('button')
       decreaseBtn.id = 'decrease-font'
       decreaseBtn.setAttribute('aria-label', t.decrease_font_size)
       decreaseBtn.title = t.decrease_font_size
       decreaseBtn.innerHTML = '<svg class="icon" width="24" height="24" aria-hidden="true"><text x="11" y="18" text-anchor="middle" font-size="18" font-weight="bold">A</text><line x1="17" y1="14" x2="22" y2="14" stroke="currentColor" stroke-width="2"/></svg>'
       decreaseBtn.addEventListener('click', () => this.#decreaseFontSize())

       const resetBtn = document.createElement('button')
       resetBtn.id = 'reset-font'
       resetBtn.setAttribute('aria-label', t.reset_font_size)
       resetBtn.title = t.reset_font_size
       resetBtn.innerHTML = '<svg class="icon" width="24" height="24" aria-hidden="true"><text x="12" y="18" text-anchor="middle" font-size="16" font-weight="bold">A</text></svg>'
       resetBtn.addEventListener('click', () => this.#resetFontSize())

       const increaseBtn = document.createElement('button')
       increaseBtn.id = 'increase-font'
       increaseBtn.setAttribute('aria-label', t.increase_font_size)
       increaseBtn.title = t.increase_font_size
       increaseBtn.innerHTML = '<svg class="icon" width="24" height="24" aria-hidden="true"><text x="11" y="18" text-anchor="middle" font-size="18" font-weight="bold">A</text><line x1="17" y1="14" x2="22" y2="14" stroke="currentColor" stroke-width="2"/><line x1="19.5" y1="11" x2="19.5" y2="17" stroke="currentColor" stroke-width="2"/></svg>'
       increaseBtn.addEventListener('click', () => this.#increaseFontSize())

       fontSizeControls.append(decreaseBtn, resetBtn, increaseBtn)

       // Create line height controls
       const lineHeightControls = document.createElement('div')
       lineHeightControls.id = 'line-height-controls'

       const regularLineBtn = document.createElement('button')
       regularLineBtn.setAttribute('data-line-height', '1.4')
       regularLineBtn.setAttribute('aria-label', t.regular + ' ' + t.line_height)
       regularLineBtn.title = t.regular + ' (1.4)'
       regularLineBtn.innerHTML = '<svg class="icon" width="24" height="24" aria-hidden="true"><line x1="4" y1="8" x2="20" y2="8" stroke="currentColor" stroke-width="1.5"/><line x1="4" y1="12" x2="20" y2="12" stroke="currentColor" stroke-width="1.5"/><line x1="4" y1="16" x2="20" y2="16" stroke="currentColor" stroke-width="1.5"/></svg>'
       regularLineBtn.addEventListener('click', () => this.#setLineHeight(1.4))

       const mediumLineBtn = document.createElement('button')
       mediumLineBtn.setAttribute('data-line-height', '1.6')
       mediumLineBtn.setAttribute('aria-label', t.line_height + ' 1.6')
       mediumLineBtn.title = '1.6'
       mediumLineBtn.innerHTML = '<svg class="icon" width="24" height="24" aria-hidden="true"><line x1="4" y1="7" x2="20" y2="7" stroke="currentColor" stroke-width="1.5"/><line x1="4" y1="12" x2="20" y2="12" stroke="currentColor" stroke-width="1.5"/><line x1="4" y1="17" x2="20" y2="17" stroke="currentColor" stroke-width="1.5"/></svg>'
       mediumLineBtn.addEventListener('click', () => this.#setLineHeight(1.6))

       const largeLineBtn = document.createElement('button')
       largeLineBtn.setAttribute('data-line-height', '2.0')
       largeLineBtn.setAttribute('aria-label', t.line_height + ' 2.0')
       largeLineBtn.title = '2.0'
       largeLineBtn.innerHTML = '<svg class="icon" width="24" height="24" aria-hidden="true"><line x1="4" y1="6" x2="20" y2="6" stroke="currentColor" stroke-width="1.5"/><line x1="4" y1="12" x2="20" y2="12" stroke="currentColor" stroke-width="1.5"/><line x1="4" y1="18" x2="20" y2="18" stroke="currentColor" stroke-width="1.5"/></svg>'
       largeLineBtn.addEventListener('click', () => this.#setLineHeight(2.0))

       lineHeightControls.append(regularLineBtn, mediumLineBtn, largeLineBtn)
       this.lineHeightButtons = { regularLineBtn, mediumLineBtn, largeLineBtn }

       // Create font family controls
       const fontFamilyControls = document.createElement('div')
       fontFamilyControls.id = 'font-family-controls'

       const serifBtn = document.createElement('button')
       serifBtn.setAttribute('data-font-family', 'serif')
       serifBtn.setAttribute('aria-label', t.serif)
       serifBtn.title = t.serif
       serifBtn.innerHTML = '<svg class="icon" width="24" height="24" aria-hidden="true"><text x="12" y="18" text-anchor="middle" font-size="16" font-weight="bold" font-family="serif">Aa</text></svg>'
       serifBtn.addEventListener('click', () => this.#setFontFamily('serif'))

       const sansSerifBtn = document.createElement('button')
       sansSerifBtn.setAttribute('data-font-family', 'sans-serif')
       sansSerifBtn.setAttribute('aria-label', t.sans_serif)
       sansSerifBtn.title = t.sans_serif
       sansSerifBtn.innerHTML = '<svg class="icon" width="24" height="24" aria-hidden="true"><text x="12" y="18" text-anchor="middle" font-size="16" font-weight="bold" font-family="sans-serif">Aa</text></svg>'
       sansSerifBtn.addEventListener('click', () => this.#setFontFamily('sans-serif'))

       fontFamilyControls.append(serifBtn, sansSerifBtn)
       this.fontFamilyButtons = { serifBtn, sansSerifBtn }

       const menu = createMenu([
            {
                name: 'continuous',
                label: t.continuous,
                type: 'checkbox',
                onclick: checked => {
                    window.localStorage.setItem('reader-continuous', checked)
                    this.view?.renderer.setAttribute('flow', checked ? 'scrolled' : 'paginated')
                },
            },
            {
                type: 'separator',
            },
            {
                name: 'theme',
                label: 'Theme',
                type: 'radio',
                items: [
                    [t.auto, 'auto'],
                    [t.light, 'light'],
                    [t.dark, 'dark'],
                ],
                onclick: value => {
                    this.style.theme = value
                    this.#applyTheme(value)
                    if (this.view?.renderer) {
                        this.view.renderer.setStyles?.(getCSS(this.style))
                    }
                },
            },
            {
                type: 'separator',
            },
            {
                name: 'fontSize',
                type: 'custom',
                content: fontSizeControls
            },
            {
                type: 'separator',
            },
            {
                name: 'lineHeight',
                type: 'custom',
                content: lineHeightControls
            },
            {
                type: 'separator',
            },
            {
                name: 'fontFamily',
                type: 'custom',
                content: fontFamilyControls
            },
        ])
        menu.element.classList.add('menu')

        // Store references to font size elements for later removal if needed
        this.fontSizeMenuItem = menu.groups.fontSize?.element
        // The separator is the element right before the fontSize menu item
        this.fontSizeSeparator = this.fontSizeMenuItem?.previousElementSibling

        // Store references to line height elements for later removal if needed
        this.lineHeightMenuItem = menu.groups.lineHeight?.element
        // The separator is the element right before the lineHeight menu item
        this.lineHeightSeparator = this.lineHeightMenuItem?.previousElementSibling

        $('#menu-button').append(menu.element)
        $('#menu-button > button').addEventListener('click', () => {
            const wasOpen = menu.element.classList.contains('show')
            menu.element.classList.toggle('show')
            // If we're closing the menu, refocus the view for keyboard navigation
            if (wasOpen && this.view) {
                this.view.focus()
            }
        })

        // Watch for menu being hidden by other means (click outside, window blur)
        // and refocus the view for keyboard navigation
        let wasMenuVisible = false
        const menuObserver = new MutationObserver(() => {
            const isMenuVisible = menu.element.classList.contains('show')
            // Only refocus if menu transitioned from visible to hidden
            if (wasMenuVisible && !isMenuVisible && this.view) {
                // Menu was closed, refocus the view after a brief delay
                setTimeout(() => {
                    if (this.view) {
                        this.view.focus()
                    }
                }, 100)
            }
            wasMenuVisible = isMenuVisible
        })
        menuObserver.observe(menu.element, { attributes: true, attributeFilter: ['class'] })

        // Load saved theme from localStorage or default to 'auto'
        const storage = window.localStorage
        const savedTheme = storage.getItem('reader-theme') || 'auto'
        this.style.theme = savedTheme
        menu.groups.theme.select(savedTheme)
        this.#applyTheme(savedTheme)

        // Load saved continuous mode from localStorage or default to false (paginated mode)
        const savedContinuous = storage.getItem('reader-continuous') === 'true'
        menu.groups.continuous.setChecked(savedContinuous)

        // Load saved font size from localStorage or default to 100%
        const savedFontSize = parseInt(storage.getItem('reader-fontSize'))
        if (savedFontSize && savedFontSize >= this.#minFontSize && savedFontSize <= this.#maxFontSize) {
            this.style.fontSize = savedFontSize
        }

        // Load saved line height from localStorage or default to 1.4
        const savedLineHeight = storage.getItem('reader-lineHeight') || '1.4'
        this.style.spacing = parseFloat(savedLineHeight)

        // Load saved font family from localStorage or default to 'serif'
        const savedFontFamily = storage.getItem('reader-fontFamily') || 'serif'
        this.style.fontFamily = savedFontFamily

        // Initialize button states
        this.#updateFontSizeButtons()
        this.#updateLineHeightButtons()
        this.#updateFontFamilyButtons()

        // Initialize footnote modal
        this.#setupFootnoteModal()

        // Sync position from server when tab becomes visible or window gains focus
        document.addEventListener('visibilitychange', () => {
            if (!document.hidden && !this.#sidebarOpening) {
                // Set flag to skip pushing position updates triggered by this event
                this.#skipNextPush = true
                setTimeout(() => {
                    this.#skipNextPush = false
                }, 500)

                // Tab is visible again, sync from server (debounced)
                this.sync.debouncedSyncPositionFromServer()
            }
        })

        window.addEventListener('focus', () => {
            // Window gained focus, sync from server (debounced)
            if (!this.#sidebarOpening) {
                // Set flag to skip pushing position updates triggered by this event
                this.#skipNextPush = true
                setTimeout(() => {
                    this.#skipNextPush = false
                }, 500)

                this.sync.debouncedSyncPositionFromServer()
            }
        })
    }
    async open(file) {
        this.view = document.createElement('foliate-view')
        // Make the view focusable so it can receive keyboard events
        this.view.setAttribute('tabindex', '0')
        const storage = window.localStorage
        const slug = document.getElementById('slug').value
        document.body.append(this.view)
        await this.view.open(file)

        // Get position, syncing with server if authenticated
        const localData = this.sync.getLocalPosition(slug)
        let lastLocation = localData.position

        if (this.sync.isAuthenticated) {
            const serverData = await this.sync.getServerPosition(slug)

            // Compare timestamps and use the newer position
            if (serverData.position && serverData.updated) {
                if (!localData.updated || new Date(serverData.updated) > new Date(localData.updated)) {
                    // Server position is newer
                    lastLocation = serverData.position
                    // Update localStorage with server data
                    storage.setItem(slug, JSON.stringify({
                        position: serverData.position,
                        updated: serverData.updated
                    }))
                }
            }
        }

        await this.view.init({lastLocation})

        // Set view in sync helper after initialization
        this.sync.setView(this.view)

        // Check if it's pre-paginated content (PDF or fixed-layout) after the book is opened
        // Font size and line height controls don't work for pre-paginated content
        const { book } = this.view
        const isPrePaginated = book?.rendition?.layout === 'pre-paginated'
        if (isPrePaginated) {
            // Remove font size and line height controls and their separators from the menu
            this.fontSizeMenuItem?.remove()
            this.fontSizeSeparator?.remove()
            this.lineHeightMenuItem?.remove()
            this.lineHeightSeparator?.remove()
        }
        this.view.addEventListener('load', this.#onLoad.bind(this))
        this.view.addEventListener('relocate', this.#onRelocate.bind(this))

        // Add keyboard listener directly to the view
        this.view.addEventListener('keydown', this.#handleKeydown.bind(this))

        // Focus the view when clicked to enable keyboard navigation
        this.view.addEventListener('click', () => {
            this.view.focus()
        })

        // Focus the view initially
        this.view.focus()

        // Intercept link events to handle footnotes
        this.view.addEventListener('link', e => {
            const { a, href } = e.detail

            // Check if this looks like a footnote link
            const isLikelyFootnote = href && (
                href.includes('footnote') ||
                href.includes('note') ||
                a.textContent.match(/^\d+$/) ||
                a.getAttributeNS('http://www.idpf.org/2007/ops', 'type') === 'noteref' ||
                a.querySelector('sup')
            )

            if (isLikelyFootnote) {
                // Return false to prevent view.js from navigating
                e.preventDefault()
                this.#handleFootnoteLinkEvent(href)
                return false
            }
        })

        document.body.removeChild($('#spinner-container'))
        document.body.removeChild($('#error-icon-container'))

        this.view.renderer.setStyles?.(getCSS(this.style))

        // Apply saved continuous mode state
        const savedContinuous = storage.getItem('reader-continuous') === 'true'
        this.view.renderer.setAttribute('flow', savedContinuous ? 'scrolled' : 'paginated')

        $('#header-bar').style.visibility = 'visible'
        $('#nav-bar').style.visibility = 'visible'
        $('#left-button').addEventListener('click', () => this.view.goLeft())
        $('#right-button').addEventListener('click', () => this.view.goRight())

        const slider = $('#progress-slider')
        slider.dir = book.dir
        slider.addEventListener('input', e =>
            this.view.goToFraction(parseFloat(e.target.value)))
        for (const fraction of this.view.getSectionFractions()) {
            const option = document.createElement('option')
            option.value = fraction
            $('#tick-marks').append(option)
        }

        document.addEventListener('keydown', this.#handleKeydown.bind(this))

        const title = formatLanguageMap(book.metadata?.title) || t.untitled_document
        document.title = title
        $('#side-bar-title').innerText = title
        $('#side-bar-author').innerText = formatContributor(book.metadata?.author)
        Promise.resolve(book.getCover?.())?.then(blob =>
            blob ? $('#side-bar-cover').src = URL.createObjectURL(blob) : null)

        const toc = book.toc
        if (toc) {
            this.#tocView = createTOCView(toc, href => {
                this.view.goTo(href).catch(e => console.error(e))
                this.closeSideBar()
            })
            $('#toc-view').append(this.#tocView.element)
        }

        // load and show highlights embedded in the file by Calibre
        const bookmarks = await book.getCalibreBookmarks?.()
        if (bookmarks) {
            const { fromCalibreHighlight } = await import('./foliate-js/epubcfi.js')
            for (const obj of bookmarks) {
                if (obj.type === 'highlight') {
                    const value = fromCalibreHighlight(obj)
                    const color = obj.style.which
                    const note = obj.notes
                    const annotation = { value, color, note }
                    const list = this.annotations.get(obj.spine_index)
                    if (list) list.push(annotation)
                    else this.annotations.set(obj.spine_index, [annotation])
                    this.annotationsByValue.set(value, annotation)
                }
            }
            this.view.addEventListener('create-overlay', e => {
                const { index } = e.detail
                const list = this.annotations.get(index)
                if (list) for (const annotation of list)
                    this.view.addAnnotation(annotation)
            })
            this.view.addEventListener('draw-annotation', e => {
                const { draw, annotation } = e.detail
                const { color } = annotation
                draw(Overlayer.highlight, { color })
            })
            this.view.addEventListener('show-annotation', e => {
                const annotation = this.annotationsByValue.get(e.detail.value)
                if (annotation.note) alert(annotation.note)
            })
        }
    }
    #handleKeydown(event) {
        // Don't handle navigation keys when focus is on input elements
        const target = event.target
        if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT')) {
            return
        }

        const k = event.key
        if (k === 'ArrowLeft' || k === 'h') this.view.goLeft()
        else if(k === 'ArrowRight' || k === 'l') this.view.goRight()
    }
    #onLoad({ detail: { doc, index } }) {
        doc.addEventListener('keydown', this.#handleKeydown.bind(this))
    }
    async #handleFootnoteLinkEvent(href) {
        try {
            const { book } = this.view

            // Resolve the href to get the target
            const target = await book.resolveHref(href)

            if (target && target.index !== undefined) {
                const targetSection = book.sections[target.index]
                const targetDoc = await targetSection.createDocument()

                if (targetDoc && target.anchor) {
                    let element = target.anchor(targetDoc)

                    if (element) {
                        // If the anchor element is empty or just whitespace,
                        // try to find the actual content
                        if (!element.textContent.trim()) {
                            // Try parent element (for EPUB2 style footnotes)
                            const parent = element.parentElement
                            if (parent && parent.textContent.trim()) {
                                element = parent
                            } else {
                                // Try next sibling
                                const nextSibling = element.nextElementSibling
                                if (nextSibling && nextSibling.textContent.trim()) {
                                    element = nextSibling
                                }
                            }
                        }

                        // Check if we have content now
                        if (element && element.textContent.trim()) {
                            this.#showFootnote(element)
                            return
                        }
                    }
                }
            }

            // Fallback: show error message
            this.#showFootnoteError()
        } catch (error) {
            console.error('Error handling footnote:', error)
            this.#showFootnoteError()
        }
    }
    #showFootnote(element) {
        if (!this.#footnoteModal || !this.#footnoteContent) return

        const clonedContent = element.cloneNode(true)

        // Remove backlinks
        const backlinks = clonedContent.querySelectorAll('[role="doc-backlink"]')
        backlinks.forEach(backlink => backlink.remove())

        // Remove internal links
        const links = clonedContent.querySelectorAll('a[href]')
        links.forEach(link => {
            const href = link.getAttribute('href')
            if (href && (href.startsWith('#') || !href.includes('://'))) {
                link.remove()
            }
        })

        this.#footnoteContent.innerHTML = ''
        this.#footnoteContent.appendChild(clonedContent)
        this.#footnoteModal.showModal()

        // Focus the content for keyboard navigation
        setTimeout(() => {
            this.#footnoteContent.focus()
        }, 0)
    }
    #showFootnoteError() {
        if (!this.#footnoteModal || !this.#footnoteContent) return

        const errorMessage = this.#footnoteModal.dataset.errorMessage || '<p><em>Footnote content could not be loaded.</em></p>'
        this.#footnoteContent.innerHTML = errorMessage
        this.#footnoteModal.showModal()

        // Focus the content for keyboard navigation
        setTimeout(() => {
            this.#footnoteContent.focus()
        }, 0)
    }
    #onRelocate({ detail }) {
        const storage = window.localStorage
        const slug = document.getElementById('slug').value

        storage.setItem(slug, JSON.stringify({
            position: detail.cfi,
            updated: new Date().toISOString()
        }))

        // Update position on server if authenticated (debounced)
        // Skip if sidebar is being opened or if we're skipping pushes (e.g., after focus events)
        if (this.sync.isAuthenticated && !this.#sidebarOpening && !this.#skipNextPush) {
            this.sync.schedulePositionUpdate(slug, detail.cfi)
        }

        const { fraction, location, tocItem, pageItem } = detail
        const percent = percentFormat.format(fraction)
        const loc = pageItem
            ? `Page ${pageItem.label}`
            : `Loc ${location.current}`
        const slider = $('#progress-slider')
        slider.style.visibility = 'visible'
        slider.value = fraction
        slider.title = `${percent} · ${loc}`
        if (tocItem?.href) this.#tocView?.setCurrentHref?.(tocItem.href)
    }
    showSessionExpired() {
        // Only show the notification once
        if (this.#sessionExpiredShown) return
        this.#sessionExpiredShown = true

        this.#toast.show('warning', this.translations.session_expired_reading)
    }
    showPositionUpdated() {
        this.#toast.show('success', this.translations.position_updated_from_server)
    }
    showNotLoggedIn() {
        // Only show the notification once
        if (this.#notLoggedInShown) return
        this.#notLoggedInShown = true

        this.#toast.show('warning', this.translations.not_logged_in_reading)
    }
}

const open = async file => {
    const reader = new Reader()
    globalThis.reader = reader
    await reader.open(file)
}

const url = document.getElementById('url').value
if (url) fetch(url)
    .then(res => {
        if (res.status == 403) {
            // Session expired or authentication required
            // Stop the response from rendering
            const spinner = $('#spinner-container');
            if (spinner) document.body.removeChild(spinner);
            $('#error-icon-container').classList.remove('d-none');
            const errorMsg = document.createElement('p');
            errorMsg.textContent = 'Session expired. Please ';
            const loginLink = document.createElement('a');
            loginLink.href = '/sessions/new';
            loginLink.textContent = 'log in';
            errorMsg.appendChild(loginLink);
            errorMsg.appendChild(document.createTextNode(' to continue reading.'));
            $('#error-icon-container').appendChild(errorMsg);
            throw new Error('Authentication required');
        }
        if (!res.ok) {
            throw new Error(`HTTP error! status: ${res.status}`);
        }
        return res.blob()
    })
    .then(blob => {
        if (blob) open(new File([blob], new URL(url).pathname))
    })
    .catch(e => {
        if (e.message !== 'Authentication required') {
            const spinner = $('#spinner-container');
            if (spinner) document.body.removeChild(spinner);
            $('#error-icon-container').classList.remove('d-none');
        }
        console.error(e);
    })
else dropTarget.style.visibility = 'visible'
