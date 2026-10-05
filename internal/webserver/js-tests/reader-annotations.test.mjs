import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('../embedded/js/reader-annotations.js', import.meta.url), 'utf8')
const cfiSource = await readFile(new URL('../embedded/js/foliate-js/epubcfi.js', import.meta.url), 'utf8')
const moduleURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const { ReaderAnnotations } = await import(moduleURL(
    source.replace("'./foliate-js/epubcfi.js'", JSON.stringify(moduleURL(cfiSource)))))
const FIRST = 'epubcfi(/6/2!/4/2/1:0)'
const SECOND = 'epubcfi(/6/4!/4/2/1:0)'
const LAST = 'epubcfi(/6/10!/4/2/1:0)'

class Element extends EventTarget {
    children = []
    open = false
    style = { removeProperty(name) { delete this[name] } }
    attributes = new Map()
    focusCalls = []
    get lastElementChild() { return this.children.at(-1) }
    append(...children) { this.children.push(...children) }
    replaceChildren(...children) { this.children = children }
    setAttribute(name, value) { this.attributes.set(name, value) }
    getBoundingClientRect() { return { width: 240, height: 48 } }
    close() { this.open = false }
    show() { this.open = true }
    focus(options) { this.focusCalls.push(options) }
}

function setup(t, authenticated = true, { withList = false, allowWarnings = false } = {}) {
    const body = new Element()
    const list = withList ? new Element() : null
    const listClose = new Element()
    for (const name of ['document', 'window']) {
        const descriptor = Object.getOwnPropertyDescriptor(globalThis, name)
        t.after(() => {
            if (descriptor) Object.defineProperty(globalThis, name, descriptor)
            else delete globalThis[name]
        })
    }
    globalThis.document = {
        body,
        documentElement: { clientWidth: 390, clientHeight: 844 },
        createElement: () => new Element(),
        getElementById: id => id === 'annotations-list' ? list :
            id === 'annotations-side-bar-close' ? listClose : null,
    }
    globalThis.window = new EventTarget()
    const timers = new Map()
    let nextTimer = 0
    t.mock.method(globalThis, 'setTimeout', (callback, delay) => {
        timers.set(++nextTimer, { callback, delay })
        return nextTimer
    })
    t.mock.method(globalThis, 'clearTimeout', id => timers.delete(id))
    const settle = () => {
        const pending = [...timers.values()]
        timers.clear()
        for (const { callback } of pending) callback()
    }
    const doc = new EventTarget()
    let text = ''
    const selection = {
        get rangeCount() { return text ? 1 : 0 },
        get isCollapsed() { return !text },
        toString: () => text,
        getRangeAt: () => ({
            cloneRange: () => ({
                text,
                startContainer: { ownerDocument: doc },
                getClientRects: () => [{ left: 20, right: 250, top: 100, bottom: 125 }],
            }),
        }),
    }
    doc.defaultView = { getSelection: () => selection }
    const view = new EventTarget()
    view.renderer = new EventTarget()
    const ranges = []
    const focusCalls = []
    const notifications = []
    const deleted = []
    let navigations = 0
    view.getCFI = (index, range) => {
        ranges.push({ index, text: range.text })
        return `cfi:${range.text}`
    }
    view.deselect = () => { text = '' }
    view.focus = options => focusCalls.push(options)
    view.resolveNavigation = async () => ({ index: 2 })
    view.addAnnotation = async () => {}
    view.deleteAnnotation = async annotation => deleted.push(annotation)
    view.goTo = () => assert.fail('Saving an annotation must not navigate the book')
    const sync = { isAuthenticated: authenticated }
    const annotations = new ReaderAnnotations({
        view,
        sync,
        translations: {
            save_annotation: 'Save annotation', cancel: 'Cancel', annotation_saved: 'Annotation saved.',
            remove_annotation: 'Remove annotation', annotation_removed: 'Annotation removed.',
            annotation_remove_failed: 'Removal failed.', no_annotations: 'No annotations.',
        },
        notify: (variant, message) => {
            if (!allowWarnings) assert.notEqual(variant, 'warning', 'Unexpected annotation warning')
            notifications.push({ variant, message })
        },
        slug: 'test-book',
        onNavigate: () => navigations++,
    })
    view.dispatchEvent(new CustomEvent('load', { detail: { doc, index: 2 } }))
    return {
        doc,
        popup: body.children[0],
        ranges,
        focusCalls,
        notifications,
        annotations,
        list,
        listClose,
        view,
        sync,
        deleted,
        get navigations() { return navigations },
        settle,
        timers,
        select(value) {
            text = value
            doc.dispatchEvent(new Event('selectionchange'))
        },
        dispatch(type, properties = {}) {
            doc.dispatchEvent(Object.assign(new Event(type), properties))
        },
    }
}

