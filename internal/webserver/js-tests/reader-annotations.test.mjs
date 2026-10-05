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
    hidden = true
    disabled = false
    get open() { return !this.hidden }
    style = { removeProperty(name) { delete this[name] } }
    attributes = new Map()
    focusCalls = []
    get lastElementChild() { return this.children.at(-1) }
    append(...children) { this.children.push(...children) }
    replaceChildren(...children) { this.children = children }
    setAttribute(name, value) { this.attributes.set(name, value) }
    getBoundingClientRect() { return { width: 240, height: 48 } }
    focus(options) { this.focusCalls.push(options) }
    contains(element) { return this === element || this.children.some(child => child.contains(element)) }
}

function setup(t, authenticated = true, { withList = false, allowWarnings = false, viewport } = {}) {
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
    globalThis.document = Object.assign(new EventTarget(), {
        body,
        documentElement: { clientWidth: 390, clientHeight: 844 },
        createElement: tag => Object.assign(new Element(), { tagName: tag.toUpperCase() }),
        getElementById: id => id === 'annotations-list' ? list :
            id === 'annotations-side-bar-close' ? listClose : null,
    })
    globalThis.window = new EventTarget()
    window.visualViewport = viewport
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
    const previews = new Map()
    view.renderer.getContents = () => [{
        doc,
        overlayer: {
            add: (key, range, draw, options) => previews.set(key, { range, options }),
            remove: key => previews.delete(key),
        },
    }]
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
            comment: 'Comment',
            comment_limit: 'Comments can contain up to 65,536 characters.',
            remove_annotation: 'Remove annotation', annotation_removed: 'Annotation removed.',
            annotation_remove_failed: 'Removal failed.', no_annotations: 'No annotations.',
            annotations_display_failed: 'Saved annotations could not be displayed.',
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
        get commentInput() { return body.children[0].children[0].children[0] },
        get commentText() { return body.children[0].children[1] },
        get actionButton() { return body.children[0].children[2].children[0] },
        ranges,
        previews,
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
    assert.deepEqual(reader.commentInput.focusCalls, [])
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
    assert.deepEqual(reader.commentInput.focusCalls, [])
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
    reader.actionButton.dispatchEvent(new Event('click'))
    assert.equal(reader.commentInput.disabled, true)
    await new Promise(setImmediate)
    assert.equal(reader.popup.open, false)
    assert.equal(reader.commentInput.disabled, false)
    assert.deepEqual(reader.focusCalls, [{ preventScroll: true }])
    assert.deepEqual(reader.notifications, [{ variant: 'success', message: 'Annotation saved.' }])
    assert.equal(requests[0].url, '/documents/test-book/annotations')
    assert.deepEqual(JSON.parse(requests[0].body), { cfi: 'cfi:Saved text', content: 'Saved text', comment: '' })
})

test('Ctrl/Cmd+Enter in the textarea saves once, including multiline and empty comments', async t => {
    const reader = setup(t)
    const requests = []
    t.mock.method(globalThis, 'fetch', async (url, options) => {
        requests.push(JSON.parse(options.body))
        return { status: 204, ok: true }
    })
    for (const [comment, modifier] of [['My comment\nSecond line', 'ctrlKey'], ['', 'metaKey']]) {
        const text = `Saved text ${requests.length}`
        reader.select(text)
        reader.settle()
        reader.commentInput.value = comment
        const enter = Object.assign(new Event('keydown', { cancelable: true }), { key: 'Enter', [modifier]: true })
        reader.commentInput.dispatchEvent(enter)
        assert.equal(enter.defaultPrevented, true)
        reader.commentInput.dispatchEvent(Object.assign(new Event('keydown'), { key: 'Enter', [modifier]: true }))
        await new Promise(setImmediate)
        assert.equal(reader.popup.open, false)
        assert.deepEqual(requests.at(-1), {
            cfi: `cfi:${text}`, content: text, comment,
        })
    }
    assert.equal(requests.length, 2)
})

