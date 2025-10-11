import './foliate-js/view.js'
import { createTOCView } from './foliate-js/ui/tree.js'
import { createMenu } from './foliate-js/ui/menu.js'
import { Overlayer } from './foliate-js/overlayer.js'

const getCSS = ({ spacing, justify, hyphenate }) => `
    @namespace epub "http://www.idpf.org/2007/ops";
    html {
        color-scheme: light dark;
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
    }
    annotations = new Map()
    annotationsByValue = new Map()
    closeSideBar() {
        $('#dimming-overlay').classList.remove('show')
        $('#side-bar').classList.remove('show')
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

       const t = JSON.parse(document.getElementById('i18n').textContent).i18n;

       const menu = createMenu([
            {
                name: 'layout',
                label: 'Layout',
                type: 'radio',
                items: [
                    [t.paginated, 'paginated'],
                    [t.scrolled, 'scrolled'],
                ],
                onclick: value => {
                    this.view?.renderer.setAttribute('flow', value)
                },
            },
        ])
        menu.element.classList.add('menu')

        $('#menu-button').append(menu.element)
        $('#menu-button > button').addEventListener('click', () =>
            menu.element.classList.toggle('show'))
        menu.groups.layout.select('paginated')
        
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

        const { book } = this.view
        this.view.renderer.setStyles?.(getCSS(this.style))
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
                    const element = target.anchor(targetDoc)
                    if (element) {
                        this.#showFootnote(element)
                        return
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
        
        // Force positioning
        setTimeout(() => {
            this.#footnoteModal.style.position = 'fixed'
            this.#footnoteModal.style.top = '50%'
            this.#footnoteModal.style.left = '50%'
            this.#footnoteModal.style.transform = 'translate(-50%, -50%)'
            this.#footnoteModal.style.margin = '0'
            this.#footnoteContent.focus()
        }, 0)
    }
    #showFootnoteError() {
        if (!this.#footnoteModal || !this.#footnoteContent) return
        
        const errorMessage = this.#footnoteModal.dataset.errorMessage || '<p><em>Footnote content could not be loaded.</em></p>'
        this.#footnoteContent.innerHTML = errorMessage
        this.#footnoteModal.showModal()
        
        setTimeout(() => {
            this.#footnoteModal.style.position = 'fixed'
            this.#footnoteModal.style.top = '50%'
            this.#footnoteModal.style.left = '50%'
            this.#footnoteModal.style.transform = 'translate(-50%, -50%)'
            this.#footnoteModal.style.margin = '0'
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