test('mobile selection changes open the popup without a pointerup event', t => {
    const reader = setup(t)
    reader.select('Selected text')
    assert.equal(reader.popup.open, false)
    assert.equal([...reader.timers.values()][0].delay, 150)
    reader.settle()
    assert.equal(reader.popup.open, true)
    assert.equal(reader.popup.attributes.get('aria-label'), 'Save annotation')
    assert.deepEqual(reader.ranges, [{ index: 2, text: 'Selected text' }])
})

test('long-press pointer cancellation does not discard the native selection', t => {
    const reader = setup(t)
    reader.dispatch('pointerdown')
    reader.select('Long press')
    reader.dispatch('pointercancel')
    reader.settle()
    assert.equal(reader.popup.open, true)
})

test('selection arriving after touchend or touchcancel still opens the popup', t => {
    const reader = setup(t)
    for (const type of ['touchend', 'touchcancel']) {
        reader.dispatch('touchstart')
        reader.dispatch(type, { touches: [] })
        reader.settle()
        assert.equal(reader.popup.open, false)
        reader.select('Native selection')
        reader.settle()
        assert.equal(reader.popup.open, true)
        reader.select('')
        reader.settle()
    }
})

test('handle adjustments are debounced and use the latest selection', t => {
    const reader = setup(t)
    reader.select('Initial text')
    reader.settle()
    reader.select('Expanded text')
    reader.select('Final selected text')
    assert.equal(reader.timers.size, 1)
    reader.settle()
    assert.equal(reader.popup.open, true)
    assert.deepEqual(reader.ranges.at(-1), { index: 2, text: 'Final selected text' })
})

test('desktop mouse selection waits for release', t => {
    const reader = setup(t)
    reader.dispatch('pointerdown')
    reader.select('Mouse drag')
    reader.settle()
    assert.equal(reader.popup.open, false)
    reader.dispatch('pointerup', { button: 0 })
    reader.settle()
    assert.equal(reader.popup.open, true)
})

test('clearing selection closes the add popup', t => {
    const reader = setup(t)
    reader.select('Text')
    reader.settle()
    reader.select('')
    reader.settle()
    assert.equal(reader.popup.open, false)
})

test('anonymous readers do not get an annotation popup', t => {
    const reader = setup(t, false)
    reader.select('Text')
    reader.settle()
    assert.equal(reader.popup, undefined)
    assert.equal(reader.ranges.length, 0)
})

test('saving restores reader focus without scrolling or navigating', async t => {
    const reader = setup(t)
    const requests = []
    t.mock.method(globalThis, 'fetch', async (url, options) => {
        requests.push({ url, ...options })
        return { status: 201, ok: true }
    })
    reader.select('Saved text')
    reader.settle()
    reader.popup.children[0].dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(reader.popup.open, false)
    assert.deepEqual(reader.focusCalls, [{ preventScroll: true }])
    assert.deepEqual(reader.notifications, [{ variant: 'success', message: 'Annotation saved.' }])
    assert.equal(requests[0].url, '/documents/test-book/annotations')
    assert.deepEqual(JSON.parse(requests[0].body), { cfi: 'cfi:Saved text', content: 'Saved text' })
})

async function loadList(t, entries, deleteResponse = { status: 204, ok: true }) {
    const reader = setup(t, true, { withList: true, allowWarnings: !deleteResponse.ok })
    const requests = []
    t.mock.method(globalThis, 'fetch', async (url, options) => {
        if (!options) return { status: 200, ok: true, json: async () => entries }
        requests.push({ url, ...options })
        return deleteResponse
    })
    await reader.annotations.load()
    return { reader, requests, rows: () => reader.list.children[0].children }
}

test('each panel entry has a separate, accessible delete cross', async t => {
    const { rows } = await loadList(t, [{ cfi: FIRST, content: 'First annotation' }])
    const [navigate, remove] = rows()[0].children
    assert.equal(navigate.children[0].textContent, 'First annotation')
    assert.equal(remove.className, 'annotation-remove')
    assert.equal(remove.textContent, '\u00d7')
    assert.equal(remove.attributes.get('aria-label'), 'Remove annotation')
    assert.equal(remove.type, 'button')
})

test('the cross deletes only its entry and highlight, keeps the panel open, and prevents duplicate requests', async t => {
    const { reader, requests, rows } = await loadList(t, [
        { cfi: FIRST, content: 'First annotation' },
        { cfi: SECOND, content: 'Second annotation' },
    ])
    const remove = rows()[0].children[1]
    remove.dispatchEvent(new Event('click'))
    assert.equal(remove.disabled, true)
    remove.dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(requests.length, 1)
    assert.equal(requests[0].method, 'DELETE')
    assert.equal(requests[0].url, '/documents/test-book/annotations')
    assert.deepEqual(JSON.parse(requests[0].body), { cfi: FIRST })
    assert.equal(rows().length, 1)
    assert.equal(rows()[0].children[0].children[0].textContent, 'Second annotation')
    assert.deepEqual(reader.deleted, [{ value: FIRST, content: 'First annotation' }])
    assert.deepEqual(rows()[0].children[1].focusCalls, [{ preventScroll: true }])
    assert.equal(reader.focusCalls.length, 0)
    assert.equal(reader.navigations, 0)
    assert.deepEqual(reader.notifications, [{ variant: 'success', message: 'Annotation removed.' }])
})

