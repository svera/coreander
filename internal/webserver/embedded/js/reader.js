import './foliate-js/view.js'
import { createTOCView } from './foliate-js/ui/tree.js'
import { createMenu } from './menu.js'
import { Overlayer } from './foliate-js/overlayer.js'

const getCSS = ({ spacing, justify, hyphenate, theme, fontSize }) => `
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
    style = {
        spacing: 1.4,
        justify: true,
        hyphenate: true,
        theme: 'auto',
        fontSize: 100, // Percentage: 100 = 100%
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
    #applyTheme(theme) {
        // Save theme preference to localStorage
        window.localStorage.setItem('reader-theme', theme)
        
        // Apply theme to the main document
        const html = document.documentElement
        html.dataset.theme = theme
        if (theme === 'dark') {
            html.style.colorScheme = 'dark'
            document.body.style.background = '#1a1a1a'
            document.body.style.color = '#e0e0e0'
        } else if (theme === 'light') {
            html.style.colorScheme = 'light'
            document.body.style.background = '#ffffff'
            document.body.style.color = '#000000'
        } else {
            // Auto mode
            html.style.colorScheme = 'light dark'
            document.body.style.removeProperty('background')
            document.body.style.removeProperty('color')
        }
    }
    #setupFootnoteModal() {
        this.#footnoteModal = $('#footnote-modal')
        this.#footnoteContent = $('#footnote-content')
        
        if (!this.#footnoteModal || !this.#footnoteContent) return
        
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
        $('#side-bar-button').addEventListener('click', () => {
            $('#dimming-overlay').classList.add('show')
            $('#side-bar').classList.add('show')
        })
        $('#dimming-overlay').addEventListener('click', () => this.closeSideBar())
        $('#side-bar-close').addEventListener('click', () => this.closeSideBar())

       const t = JSON.parse(document.getElementById('i18n').textContent).i18n;
       
       // Create font size controls
       const fontSizeControls = document.createElement('div')
       fontSizeControls.id = 'font-size-controls'
       fontSizeControls.style.display = 'flex'
       fontSizeControls.style.gap = '6px'
       
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
        ])
        menu.element.classList.add('menu')
        
        // Store references to font size elements for later removal if needed
        this.fontSizeMenuItem = menu.groups.fontSize?.element
        // The separator is the element right before the fontSize menu item
        this.fontSizeSeparator = this.fontSizeMenuItem?.previousElementSibling

        $('#menu-button').append(menu.element)
        $('#menu-button > button').addEventListener('click', () =>
            menu.element.classList.toggle('show'))
        
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
        
        // Initialize font size button states
        this.#updateFontSizeButtons()
        
        // Initialize footnote modal
        this.#setupFootnoteModal()
    }
    async open(file) {
        this.view = document.createElement('foliate-view')
        const storage = window.localStorage
        const slug = document.getElementById('slug').value
        document.body.append(this.view)
        await this.view.open(file)
        await this.view.init({lastLocation: storage.getItem(slug)})
        
        // Check if it's pre-paginated content (PDF or fixed-layout) after the book is opened
        // Font size controls don't work for pre-paginated content
        const { book } = this.view
        const isPrePaginated = book?.rendition?.layout === 'pre-paginated'
        if (isPrePaginated) {
            // Remove font size controls and their separator from the menu
            this.fontSizeMenuItem?.remove()
            this.fontSizeSeparator?.remove()
        }
        this.view.addEventListener('load', this.#onLoad.bind(this))
        this.view.addEventListener('relocate', this.#onRelocate.bind(this))
        
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
        
        this.view.renderer.next()

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

        storage.setItem(slug, detail.cfi)
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
            return location.reload()
        }
        return res.blob()
    })
    .then(blob => open(new File([blob], new URL(url).pathname)))
    .catch(e => {
        document.body.removeChild($('#spinner-container'));
        $('#error-icon-container').classList.remove('d-none');
        console.error(e);
    })
else dropTarget.style.visibility = 'visible'