test('plain Enter, composition, held keys, and consumed shortcuts do not save', t => {
    const reader = setup(t)
    t.mock.method(globalThis, 'fetch', () => assert.fail('Unexpected annotation save'))
    reader.select('Text')
    reader.settle()
    assert.equal(reader.commentInput.tagName, 'TEXTAREA')
    assert.equal(reader.commentInput.rows, 3)
    assert.equal(reader.popup.children[0].children.length, 1)
    for (const options of [{ ctrlKey: false }, { isComposing: true }, { repeat: true }, { key: 'a' }]) {
        const event = Object.assign(new Event('keydown', { cancelable: true }), { key: 'Enter', ctrlKey: true, ...options })
        reader.commentInput.dispatchEvent(event)
        assert.equal(event.defaultPrevented, false)
    }
    const consumed = Object.assign(new Event('keydown', { cancelable: true }), { key: 'Enter', ctrlKey: true })
    consumed.preventDefault()
    reader.commentInput.dispatchEvent(consumed)
    assert.equal(reader.popup.open, true)
})

for (const character of ['x', '\u00e9', '\u{1f600}']) {
    test(`comment limit counts Unicode characters for ${character}`, async t => {
        const reader = setup(t, true, { allowWarnings: true })
        const requests = []
        t.mock.method(globalThis, 'fetch', async (url, options) => {
            requests.push(JSON.parse(options.body))
            return { status: 204, ok: true }
        })
        reader.select('Selected passage')
        reader.settle()
        const comment = character.repeat(65536)
        reader.commentInput.value = comment + character
        reader.actionButton.dispatchEvent(new Event('click'))
        assert.equal(requests.length, 0)
        assert.equal(reader.popup.open, true)
        assert.equal(reader.commentInput.value, comment + character)
        assert.notEqual(reader.actionButton.disabled, true)
        assert.notEqual(reader.commentInput.disabled, true)
        assert.deepEqual(reader.notifications, [{
            variant: 'warning', message: 'Comments can contain up to 65,536 characters.',
        }])
        assert.equal(reader.commentInput.attributes.has('aria-describedby'), false)
        assert.equal(reader.popup.children[0].children.length, 1)
        reader.commentInput.value = comment
        reader.actionButton.dispatchEvent(new Event('click'))
        await new Promise(setImmediate)
        assert.equal(requests.length, 1)
        assert.equal(requests[0].comment, comment)
        assert.equal(reader.popup.open, false)
    })
}

test('a comment survives selection changes while typing and is shown after saving', async t => {
    const reader = setup(t)
    const requests = []
    t.mock.method(globalThis, 'fetch', async (url, options) => {
        requests.push(JSON.parse(options.body))
        return { status: 204, ok: true }
    })
    reader.select('Saved text')
    reader.settle()
    assert.equal(reader.popup.children[0].hidden, false)
    assert.equal(reader.commentText.hidden, true)
    reader.commentInput.value = '<img src=x onerror=alert(1)> My comment'
    document.activeElement = reader.commentInput
    reader.select('')
    reader.settle()
    assert.equal(reader.popup.open, true)
    assert.equal(reader.commentInput.value, '<img src=x onerror=alert(1)> My comment')
    reader.actionButton.dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.deepEqual(requests, [{
        cfi: 'cfi:Saved text', content: 'Saved text', comment: '<img src=x onerror=alert(1)> My comment',
    }])
    document.activeElement = null
    const range = reader.doc.defaultView.getSelection().getRangeAt(0).cloneRange()
    reader.view.dispatchEvent(new CustomEvent('show-annotation', {
        detail: { value: 'cfi:Saved text', range },
    }))
    assert.equal(reader.popup.children[0].hidden, true)
    assert.equal(reader.commentText.hidden, false)
    assert.equal(reader.commentText.textContent, '<img src=x onerror=alert(1)> My comment')
    assert.equal(reader.actionButton.textContent, 'Remove annotation')
})

test('comments are reset between new selections', t => {
    const reader = setup(t)
    reader.select('First')
    reader.settle()
    reader.commentInput.value = 'First comment'
    reader.select('Second')
    reader.settle()
    assert.equal(reader.commentInput.value, '')
})

test('manual textarea focus previews the captured passage without autofocus', t => {
    const reader = setup(t)
    reader.select('Selected passage')
    reader.settle()
    assert.equal(reader.previews.size, 0)
    assert.deepEqual(reader.commentInput.focusCalls, [])
    document.activeElement = reader.commentInput
    reader.commentInput.dispatchEvent(new Event('focus'))
    reader.select('')
    reader.settle()
    assert.equal(reader.popup.hidden, false)
    assert.equal(reader.previews.size, 1)
    const preview = [...reader.previews.values()][0]
    assert.equal(preview.range.text, 'Selected passage')
    assert.deepEqual(preview.options, { color: 'yellow' })
    reader.popup.children[2].children[1].dispatchEvent(new Event('click'))
    assert.equal(reader.previews.size, 0)
})

