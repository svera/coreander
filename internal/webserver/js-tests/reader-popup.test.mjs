import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('../embedded/js/reader-popup.js', import.meta.url), 'utf8')
const { ReaderPopup } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

class Element extends EventTarget {
    hidden = false
    open = false
    className = ''
    attributes = new Map()
    style = { removeProperty(name) { delete this[name] } }
    modalCalls = 0
    closeCalls = 0
    focusCalls = 0
    setAttribute(name, value) { this.attributes.set(name, value) }
    showModal() {
        this.modalCalls++
        this.open = true
    }
    close() {
        this.closeCalls++
        this.open = false
        this.dispatchEvent(new Event('close'))
    }
    focus() { this.focusCalls++ }
}

function setup(t, mode = 'anchored', { existingClose = false } = {}) {
    for (const name of ['document', 'window']) {
        const descriptor = Object.getOwnPropertyDescriptor(globalThis, name)
        t.after(() => {
            if (descriptor) Object.defineProperty(globalThis, name, descriptor)
            else delete globalThis[name]
        })
    }
    let modalOpen = false
    globalThis.document = Object.assign(new EventTarget(), {
        createElement: () => new Element(),
        querySelector: () => modalOpen ? {} : null,
    })
    globalThis.window = new EventTarget()
    window.visualViewport = new EventTarget()
    const timers = new Map()
    let nextTimer = 0
    t.mock.method(globalThis, 'setTimeout', callback => {
        timers.set(++nextTimer, callback)
        return nextTimer
    })
    t.mock.method(globalThis, 'clearTimeout', id => timers.delete(id))
    const element = new Element()
    element.className = 'existing'
    const content = new Element()
    const close = existingClose ? new Element() : undefined
    let dismissals = 0
    let positions = 0
    const popup = new ReaderPopup({
        element, mode, closeLabel: 'Cerrar', closeButton: close,
        focusTarget: content,
        onDismiss: () => dismissals++,
        onPosition: () => positions++,
    })
    return {
        popup, element, content, close, timers,
        get dismissals() { return dismissals },
        get positions() { return positions },
        set modalOpen(value) { modalOpen = value },
        settle() {
            const pending = [...timers.values()]
            timers.clear()
            for (const callback of pending) callback()
        },
    }
}

function key(target, key, properties = {}) {
    const event = Object.assign(new Event('keydown', { cancelable: true }), { key, ...properties })
    target.dispatchEvent(event)
    return event
}

test('both modes share an accessible, localized close control and preserve existing classes', t => {
    const fixture = setup(t)
    assert.equal(fixture.element.className, 'existing reader-popup reader-popup--anchored')
    assert.equal(fixture.element.attributes.get('role'), 'dialog')
    assert.equal(fixture.popup.closeButton.type, 'button')
    assert.equal(fixture.popup.closeButton.textContent, '\u00d7')
    assert.equal(fixture.popup.closeButton.attributes.get('aria-label'), 'Cerrar')
    assert.equal(fixture.popup.closeButton.title, 'Cerrar')
    assert.equal(fixture.popup.closeButton.className, 'reader-popup-close')
})

test('anchored show positions before revealing without modal or focus calls', t => {
    const fixture = setup(t)
    fixture.popup.show({ label: 'Save annotation', compact: true })
    assert.equal(fixture.popup.isOpen, true)
    assert.equal(fixture.element.attributes.get('aria-label'), 'Save annotation')
    assert.equal(fixture.element.attributes.get('data-compact'), 'true')
    assert.equal(fixture.positions, 1)
    assert.equal(fixture.element.style.visibility, undefined)
    assert.equal(fixture.element.modalCalls, 0)
    assert.equal(fixture.content.focusCalls, 0)
    assert.equal(fixture.timers.size, 0)
    fixture.popup.show()
    assert.equal(fixture.element.attributes.get('data-compact'), 'false')
})

test('anchored popups reposition only while open on viewport changes', t => {
    const fixture = setup(t)
    window.dispatchEvent(new Event('resize'))
    assert.equal(fixture.positions, 0)
    fixture.popup.show()
    window.dispatchEvent(new Event('resize'))
    window.visualViewport.dispatchEvent(new Event('resize'))
    window.visualViewport.dispatchEvent(new Event('scroll'))
    assert.equal(fixture.positions, 4)
    fixture.popup.hide()
    window.visualViewport.dispatchEvent(new Event('scroll'))
    assert.equal(fixture.positions, 4)
    assert.equal(fixture.dismissals, 0)
})

