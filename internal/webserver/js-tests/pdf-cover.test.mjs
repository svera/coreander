import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import vm from 'node:vm'

const source = await readFile(new URL('../embedded/js/pdf-cover.js', import.meta.url), 'utf8')
const { createPDFCoverRenderer } = await import(`data:text/javascript;base64,${Buffer.from(
    source.replace("import { importVersioned } from './asset-version.js'", '')
        .replaceAll('import.meta.url', JSON.stringify(new URL('../embedded/js/pdf-cover.js', import.meta.url).href))
).toString('base64')}`)
const coverSource = await readFile(new URL('../embedded/js/cover.js', import.meta.url), 'utf8')

function setup(t, { failLoad = false, failRender = false, failBlob = false } = {}) {
    const calls = []
    const blob = new Blob(['first-page'], { type: 'image/webp' })
    const canvas = {
        getContext: () => 'context',
        toBlob: (callback, type, quality) => {
            calls.push(['blob', type, quality])
            callback(failBlob ? null : blob)
        },
    }
    const descriptor = Object.getOwnPropertyDescriptor(globalThis, 'document')
    Object.defineProperty(globalThis, 'document', { configurable: true, value: {
        createElement: name => {
            assert.equal(name, 'canvas')
            return canvas
        },
    } })
    t.after(() => {
        if (descriptor) Object.defineProperty(globalThis, 'document', descriptor)
        else delete globalThis.document
    })
    const page = {
        getViewport: ({ scale }) => ({ width: 800 * scale, height: 1000 * scale }),
        render: options => {
            calls.push(['render', options])
            return { promise: failRender ? Promise.reject(new Error('render failed')) : Promise.resolve() }
        },
    }
    const pdfjs = {
        getDocument: options => {
            calls.push(['document', options])
            return {
                promise: failLoad ? Promise.reject(new Error('load failed')) : Promise.resolve({
                    getPage: async number => {
                        calls.push(['page', number])
                        return page
                    },
                }),
                destroy: async () => { calls.push(['destroy']) },
            }
        },
    }
    return { renderer: createPDFCoverRenderer(async () => pdfjs), calls, blob, canvas }
}

test('PDF covers render exactly page 1 at the configured width with range loading', async t => {
    const { renderer, calls, blob, canvas } = setup(t)
    assert.equal(await renderer('/documents/book/download', 600), blob)
    assert.equal(canvas.width, 600)
    assert.equal(canvas.height, 750)
    assert.deepEqual(calls.filter(([name]) => name === 'page'), [['page', 1]])
    const options = calls.find(([name]) => name === 'document')[1]
    assert.equal(options.url, '/documents/book/download')
    assert.equal(options.disableAutoFetch, true)
    assert.equal(options.disableStream, true)
    assert.equal(options.isEvalSupported, false)
    assert.deepEqual(calls.at(-1), ['destroy'])
    assert.deepEqual(calls.find(([name]) => name === 'blob'), ['blob', 'image/webp', 0.8])
})

test('duplicate covers share a render, and different PDFs are processed serially', async t => {
    const { renderer, calls } = setup(t)
    const first = renderer('/one', 600)
    assert.equal(renderer('/one', 600), first)
    const second = renderer('/two', 600)
    await Promise.all([first, second])
    assert.equal(calls.filter(([name]) => name === 'document').length, 2)
    assert.ok(calls.findIndex(([name]) => name === 'destroy') <
        calls.findIndex(([name, options]) => name === 'document' && options.url === '/two'))
    assert.equal(renderer('/one', 600), first)
})

test('zero cover width preserves the first page dimensions', async t => {
    const { renderer, canvas } = setup(t)
    await renderer('/book', 0)
    assert.equal(canvas.width, 800)
    assert.equal(canvas.height, 1000)
})

for (const failure of ['failLoad', 'failRender', 'failBlob']) {
    test(`${failure} is surfaced, destroys the PDF and allows retrying`, async t => {
        const { renderer, calls } = setup(t, { [failure]: true })
        const first = renderer('/broken', 600)
        await assert.rejects(first, /failed|encode/)
        assert.deepEqual(calls.at(-1), ['destroy'])
        const retry = renderer('/broken', 600)
        assert.notEqual(retry, first)
        await assert.rejects(retry, /failed|encode/)
        assert.equal(calls.filter(([name]) => name === 'document').length, 2)
    })
}