test('saving clears the preview and preserves the captured passage and multiline comment', async t => {
    const reader = setup(t)
    let saved
    t.mock.method(globalThis, 'fetch', async (url, options) => {
        saved = JSON.parse(options.body)
        return { status: 204, ok: true }
    })
    reader.select('Selected passage')
    reader.settle()
    document.activeElement = reader.commentInput
    reader.commentInput.dispatchEvent(new Event('focus'))
    reader.select('')
    reader.settle()
    reader.commentInput.value = 'First line\nSecond line'
    reader.actionButton.dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(reader.previews.size, 0)
    assert.deepEqual(saved, {
        cfi: 'cfi:Selected passage', content: 'Selected passage', comment: 'First line\nSecond line',
    })
})

test('preview follows new selections and is removed on navigation or session expiry', t => {
    const reader = setup(t)
    reader.select('First')
    reader.settle()
    document.activeElement = reader.commentInput
    reader.commentInput.dispatchEvent(new Event('focus'))
    reader.select('Second')
    reader.settle()
    assert.equal(reader.previews.size, 1)
    assert.equal([...reader.previews.values()][0].range.text, 'Second')
    reader.view.renderer.dispatchEvent(new CustomEvent('relocate', { detail: { index: 2, fraction: 0 } }))
    reader.view.renderer.dispatchEvent(new CustomEvent('relocate', { detail: { index: 2, fraction: 0.5 } }))
    assert.equal(reader.previews.size, 0)
    reader.select('Third')
    reader.settle()
    assert.equal(reader.previews.size, 1)
    window.dispatchEvent(new Event('reader-session-expired'))
    assert.equal(reader.previews.size, 0)
})

test('preview errors are reported without losing the pending comment', t => {
    const reader = setup(t, true, { allowWarnings: true })
    t.mock.method(console, 'error', () => {})
    t.mock.method(reader.view.renderer, 'getContents', () => [])
    reader.select('Text')
    reader.settle()
    reader.commentInput.value = 'Comment'
    reader.commentInput.dispatchEvent(new Event('focus'))
    assert.equal(reader.popup.hidden, false)
    assert.equal(reader.commentInput.value, 'Comment')
    assert.equal(reader.notifications[0].variant, 'warning')
})

test('failed saves preserve the entered comment for retry', async t => {
    const reader = setup(t, true, { allowWarnings: true })
    t.mock.method(globalThis, 'fetch', async () => ({ status: 500, ok: false }))
    t.mock.method(console, 'error', () => {})
    reader.select('Text')
    reader.settle()
    reader.commentInput.value = 'Keep this comment'
    document.activeElement = reader.commentInput
    reader.commentInput.dispatchEvent(new Event('focus'))
    reader.actionButton.dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(reader.popup.open, true)
    assert.equal(reader.commentInput.value, 'Keep this comment')
    assert.equal(reader.previews.size, 1)
    assert.equal(reader.actionButton.disabled, false)
    assert.equal(reader.commentInput.disabled, false)
    assert.equal(reader.notifications[0].variant, 'warning')
})

test('cancel closes the popup and clears the native selection without saving', t => {
    const reader = setup(t)
    t.mock.method(globalThis, 'fetch', () => assert.fail('Cancel must not save an annotation'))
    reader.select('Selected text')
    reader.settle()
    reader.popup.children[2].children[1].dispatchEvent(new Event('click'))
    assert.equal(reader.popup.open, false)
    assert.equal(reader.doc.defaultView.getSelection().isCollapsed, true)
})

test('page movement and session expiration close the popup', t => {
    const reader = setup(t)
    reader.view.renderer.dispatchEvent(new CustomEvent('relocate', {
        detail: { index: 2, fraction: 0 },
    }))
    reader.select('Selected text')
    reader.settle()
    reader.view.renderer.dispatchEvent(new CustomEvent('relocate', {
        detail: { index: 2, fraction: 0.5 },
    }))
    assert.equal(reader.popup.open, false)
    reader.select('Another passage')
    reader.settle()
    window.dispatchEvent(new Event('reader-session-expired'))
    assert.equal(reader.popup.open, false)
})

test('opening the popup preserves native selection without moving focus or requiring an overlay', t => {
    const reader = setup(t)
    t.mock.method(reader.view, 'addAnnotation', () => assert.fail('Opening must not add a highlight'))
    reader.select('Selected text')
    reader.settle()
    assert.equal(reader.popup.open, true)
    assert.equal(reader.doc.defaultView.getSelection().toString(), 'Selected text')
    assert.deepEqual(reader.commentInput.focusCalls, [])
    assert.deepEqual(reader.focusCalls, [])
})