test('close and Escape dismiss anchored popups once; consumed Escape is respected', t => {
    const fixture = setup(t)
    fixture.popup.show()
    const consumed = Object.assign(new Event('keydown', { cancelable: true }), { key: 'Escape' })
    consumed.preventDefault()
    document.dispatchEvent(consumed)
    assert.equal(fixture.popup.isOpen, true)
    assert.equal(key(document, 'Escape').defaultPrevented, true)
    assert.equal(fixture.popup.isOpen, false)
    assert.equal(fixture.dismissals, 1)
    fixture.popup.closeButton.dispatchEvent(new Event('click'))
    assert.equal(fixture.dismissals, 1)
    fixture.popup.show()
    fixture.popup.closeButton.dispatchEvent(new Event('click'))
    assert.equal(fixture.dismissals, 2)
})

test('Escape inside a popup stops reader shortcuts; other keys do not dismiss', t => {
    const fixture = setup(t)
    fixture.popup.show()
    const enter = key(fixture.element, 'Enter')
    assert.equal(enter.cancelBubble, true)
    assert.equal(enter.defaultPrevented, false)
    assert.equal(fixture.popup.isOpen, true)
    const escape = key(fixture.element, 'Escape')
    assert.equal(escape.defaultPrevented, true)
    assert.equal(fixture.dismissals, 1)
})

test('an open modal prevents outer-document Escape from dismissing an anchored popup behind it', t => {
    const fixture = setup(t)
    fixture.popup.show()
    fixture.modalOpen = true
    assert.equal(key(document, 'Escape').defaultPrevented, false)
    assert.equal(fixture.popup.isOpen, true)
    assert.equal(fixture.dismissals, 0)
})

test('modal mode reuses the existing close button and focuses its content after opening', t => {
    const fixture = setup(t, 'modal', { existingClose: true })
    assert.equal(fixture.popup.closeButton, fixture.close)
    assert.equal(fixture.element.className, 'existing reader-popup reader-popup--modal')
    fixture.popup.show()
    assert.equal(fixture.element.modalCalls, 1)
    assert.equal(fixture.content.focusCalls, 0)
    fixture.settle()
    assert.equal(fixture.content.focusCalls, 1)
    assert.equal(fixture.positions, 0)
    fixture.popup.show()
    assert.equal(fixture.element.modalCalls, 1)
    fixture.close.dispatchEvent(new Event('click'))
    assert.equal(fixture.element.closeCalls, 1)
    assert.equal(fixture.dismissals, 1)
    assert.equal(fixture.timers.size, 0)
})

test('modal backdrop clicks dismiss but content clicks do not', t => {
    const fixture = setup(t, 'modal')
    fixture.popup.show()
    const contentClick = new Event('click')
    Object.defineProperty(contentClick, 'target', { value: fixture.content })
    fixture.element.dispatchEvent(contentClick)
    assert.equal(fixture.popup.isOpen, true)
    fixture.element.dispatchEvent(new Event('click'))
    assert.equal(fixture.popup.isOpen, false)
    assert.equal(fixture.dismissals, 1)
})

test('modal Escape and native cancel use the same dismissal path', t => {
    const fixture = setup(t, 'modal')
    fixture.popup.show()
    assert.equal(key(fixture.element, 'Escape').defaultPrevented, true)
    assert.equal(fixture.dismissals, 1)
    fixture.popup.show()
    const cancel = new Event('cancel', { cancelable: true })
    fixture.element.dispatchEvent(cancel)
    assert.equal(cancel.defaultPrevented, true)
    assert.equal(fixture.popup.isOpen, false)
    assert.equal(fixture.dismissals, 2)
})

test('closing a modal before deferred focus prevents focus from moving back into hidden content', t => {
    const fixture = setup(t, 'modal')
    fixture.popup.show()
    fixture.element.close()
    assert.equal(fixture.timers.size, 0)
    fixture.settle()
    assert.equal(fixture.content.focusCalls, 0)
    fixture.popup.show()
    fixture.popup.hide()
    fixture.settle()
    assert.equal(fixture.content.focusCalls, 0)
    assert.equal(fixture.dismissals, 0)
})

test('invalid popup modes fail explicitly', t => {
    setup(t)
    assert.throws(() => new ReaderPopup({ element: new Element(), mode: 'invalid' }), /Invalid reader popup mode/)
})
