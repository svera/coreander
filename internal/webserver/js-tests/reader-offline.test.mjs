import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import vm from 'node:vm'

const workerSource = await readFile(new URL('../embedded/js/reader-service-worker.js', import.meta.url), 'utf8')
const syncSource = await readFile(new URL('../embedded/js/reader-sync.js', import.meta.url), 'utf8')
const { ReaderSync } = await import(`data:text/javascript;base64,${Buffer.from(syncSource).toString('base64')}`)
const offlineSource = await readFile(new URL('../embedded/js/reader-offline.js', import.meta.url), 'utf8')
const { saveOfflineReader } = await import(`data:text/javascript;base64,${Buffer.from(offlineSource).toString('base64')}`)

function browserGlobals(t, values) {
    for (const [name, value] of Object.entries(values)) {
        const descriptor = Object.getOwnPropertyDescriptor(globalThis, name)
        t.after(() => {
            if (descriptor) Object.defineProperty(globalThis, name, descriptor)
            else delete globalThis[name]
        })
        Object.defineProperty(globalThis, name, { value, configurable: true, writable: true })
    }
}

test('the first loaded document saves the complete blob and reader page', async t => {
    const saved = new Map()
    const blob = new Blob(['complete document'], { type: 'application/epub+zip' })
    browserGlobals(t, {
        window: { isSecureContext: true, location: { href: 'https://books.example.com/documents/book/read?l=es' } },
        navigator: { serviceWorker: {
            register: async (url, options) => {
                assert.equal(url, '/reader-service-worker.js')
                assert.equal(options.scope, '/')
            },
            ready: Promise.resolve({ active: {} }),
        } },
        caches: { open: async () => ({
            put: async (url, response) => saved.set(url, response),
        }) },
    })
    t.mock.method(globalThis, 'fetch', async () => new Response('<html>reader</html>'))
    await saveOfflineReader('/documents/book/download', blob)
    assert.equal(await saved.get('/documents/book/download').text(), 'complete document')
    assert.equal(saved.get('/documents/book/download').headers.get('Content-Type'), 'application/epub+zip')
    assert.equal(await saved.get(window.location.href).text(), '<html>reader</html>')
    globalThis.caches.open = async () => { throw new Error('Storage quota exceeded') }
    await assert.rejects(saveOfflineReader('/documents/book/download', blob), /quota/)
})

test('unsupported HTTP access produces an explicit offline setup failure', async t => {
    browserGlobals(t, { window: { isSecureContext: false } })
    await assert.rejects(saveOfflineReader('/documents/book/download', new Blob()), /HTTPS/)
})

test('offline setup still times out and clears its timer', async t => {
    browserGlobals(t, {
        window: { isSecureContext: true },
        navigator: { serviceWorker: {
            register: async () => {},
            ready: new Promise(() => {}),
        } },
    })
    t.mock.method(globalThis, 'setTimeout', (callback, delay) => {
        assert.equal(delay, 30000)
        queueMicrotask(callback)
        return 1
    })
    const clear = t.mock.method(globalThis, 'clearTimeout', timer => assert.equal(timer, 1))
    await assert.rejects(saveOfflineReader('/documents/book/download', new Blob()), /timed out/)
    assert.equal(clear.mock.callCount(), 1)
})

function setup() {
    const origin = 'https://books.example.com'
    const handlers = new Map()
    const stores = new Map()
    const cache = name => {
        if (!stores.has(name)) stores.set(name, new Map())
        const store = stores.get(name)
        const key = request => new URL(typeof request === 'string' ? request : request.url, origin).href
        return {
            addAll: async urls => {
                for (const url of urls) store.set(key(url), new Response(`asset ${url}`))
            },
            put: async (request, response) => store.set(key(request), response.clone()),
            match: async (request, options) => {
                const target = key(request)
                for (const [url, response] of store) {
                    if (url === target || (options?.ignoreSearch &&
                        new URL(url).pathname === new URL(target).pathname)) return response.clone()
                }
            },
            keys: async () => [...store.keys()].map(url => new Request(url)),
            delete: async request => store.delete(key(request)),
        }
    }
    let network = async () => new Response('online')
    let claims = 0
    vm.runInNewContext(workerSource.replace('__READER_CONFIG__', JSON.stringify({
        assets: ['/js/reader.js', '/css/reader.css'], assetVersion: 'test',
    })), {
        self: {
            location: { origin },
            addEventListener: (name, handler) => handlers.set(name, handler),
            skipWaiting: async () => {},
            clients: { claim: async () => { claims++ } },
        },
        caches: {
            open: async name => cache(name),
            keys: async () => [...stores.keys()],
            delete: async name => stores.delete(name),
        },
        URL, console,
        fetch: request => network(request),
    })
    return {
        cache, stores,
        get claims() { return claims },
        network: callback => { network = callback },
        lifecycle: async name => {
            let work
            handlers.get(name)({ waitUntil: promise => { work = promise } })
            await work
        },
        request: (path, options) => {
            let response
            handlers.get('fetch')({
                request: new Request(new URL(path, origin), options),
                respondWith: promise => { response = promise },
            })
            return response
        },
    }
}

