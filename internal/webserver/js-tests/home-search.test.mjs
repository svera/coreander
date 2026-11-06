import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import vm from 'node:vm'

const source = await readFile(new URL('../embedded/js/home-search.js', import.meta.url), 'utf8')
const context = vm.createContext({
    URLSearchParams,
    document: { getElementById: () => null },
})
vm.runInContext(source.replace(/^import .* from .*$/m, ''), context)

test('home advanced search preserves the unrated-only upper bound', () => {
    let composed = false
    const panel = {
        querySelectorAll: () => [
            { name: 'review-score-from', value: '', tagName: 'SELECT' },
            { name: 'review-score-to', value: '0', tagName: 'SELECT' },
            { name: 'pages-from', value: '0', tagName: 'INPUT' },
            { name: 'review-score-to', value: '5', tagName: 'SELECT', disabled: true },
            { name: 'illustrated', value: '1', type: 'checkbox', checked: false },
        ],
    }
    const params = context.collectPanelParams(panel, () => { composed = true }, 'documents')
    assert.equal(composed, true)
    assert.equal(params.toString(), 'type=documents&review-score-to=0')
    assert.equal(context.hasFilterParams(params), true)
})

test('empty home advanced search does not submit', () => {
    const panel = {
        querySelectorAll: () => [
            { name: 'review-score-from', value: '', tagName: 'SELECT' },
            { name: 'review-score-to', value: '', tagName: 'SELECT' },
            { name: 'pages-from', value: '0', tagName: 'INPUT' },
        ],
    }
    const params = context.collectPanelParams(panel, () => {}, 'documents')
    assert.equal(params.toString(), 'type=documents')
    assert.equal(context.hasFilterParams(params), false)
})