test('popup uses a non-modal dialog role and Escape dismisses it from the outer document', t => {
    const reader = setup(t)
    reader.select('Text')
    reader.settle()
    assert.equal(reader.popup.hidden, false)
    assert.equal(reader.popup.attributes.get('role'), 'dialog')
    const escape = Object.assign(new Event('keydown', { cancelable: true }), { key: 'Escape' })
    document.dispatchEvent(escape)
    assert.equal(escape.defaultPrevented, true)
    assert.equal(reader.popup.hidden, true)
    assert.deepEqual(reader.focusCalls, [{ preventScroll: true }])
})

test('a new nonempty selection is processed even while the popup has focus', t => {
    const reader = setup(t)
    reader.select('First')
    reader.settle()
    document.activeElement = reader.commentInput
    reader.select('Second')
    reader.settle()
    assert.equal(reader.ranges.at(-1).text, 'Second')
    assert.equal(reader.popup.hidden, false)
})

test('popup follows the visible viewport on resize and panning without moving focus', t => {
    const viewport = Object.assign(new EventTarget(), {
        width: 300, height: 300, offsetLeft: 10, offsetTop: 20,
    })
    const reader = setup(t, true, { viewport })
    reader.select('Text')
    reader.settle()
    assert.equal(reader.popup.style.maxWidth, '284px')
    assert.equal(reader.popup.style.maxHeight, '284px')
    viewport.width = 260
    viewport.offsetLeft = 30
    viewport.dispatchEvent(new Event('resize'))
    assert.equal(reader.popup.style.maxWidth, '244px')
    assert.ok(parseFloat(reader.popup.style.left) >= 38)
    assert.ok(parseFloat(reader.popup.style.left) + 240 <= 282)
    viewport.offsetTop = 40
    viewport.dispatchEvent(new Event('scroll'))
    assert.ok(parseFloat(reader.popup.style.top) >= 48)
    assert.deepEqual(reader.commentInput.focusCalls, [])
})

for (const remove of [false, true]) {
    test(`overlay failure after ${remove ? 'deletion' : 'saving'} preserves committed state and reports only a warning`, async t => {
        const reader = setup(t, true, { withList: true, allowWarnings: true })
        t.mock.method(console, 'error', () => {})
        t.mock.method(globalThis, 'fetch', async (url, options) => options
            ? { status: 204, ok: true }
            : { status: 200, ok: true, json: async () => [{ cfi: FIRST, content: 'Text' }] })
        t.mock.method(reader.view, remove ? 'deleteAnnotation' : 'addAnnotation',
            async () => { throw new Error('Overlay unavailable') })
        if (remove) {
            await reader.annotations.load()
            reader.list.children[0].children[0].children[1].dispatchEvent(new Event('click'))
        } else {
            reader.select('Text')
            reader.settle()
            reader.actionButton.dispatchEvent(new Event('click'))
        }
        await new Promise(setImmediate)
        assert.equal(reader.popup.hidden, true)
        assert.equal(reader.actionButton.disabled, false)
        assert.equal(reader.commentInput.disabled, false)
        assert.deepEqual(reader.notifications, [{
            variant: 'warning', message: 'Saved annotations could not be displayed.',
        }])
        if (remove) assert.equal(reader.list.children[0].textContent, 'No annotations.')
        else assert.equal(reader.list.children[0].children[0].children[0].children[0].textContent, 'Text')
    })
}

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

