import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('../embedded/js/reader-toast.js', import.meta.url), 'utf8')
const { ReaderToast } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

function setup(t, autoHide = true, viewport = null) {
    const classes = new Set()
    const button = new EventTarget()
    const message = {}
    const action = {}
    const book = {}
    const toast = {
        hidden: true,
        style: {},
        dataset: { autoHide: String(autoHide), delay: '5000' },
        classList: {
            add: value => classes.add(value),
            remove: (...values) => values.forEach(value => classes.delete(value)),
        },
        querySelector: selector => selector === '.toast-close' ? button :
            selector === '.toast-action' ? action : message,
        show: () => assert.fail('Notifications must not invoke dialog focusing steps'),
        close: () => assert.fail('Notifications must not invoke dialog focus restoration'),
    }
    for (const name of ['document', 'window', 'requestAnimationFrame']) {
        const descriptor = Object.getOwnPropertyDescriptor(globalThis, name)
        t.after(() => {
            if (descriptor) Object.defineProperty(globalThis, name, descriptor)
            else delete globalThis[name]
        })
    }
    globalThis.document = {
        activeElement: book, getElementById: () => toast,
        documentElement: { clientWidth: 768 },
    }
    globalThis.window = new EventTarget()
    window.visualViewport = viewport
    const frames = []
    globalThis.requestAnimationFrame = callback => frames.push(callback)
    const timers = new Map()
    let nextTimer = 0
    t.mock.method(globalThis, 'setTimeout', (callback, delay) => {
        timers.set(++nextTimer, { callback, delay })
        return nextTimer
    })
    t.mock.method(globalThis, 'clearTimeout', id => timers.delete(id))
    return {
        reader: new ReaderToast(),
        toast, button, message, action, classes, book, timers,
        render() {
            for (const callback of frames.splice(0)) callback()
        },
        expire() {
            const pending = [...timers.values()]
            timers.clear()
            for (const { callback } of pending) callback()
        },
    }
}

test('showing and auto-hiding a notification never moves reader focus', t => {
    const fixture = setup(t)
    fixture.reader.show('success', 'Annotation saved.')
    fixture.render()
    assert.equal(fixture.toast.hidden, false)
    assert.equal(fixture.message.innerHTML, 'Annotation saved.')
    assert.deepEqual([...fixture.classes], ['toast-success'])
    assert.equal(document.activeElement, fixture.book)
    assert.equal([...fixture.timers.values()][0].delay, 5000)
    fixture.expire()
    assert.equal(fixture.toast.hidden, true)
    assert.equal(document.activeElement, fixture.book)
})

test('replacement notifications reset variants and the auto-hide timer', t => {
    const fixture = setup(t)
    fixture.reader.show('success', 'Annotation saved.')
    fixture.render()
    const firstTimer = [...fixture.timers.keys()][0]
    fixture.reader.show('warning', 'Save failed.')
    fixture.render()
    assert.equal(fixture.toast.hidden, false)
    assert.deepEqual([...fixture.classes], ['toast-warning'])
    assert.equal(fixture.message.innerHTML, 'Save failed.')
    assert.equal(fixture.timers.has(firstTimer), false)
    assert.equal(fixture.timers.size, 1)
    assert.equal(document.activeElement, fixture.book)
})

test('the close button hides the notification and cancels auto-hide', t => {
    const fixture = setup(t)
    fixture.reader.show('info', 'Position updated.')
    fixture.render()
    fixture.button.dispatchEvent(new Event('click'))
    assert.equal(fixture.toast.hidden, true)
    assert.equal(fixture.timers.size, 0)
})

test('notifications with auto-hide disabled remain open', t => {
    const fixture = setup(t, false)
    fixture.reader.show('success', 'Annotation saved.')
    fixture.render()
    assert.equal(fixture.toast.hidden, false)
    assert.equal(fixture.timers.size, 0)
})

test('reload actions stay visible, run only on click, and clear on replacement', t => {
    const fixture = setup(t)
    let reloads = 0
    fixture.reader.show('info', 'A newer version is available.', {
        label: 'Reload document', onClick: () => { reloads++ },
    })
    fixture.render()
    assert.equal(reloads, 0)
    assert.equal(fixture.action.hidden, false)
    assert.equal(fixture.action.textContent, 'Reload document')
    assert.equal(fixture.timers.size, 0)
    fixture.action.onclick()
    assert.equal(reloads, 1)
    fixture.reader.show('warning', 'Document unavailable.')
    fixture.render()
    assert.equal(fixture.action.hidden, true)
    assert.equal(fixture.action.onclick, null)
    assert.equal(fixture.timers.size, 1)
})

test('the toast is bounded by the visible mobile viewport, not the wider layout viewport', t => {
    const viewport = Object.assign(new EventTarget(), { width: 384, offsetLeft: 0, offsetTop: 0 })
    const fixture = setup(t, true, viewport)
    fixture.reader.show('success', 'Anotación guardada.')
    fixture.render()
    const left = parseFloat(fixture.toast.style.left)
    const width = parseFloat(fixture.toast.style.width)
    assert.equal(left, 16)
    assert.equal(width, 352)
    assert.equal(left + width, viewport.width - 16)
})

test('the toast follows visible viewport resizing and panning without moving focus', t => {
    const viewport = Object.assign(new EventTarget(), { width: 384, offsetLeft: 0, offsetTop: 0 })
    const fixture = setup(t, true, viewport)
    fixture.reader.show('info', 'Position updated.')
    fixture.render()
    Object.assign(viewport, { width: 320, offsetLeft: 40, offsetTop: 24 })
    viewport.dispatchEvent(new Event('resize'))
    assert.deepEqual(fixture.toast.style, { left: '56px', top: '40px', width: '288px' })
    viewport.offsetLeft = 80
    viewport.dispatchEvent(new Event('scroll'))
    assert.equal(fixture.toast.style.left, '96px')
    assert.equal(document.activeElement, fixture.book)
})

test('browsers without VisualViewport use the document viewport and track resizing', t => {
    const fixture = setup(t)
    fixture.reader.show('warning', 'Save failed.')
    fixture.render()
    assert.equal(fixture.toast.style.width, '736px')
    document.documentElement.clientWidth = 320
    window.dispatchEvent(new Event('resize'))
    assert.equal(fixture.toast.style.width, '288px')
})