test('installation saves all reader assets and activation claims the first tab', async () => {
    const worker = setup()
    worker.cache('coreander-reader-assets-old')
    worker.cache('unrelated')
    await worker.lifecycle('install')
    await worker.lifecycle('activate')
    assert.equal(worker.claims, 1)
    assert.equal(worker.stores.has('coreander-reader-assets-old'), false)
    assert.equal(worker.stores.has('unrelated'), true)
    worker.network(async () => { throw new TypeError('offline') })
    const response = await worker.request('/js/reader.js?v=other')
    assert.equal(await response.text(), 'asset /js/reader.js?v=test')
})

test('discarded tabs can reload the reader page and full document offline', async () => {
    const worker = setup()
    const cache = worker.cache('coreander-reader-documents')
    for (const path of ['/documents/book/read?l=es', '/documents/book/download']) {
        await cache.put(path, new Response(`saved ${path}`))
    }
    worker.network(async () => { throw new TypeError('offline') })
    for (const path of ['/documents/book/read?l=es', '/documents/book/download']) {
        assert.equal(await (await worker.request(path)).text(), `saved ${path}`)
    }
    await assert.rejects(worker.request('/documents/not-saved/download'), /offline/)
    assert.equal(worker.request('/documents/book/position'), undefined)
    assert.equal(worker.request('/documents/book/annotations'), undefined)
    assert.equal(worker.request('/'), undefined)
})

test('saved documents are preferred while reader pages still check the server', async () => {
    const worker = setup()
    const cache = worker.cache('coreander-reader-documents')
    await cache.put('/documents/book/read', new Response('old page'))
    await cache.put('/documents/book/download', new Response('old book'))
    await cache.put('/documents/another/download', new Response('another book'))
    worker.network(async () => assert.fail('A saved document should not be reloaded'))
    assert.equal(await (await worker.request('/documents/book/download')).text(), 'old book')

    worker.network(async () => new Response('current page'))
    assert.equal(await (await worker.request('/documents/book/read')).text(), 'current page')
    worker.network(async () => new Response('denied', { status: 403 }))
    assert.equal((await worker.request('/documents/book/read')).status, 403)
    assert.equal(await cache.match('/documents/book/read'), undefined)
    assert.equal(await cache.match('/documents/book/download'), undefined)
    assert.ok(await cache.match('/documents/another/download'))
})

test('signing in and out clears private offline documents but keeps assets', async () => {
    for (const method of ['POST', 'DELETE']) {
        const worker = setup()
        worker.cache('coreander-reader-documents')
        worker.cache('coreander-reader-assets-test')
        await worker.request('/sessions', { method })
        assert.equal(worker.stores.has('coreander-reader-documents'), false)
        assert.equal(worker.stores.has('coreander-reader-assets-test'), true)
    }
})

test('resuming does not navigate when the server timestamp changed but the position did not', async t => {
    const storage = new Map([['book', JSON.stringify({
        position: 'same', updated: '2026-10-05T10:00:00Z',
    })]])
    t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({
        position: 'same', updated: '2026-10-06T10:00:00Z', percentage: 50,
    })))
    browserGlobals(t, {
        window: {
            localStorage: { getItem: key => storage.get(key), setItem: (key, value) => storage.set(key, value) },
            dispatchEvent: () => assert.fail('No position change should be reported'),
        },
        document: { getElementById: () => ({ value: 'book' }) },
    })
    const sync = new ReaderSync(true)
    sync.setView({ goTo: () => assert.fail('Unchanged positions must not trigger navigation') })
    await sync.syncPositionFromServer()
    assert.equal(JSON.parse(storage.get('book')).percentage, 50)
})