test('deleting the last entry shows the empty state and focuses the panel close button', async t => {
    const { reader, rows } = await loadList(t, [{ cfi: LAST, content: 'Last annotation' }])
    rows()[0].children[1].dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(reader.list.children[0].textContent, 'No annotations.')
    assert.deepEqual(reader.listClose.focusCalls, [{ preventScroll: true }])
})

test('deleting the final row focuses the previous entry without scrolling', async t => {
    const { rows } = await loadList(t, [
        { cfi: FIRST, content: 'First annotation' },
        { cfi: LAST, content: 'Last annotation' },
    ])
    rows()[1].children[1].dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(rows().length, 1)
    assert.equal(rows()[0].children[0].children[0].textContent, 'First annotation')
    assert.deepEqual(rows()[0].children[1].focusCalls, [{ preventScroll: true }])
})

test('a failed deletion preserves the entry and highlight and re-enables the cross', async t => {
    t.mock.method(console, 'error', () => {})
    const { reader, rows } = await loadList(t,
        [{ cfi: FIRST, content: 'First annotation' }], { status: 500, ok: false })
    const remove = rows()[0].children[1]
    remove.dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(rows().length, 1)
    assert.equal(rows()[0].children[1], remove)
    assert.equal(remove.disabled, false)
    assert.equal(reader.deleted.length, 0)
    assert.deepEqual(reader.notifications, [{ variant: 'warning', message: 'Removal failed.' }])
})

test('an expired session clears the panel without deleting local highlights or reporting success', async t => {
    const { reader, rows } = await loadList(t,
        [{ cfi: FIRST, content: 'First annotation' }], { status: 403, ok: false })
    rows()[0].children[1].dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(reader.sync.isAuthenticated, false)
    assert.equal(reader.list.children.length, 0)
    assert.equal(reader.deleted.length, 0)
    assert.equal(reader.notifications.length, 0)
})

test('clicking annotation text still navigates without deleting it', async t => {
    const { reader, requests, rows } = await loadList(t, [{ cfi: FIRST, content: 'First annotation' }])
    const locations = []
    t.mock.method(reader.view, 'goTo', async cfi => {
        locations.push(cfi)
        return { index: 2 }
    })
    rows()[0].children[0].dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.deepEqual(locations, [FIRST])
    assert.equal(reader.navigations, 1)
    assert.equal(requests.length, 0)
    assert.equal(rows().length, 1)
})

test('the panel sorts CFI chapters, numeric offsets, and ranges in book order', async t => {
    const { rows } = await loadList(t, [
        { cfi: LAST, content: 'Chapter ten' },
        { cfi: 'epubcfi(/6/2!/4/2/1:10)', content: 'Offset ten' },
        { cfi: 'epubcfi(/6/2!/4/2,/1:2,/1:5)', content: 'Long range at offset two' },
        { cfi: FIRST, content: 'First annotation' },
        { cfi: 'epubcfi(/6/2!/4/2,/1:2,/1:3)', content: 'Short range at offset two' },
        { cfi: SECOND, content: 'Chapter two' },
    ])
    assert.deepEqual(rows().map(row => row.children[0].children[0].textContent), [
        'First annotation', 'Short range at offset two', 'Long range at offset two',
        'Offset ten', 'Chapter two', 'Chapter ten',
    ])
})

test('saving an earlier annotation inserts it at its CFI position', async t => {
    const { reader, rows } = await loadList(t, [
        { cfi: LAST, content: 'Last annotation' },
        { cfi: SECOND, content: 'Second annotation' },
    ])
    t.mock.method(reader.view, 'getCFI', () => FIRST)
    reader.select('New first annotation')
    reader.settle()
    reader.popup.children[0].dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.deepEqual(rows().map(row => row.children[0].children[0].textContent), [
        'New first annotation', 'Second annotation', 'Last annotation',
    ])
})

test('deleting from the sorted panel targets the displayed entry and preserves order', async t => {
    const { reader, requests, rows } = await loadList(t, [
        { cfi: LAST, content: 'Last annotation' },
        { cfi: FIRST, content: 'First annotation' },
        { cfi: SECOND, content: 'Second annotation' },
    ])
    rows()[1].children[1].dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.deepEqual(JSON.parse(requests[0].body), { cfi: SECOND })
    assert.deepEqual(reader.deleted, [{ value: SECOND, content: 'Second annotation' }])
    assert.deepEqual(rows().map(row => row.children[0].children[0].textContent), [
        'First annotation', 'Last annotation',
    ])
    assert.deepEqual(rows()[1].children[1].focusCalls, [{ preventScroll: true }])
})