for (const status of [401, 403]) {
    test(`HTTP ${status} during deletion clears the popup and panel and restores controls`, async t => {
        const { reader, rows } = await loadList(t,
            [{ cfi: FIRST, content: 'First annotation' }], { status, ok: false })
        const range = reader.doc.defaultView.getSelection().getRangeAt(0).cloneRange()
        reader.view.dispatchEvent(new CustomEvent('show-annotation', { detail: { value: FIRST, range } }))
        const remove = rows()[0].children[1]
        remove.dispatchEvent(new Event('click'))
        assert.equal(remove.disabled, true)
        assert.equal(reader.commentInput.disabled, true)
        await new Promise(setImmediate)
        assert.equal(reader.sync.isAuthenticated, false)
        assert.equal(reader.popup.open, false)
        assert.equal(reader.list.children.length, 0)
        assert.equal(remove.disabled, false)
        assert.equal(reader.commentInput.disabled, false)
        assert.equal(reader.deleted.length, 0)
        assert.equal(reader.notifications.length, 0)
        reader.sync.isAuthenticated = true
        reader.select('New passage')
        reader.settle()
        assert.equal(reader.popup.open, true, 'Expired submission must release the saving lock')
    })

    test(`HTTP ${status} during saving closes the popup and allows saving after reauthentication`, async t => {
        const reader = setup(t)
        let requests = 0
        t.mock.method(globalThis, 'fetch', async () => {
            requests++
            return requests === 1 ? { status, ok: false } : { status: 204, ok: true }
        })
        reader.select('Selected passage')
        reader.settle()
        reader.commentInput.value = 'Comment'
        reader.actionButton.dispatchEvent(new Event('click'))
        assert.equal(reader.actionButton.disabled, true)
        assert.equal(reader.commentInput.disabled, true)
        await new Promise(setImmediate)
        assert.equal(reader.sync.isAuthenticated, false)
        assert.equal(reader.popup.open, false)
        assert.equal(reader.actionButton.disabled, false)
        assert.equal(reader.commentInput.disabled, false)
        assert.equal(reader.notifications.length, 0)
        assert.equal(reader.deleted.length, 0)
        reader.actionButton.dispatchEvent(new Event('click'))
        assert.equal(requests, 1, 'Expired submissions must clear the pending annotation')
        reader.sync.isAuthenticated = true
        reader.actionButton.dispatchEvent(new Event('click'))
        assert.equal(requests, 1, 'Reauthentication must not restore stale pending annotations')
        reader.select('New passage')
        reader.settle()
        assert.equal(reader.popup.open, true)
        reader.actionButton.dispatchEvent(new Event('click'))
        await new Promise(setImmediate)
        assert.equal(requests, 2)
        assert.equal(reader.popup.open, false)
        assert.deepEqual(reader.notifications, [{ variant: 'success', message: 'Annotation saved.' }])
    })
}

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
    reader.actionButton.dispatchEvent(new Event('click'))
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

test('clicking a saved highlight still opens the removal popup and deletes the correct annotation', async t => {
    const { reader, requests } = await loadList(t, [{ cfi: FIRST, content: 'First annotation' }])
    const range = reader.doc.defaultView.getSelection().getRangeAt(0).cloneRange()
    reader.view.dispatchEvent(new CustomEvent('show-annotation', { detail: { value: FIRST, range } }))
    assert.equal(reader.popup.open, true)
    assert.equal(reader.commentText.hidden, true)
    assert.equal(reader.actionButton.textContent, 'Remove annotation')
    reader.actionButton.dispatchEvent(new Event('click'))
    await new Promise(setImmediate)
    assert.equal(requests[0].method, 'DELETE')
    assert.deepEqual(JSON.parse(requests[0].body), { cfi: FIRST })
    assert.deepEqual(reader.deleted, [{ value: FIRST, content: 'First annotation' }])
    assert.equal(reader.popup.open, false)
    assert.equal(reader.list.children[0].textContent, 'No annotations.')
    assert.deepEqual(reader.focusCalls, [{ preventScroll: true }])
})

test('loaded annotation comments are displayed as text in the removal popup', async t => {
    const { reader, requests } = await loadList(t, [{
        cfi: FIRST, content: 'First annotation', comment: '<b>A saved comment</b>',
    }])
    const range = reader.doc.defaultView.getSelection().getRangeAt(0).cloneRange()
    reader.view.dispatchEvent(new CustomEvent('show-annotation', { detail: { value: FIRST, range } }))
    assert.equal(reader.commentText.hidden, false)
    assert.equal(reader.commentText.textContent, '<b>A saved comment</b>')
    assert.equal(reader.popup.children[0].hidden, true)
    assert.deepEqual(reader.commentInput.focusCalls, [])
    reader.commentInput.dispatchEvent(Object.assign(new Event('keydown'), { key: 'Enter', ctrlKey: true }))
    await new Promise(setImmediate)
    assert.equal(requests.length, 0)
})

test('invalid comment response types produce a loading warning', async t => {
    const reader = setup(t, true, { allowWarnings: true })
    t.mock.method(console, 'error', () => {})
    t.mock.method(globalThis, 'fetch', async () => ({
        status: 200, ok: true,
        json: async () => [{ cfi: FIRST, content: 'Text', comment: 123 }],
    }))
    await reader.annotations.load()
    assert.equal(reader.notifications[0].variant, 'warning')
})
