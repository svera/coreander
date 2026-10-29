import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('../embedded/js/reader.js', import.meta.url), 'utf8')
const template = await readFile(new URL('../embedded/views/document/reader.html', import.meta.url), 'utf8')
const css = await readFile(new URL('../embedded/css/reader.css', import.meta.url), 'utf8')
const mirrorSource = await readFile(new URL('../embedded/js/reader-reading-mode.js', import.meta.url), 'utf8')
const { bindReaderReadingMode } = await import(
    `data:text/javascript;base64,${Buffer.from(mirrorSource).toString('base64')}`)

// Exercise the reader's actual load handler without bootstrapping its toolbar or network sync.
const onLoad = source.match(/    #onLoad\(\{ detail: \{ doc \} \}\) \{([\s\S]*?)\n    \}/)[1]
const openStart = source.match(/    async open\(file\) \{([\s\S]*?)        bindReaderWheel/)[1]
const makeReader = new Function('document', 'window', 'bindReaderReadingMode', `
    return new class {
        #handleKeydown(event) { this.keys.push(event.key) }
        #onLoad({ detail: { doc } }) { ${onLoad} }
        keys = []
        async open(file) { ${openStart} }
    }()
`)

function chapter(text = 'Chapter text.\n\nAnother paragraph.') {
    const doc = new EventTarget()
    doc.body = { innerText: text }
    doc.documentElement = { lang: 'en', dir: 'ltr' }
    return { doc }
}

function setup() {
    const view = new EventTarget()
    const attributes = new Map()
    const chapters = [chapter(), chapter('Next chapter.')]
    let active = [chapters[0]]
    const relocate = () => view.dispatchEvent(new Event('relocate'))
    const load = index => {
        active = [chapters[index]]
        view.dispatchEvent(Object.assign(new Event('load'), {
            detail: { doc: chapters[index].doc, index },
        }))
        relocate()
    }
    view.renderer = { getContents: () => active }
    view.setAttribute = (name, value) => attributes.set(name, value)
    view.open = async () => load(0)
    const appended = []
    const document = {
        createElement: tag => {
            if (tag === 'foliate-view') return view
            return {
                tagName: tag.toUpperCase(),
                children: [],
                replaceChildren(...children) { this.children = children },
            }
        },
        body: { append: element => appended.push(element) },
        getElementById: () => ({ value: 'book' }),
    }
    view.ownerDocument = document
    return {
        reader: makeReader(document, { localStorage: {} }, bindReaderReadingMode),
        view, attributes, chapters, appended, load, relocate,
        setContents: contents => { active = contents },
    }
}

test('initial chapter text is exposed in the outer main document before initialization', async () => {
    const { reader, view, attributes, appended } = setup()
    await reader.open({})
    assert.equal(appended[0], view)
    const main = appended[1]
    assert.equal(main.tagName, 'MAIN')
    assert.equal(main.hidden, false)
    assert.deepEqual(main.children.map(p => p.textContent), ['Chapter text.', 'Another paragraph.'])
    assert.equal(main.children[0].lang, 'en')
    assert.equal(attributes.get('tabindex'), '0')
})

test('navigation replaces the chapter copy without accumulating old text or changing keyboard behavior', async () => {
    const { reader, chapters, load, appended, relocate } = setup()
    await reader.open({})
    const originalParagraphs = appended[1].children
    relocate()
    assert.equal(appended[1].children, originalParagraphs)
    load(1)
    assert.deepEqual(appended[1].children.map(p => p.textContent), ['Next chapter.'])
    for (const { doc } of chapters) {
        doc.dispatchEvent(Object.assign(new Event('keydown'), { key: 'ArrowRight' }))
    }
    assert.deepEqual(reader.keys, ['ArrowRight', 'ArrowRight'])
})

test('SVG fixed-layout documents expose only text, and empty pages clear the copy', async () => {
    const { reader, chapters, appended, setContents, relocate } = setup()
    const { doc } = chapters[0]
    doc.body = null
    doc.querySelectorAll = selector => {
        assert.equal(selector, 'text')
        return [{ textContent: 'SVG page text.' }]
    }
    await reader.open({})
    assert.deepEqual(appended[1].children.map(p => p.textContent), ['SVG page text.'])
    setContents([chapter(' \n ')])
    relocate()
    assert.equal(appended[1].hidden, true)
    assert.deepEqual(appended[1].children, [])
})

test('spreads copy both documents in order and treat book text as text rather than markup', async () => {
    const { reader, appended, setContents, relocate } = setup()
    await reader.open({})
    setContents([chapter('<script>not executable</script>'), chapter('Second page.')])
    relocate()
    assert.deepEqual(appended[1].children.map(p => p.textContent),
        ['<script>not executable</script>', 'Second page.'])
})

test('chapter mirror has on-screen layout instead of clipped or one-pixel text bounds', () => {
    const styles = css.match(/#reader-reading-content \{([^}]+)\}/)[1]
    assert.match(styles, /inset: 48px 0;/)
    assert.match(styles, /z-index: -1;/)
    assert.doesNotMatch(styles, /clip-path|width: 1px|height: 1px/)
    assert.match(css, /foliate-view \{\s*background: Canvas;/)
})

test('error icon and return link stay hidden until a document fails to load', () => {
    assert.match(template, /<div id="error-icon-container" hidden>/)
    const failureHandler = source.match(/    \.catch\(e => \{([\s\S]*?)\n    \}\)/)[1]
    const errorPanel = { hidden: true }
    const errors = []
    const handleFailure = new Function('$', 'document', 'console', 'e', failureHandler)
    const error = new Error('Document download failed')
    handleFailure(
        selector => selector === '#error-icon-container' ? errorPanel : null,
        {},
        { error: value => errors.push(value) },
        error,
    )
    assert.equal(errorPanel.hidden, false)
    assert.deepEqual(errors, [error])
})

test('index and annotations panels are hidden and inert initially and after closing or switching', t => {
    assert.match(template, /<div id="side-bar"[^>]*\sinert hidden>/)
    assert.match(template, /<aside id="annotations-side-bar"[^>]*\sinert hidden>/)
    t.mock.method(globalThis, 'setTimeout', () => 0)
    const elements = new Map()
    for (const id of ['side-bar', 'annotations-side-bar', 'annotations-button',
        'annotations-side-bar-close', 'dimming-overlay']) {
        const classes = new Set()
        elements.set(id, {
            inert: id.endsWith('side-bar'),
            hidden: id.endsWith('side-bar'),
            classList: {
                add: name => classes.add(name),
                remove: name => classes.delete(name),
                contains: name => classes.has(name),
            },
            setAttribute() {},
            focus() {},
        })
    }
    const close = source.match(/    closeSideBar\(refocus = true\) \{([\s\S]*?)\n    \}/)[1]
    const open = source.match(/    #openSideBar\(id\) \{([\s\S]*?)\n    \}/)[1]
    const reader = new Function('$', 'document', `
        return new class {
            #sidebarOpening = false
            #skipNextPush = false
            closeSideBar(refocus = true) { ${close} }
            openSideBar(id) { ${open} }
        }()
    `)(selector => elements.get(selector.slice(1)), { getElementById: id => elements.get(id) })
    const index = elements.get('side-bar')
    const annotations = elements.get('annotations-side-bar')

    reader.openSideBar('side-bar')
    assert.equal(index.inert, false)
    assert.equal(index.hidden, false)
    assert.equal(index.classList.contains('show'), true)

    reader.closeSideBar()
    assert.equal(index.inert, true)
    assert.equal(index.hidden, true)
    assert.equal(index.classList.contains('show'), false)

    reader.openSideBar('side-bar')
    reader.openSideBar('annotations-side-bar')
    assert.equal(index.inert, true)
    assert.equal(index.hidden, true)
    assert.equal(index.classList.contains('show'), false)
    assert.equal(annotations.inert, false)
    assert.equal(annotations.hidden, false)
    assert.equal(annotations.classList.contains('show'), true)

    reader.closeSideBar()
    assert.equal(annotations.inert, true)
    assert.equal(annotations.hidden, true)
    assert.equal(annotations.classList.contains('show'), false)

    reader.openSideBar('annotations-side-bar')
    reader.openSideBar('side-bar')
    assert.equal(annotations.inert, true)
    assert.equal(annotations.hidden, true)
    assert.equal(annotations.classList.contains('show'), false)
    assert.equal(index.inert, false)
    assert.equal(index.hidden, false)
})
