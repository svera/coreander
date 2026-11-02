import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('../embedded/js/search-filter-utils.js', import.meta.url), 'utf8')
const { initClearRangeControls } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

for (const tag of ['input', 'select']) {
    test(`range clear controls track and reset ${tag} values`, () => {
        const controls = [new EventTarget(), new EventTarget()]
        controls.forEach(control => { control.value = '' })
        const button = new EventTarget()
        const group = {
            querySelectorAll: selector => selector.split(', ').includes(tag) ? controls : [],
            querySelector: selector => selector === '.clear-range-control'
                ? button
                : selector.split(', ').includes(tag) ? controls[0] : null,
        }
        button.closest = () => group
        const filters = {
            querySelectorAll: selector => selector === '.clear-range-control' ? [button] : [],
        }
        const reset = initClearRangeControls(filters)
        assert.equal(button.disabled, true)

        controls[1].value = '5'
        controls[1].dispatchEvent(new Event('change'))
        assert.equal(button.disabled, false)

        let inputEvents = 0
        controls[0].addEventListener('input', () => { inputEvents++ })
        button.dispatchEvent(new Event('click', { cancelable: true }))
        assert.deepEqual(controls.map(control => control.value), ['', ''])
        assert.equal(button.disabled, true)
        assert.equal(inputEvents, 1)

        controls[0].value = '1'
        controls[0].dispatchEvent(new Event('input'))
        assert.equal(button.disabled, false)
        reset()
        assert.deepEqual(controls.map(control => control.value), ['', ''])
        assert.equal(button.disabled, true)
    })
}