test('the thumbnail cache is bounded and distinguishes widths', async t => {
    const { renderer, calls } = setup(t)
    await renderer('/book', 300)
    await renderer('/book', 600)
    for (let i = 0; i < 32; i++) await renderer(`/book-${i}`, 600)
    await renderer('/book', 300)
    assert.equal(calls.filter(([name]) => name === 'document').length, 35)
})

function setupCovers(renderPDFCover) {
    const images = []
    const observed = []
    const logs = []
    const revoked = []
    const overlay = { removed: false, hidden: true,
        remove() { this.removed = true },
        classList: { remove() { overlay.hidden = false } },
    }
    const makeElement = (attributes = {}) => {
        const handlers = {}
        const classes = new Set()
        return {
            src: 'generic.webp',
            getAttribute: name => attributes[name] ?? null,
            hasAttribute: name => name in attributes,
            classList: { contains: name => classes.has(name), add: name => classes.add(name) },
            animate() {},
            addEventListener: (name, callback) => { handlers[name] = callback },
            handlers,
        }
    }
    const elements = []
    const context = {
        console: { error: (...args) => logs.push(args) },
        importVersioned: async () => ({ renderPDFCover }),
        URL: { createObjectURL: () => 'blob:cover', revokeObjectURL: url => revoked.push(url) },
        Image: class {
            handlers = {}
            constructor() { images.push(this) }
            addEventListener(name, callback) { this.handlers[name] = callback }
        },
        IntersectionObserver: class {
            constructor(callback) { context.intersect = callback }
            observe(elem) { observed.push(elem) }
            unobserve() {}
        },
        MutationObserver: class { observe() {} },
        document: {
            querySelector: () => ({ content: '600' }),
            querySelectorAll: () => elements,
            getElementById: () => overlay,
            getElementsByTagName: () => [{}],
            addEventListener() {},
            body: { addEventListener() {} },
        },
    }
    vm.createContext(context)
    vm.runInContext(coverSource.replace("import { importVersioned } from './asset-version.js'", '') +
        '\nglobalThis.loadCover = loadCover; globalThis.coversLoader = coversLoader', context)
    return { context, images, overlay, observed, elements, makeElement, logs, revoked }
}

test('PDF thumbnails use rendered blobs, remove overlays and revoke object URLs', async () => {
    const env = setupCovers(async (url, width) => {
        assert.equal(url, '/book/download')
        assert.equal(width, 600)
        return new Blob(['cover'])
    })
    const elem = env.makeElement({ 'data-pdf-src': '/book/download', 'data-cover-title-id': 'title' })
    await env.context.loadCover(elem)
    assert.equal(env.images[0].src, 'blob:cover')
    env.images[0].handlers.load()
    assert.equal(elem.src, 'blob:cover')
    assert.equal(env.overlay.removed, true)
    elem.handlers.load()
    assert.deepEqual(env.revoked, ['blob:cover'])
})

test('render failures keep the generic cover and title, and log the error', async () => {
    const env = setupCovers(async () => { throw new Error('PDF unavailable') })
    const elem = env.makeElement({ 'data-pdf-src': '/book/download', 'data-cover-title-id': 'title' })
    await env.context.loadCover(elem)
    assert.equal(elem.src, 'generic.webp')
    assert.equal(env.overlay.removed, false)
    assert.equal(env.overlay.hidden, false)
    assert.equal(env.logs.length, 1)
    assert.equal(env.images.length, 0)
})

test('EPUB covers still load server images, while PDF covers support eager and lazy loading', async () => {
    const env = setupCovers(async () => new Blob(['cover']))
    const epub = env.makeElement({ 'data-src': '/epub/cover', 'data-cover-title-id': 'epub-title' })
    await env.context.loadCover(epub)
    assert.equal(env.images[0].src, '/epub/cover')
    env.images[0].handlers.load()
    assert.equal(epub.src, '/epub/cover')
    const eager = env.makeElement({ 'data-pdf-src': '/eager/download', 'data-cover-eager': '' })
    const lazy = env.makeElement({ 'data-pdf-src': '/lazy/download' })
    env.elements.push(eager, lazy, epub)
    env.context.coversLoader()
    assert.deepEqual(env.observed, [lazy, epub])
    env.context.coversLoader()
    assert.deepEqual(env.observed, [lazy, epub])
})
