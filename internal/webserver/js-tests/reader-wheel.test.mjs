import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('../embedded/js/reader-wheel.js', import.meta.url), 'utf8')
const { bindReaderWheel } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

function setup(t) {
    let now = 0
    t.mock.method(performance, 'now', () => now)
    const view = new EventTarget()
    let flow = 'paginated'
    view.renderer = { getAttribute: () => flow }
    const turns = []
    for (const method of ['next', 'prev', 'goRight', 'goLeft']) {
        view[method] = async () => { turns.push(method) }
    }
    bindReaderWheel(view)
    const doc = new EventTarget()
    const load = () => {
        const event = new Event('load')
        event.detail = { doc }
        view.dispatchEvent(event)
    }
    load()
    const wheel = (target = doc, options = {}) => {
        const event = new Event('wheel', { cancelable: true })
        Object.assign(event, { deltaX: 0, deltaY: 100, ...options })
        target.dispatchEvent(event)
        return event
    }
    return {
        view, doc, turns, wheel, load,
        advance: () => { now += 250 },
        setFlow: value => { flow = value },
    }
}

test('vertical wheel turns pages in reading order on the view and loaded documents', async t => {
    const { view, turns, wheel, advance } = setup(t)
    assert.equal(wheel().defaultPrevented, true)
    await Promise.resolve()
    advance()
    assert.equal(wheel(view, { deltaY: -100 }).defaultPrevented, true)
    assert.deepEqual(turns, ['next', 'prev'])
})

test('horizontal wheel uses directional navigation', async t => {
    const { turns, wheel, advance } = setup(t)
    wheel(undefined, { deltaX: 100, deltaY: 5 })
    await Promise.resolve()
    advance()
    wheel(undefined, { deltaX: -100, deltaY: 0 })
    assert.deepEqual(turns, ['goRight', 'goLeft'])
})

test('continuous scrolling and zoom gestures remain native', t => {
    const { turns, wheel, setFlow } = setup(t)
    setFlow('scrolled')
    assert.equal(wheel().defaultPrevented, false)
    setFlow('paginated')
    assert.equal(wheel(undefined, { ctrlKey: true }).defaultPrevented, false)
    assert.equal(wheel(undefined, { metaKey: true }).defaultPrevented, false)
    assert.equal(wheel(undefined, { deltaY: 0 }).defaultPrevented, false)
    assert.deepEqual(turns, [])
})

test('editable controls and already-consumed wheel events do not turn pages', t => {
    const { view, doc, turns, wheel } = setup(t)
    const control = new EventTarget()
    control.closest = () => control
    const load = new Event('load')
    load.detail = { doc: control }
    view.dispatchEvent(load)
    assert.equal(wheel(control).defaultPrevented, false)
    const consumed = new Event('wheel', { cancelable: true })
    Object.assign(consumed, { deltaX: 0, deltaY: 100 })
    consumed.preventDefault()
    doc.dispatchEvent(consumed)
    assert.deepEqual(turns, [])
})

test('wheel bursts are throttled and repeated document loads do not duplicate handlers', async t => {
    const { turns, wheel, advance, load } = setup(t)
    load()
    wheel()
    await Promise.resolve()
    assert.equal(wheel().defaultPrevented, true)
    assert.deepEqual(turns, ['next'])
    advance()
    wheel()
    assert.deepEqual(turns, ['next', 'next'])
})

test('page turns do not overlap and failures are logged without disabling navigation', async t => {
    const { view, turns, wheel, advance } = setup(t)
    let reject
    view.next = () => new Promise((resolve, rejectPromise) => { reject = rejectPromise })
    const errors = []
    t.mock.method(console, 'error', (...args) => errors.push(args))
    wheel()
    advance()
    wheel()
    const error = new Error('Navigation failed')
    reject(error)
    await Promise.resolve()
    assert.equal(errors.length, 1)
    assert.equal(errors[0][1], error)
    wheel(undefined, { deltaY: -100 })
    assert.deepEqual(turns, ['prev'])
})
