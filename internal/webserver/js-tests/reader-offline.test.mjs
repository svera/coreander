import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import vm from 'node:vm'

const workerSource = await readFile(new URL('../embedded/js/reader-service-worker.js', import.meta.url), 'utf8')
const syncSource = await readFile(new URL('../embedded/js/reader-sync.js', import.meta.url), 'utf8')
const { ReaderSync } = await import(`data:text/javascript;base64,${Buffer.from(syncSource).toString('base64')}`)
const offlineSource = await readFile(new URL('../embedded/js/reader-offline.js', import.meta.url), 'utf8')
const readerSource = await readFile(new URL('../embedded/js/reader.js', import.meta.url), 'utf8')
const { saveOfflineReader, watchOfflineReader } = await import(`data:text/javascript;base64,${Buffer.from(offlineSource).toString('base64')}`)

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

test('the first loaded document saves its version signal with the complete blob', async t => {
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
    await saveOfflineReader('/documents/book/download', blob, '"version-1"')
    assert.equal(await saved.get('/documents/book/download').text(), 'complete document')
    assert.equal(saved.get('/documents/book/download').headers.get('Content-Type'), 'application/epub+zip')
    assert.equal(saved.get('/documents/book/download').headers.get('ETag'), '"version-1"')
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
    const background = []
    const messages = []
    const errors = []
    const clients = [
        { url: `${origin}/documents/book/read?l=es`, postMessage: message => messages.push(message) },
        { url: `${origin}/documents/another/read`, postMessage: () => assert.fail('Notified an unrelated reader') },
    ]
    let matchClients = async () => clients
    vm.runInNewContext(workerSource.replace('__READER_CONFIG__', JSON.stringify({
        assets: ['/js/reader.js', '/css/reader.css'], assetVersion: 'test',
    })), {
        self: {
            location: { origin },
            addEventListener: (name, handler) => handlers.set(name, handler),
            skipWaiting: async () => {},
            clients: { claim: async () => { claims++ }, matchAll: () => matchClients() },
        },
        caches: {
            open: async name => cache(name),
            keys: async () => [...stores.keys()],
            delete: async name => stores.delete(name),
        },
        URL, Request, Response, Headers,
        console: { error: (...args) => errors.push(args) },
        fetch: request => network(request),
    })
    return {
        cache, stores, messages, errors,
        get claims() { return claims },
        network: callback => { network = callback },
        matchClients: callback => { matchClients = callback },
        background: async () => {
            while (background.length) await Promise.all(background.splice(0))
        },
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
                waitUntil: promise => background.push(promise),
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
    await cache.put('/documents/book/download', new Response('old book', {
        headers: { ETag: '"book-v1"' },
    }))
    await cache.put('/documents/another/download', new Response('another book'))

    worker.network(async request => {
        assert.equal(request.headers.get('If-None-Match'), '"book-v1"')
        return new Response(null, { status: 304 })
    })
    assert.equal(await (await worker.request('/documents/book/download')).text(), 'old book')
    await worker.background()
    assert.equal(worker.messages.length, 0)

    worker.network(async () => new Response('current page'))
    assert.equal(await (await worker.request('/documents/book/read')).text(), 'current page')
    worker.network(async () => new Response('denied', { status: 403 }))
    assert.equal((await worker.request('/documents/book/read')).status, 403)
    assert.equal(await cache.match('/documents/book/read'), undefined)
    assert.equal(await cache.match('/documents/book/download'), undefined)
    assert.ok(await cache.match('/documents/another/download'))
})

test('changed online document version refreshes cache while offline uses cached copy', async () => {
    const worker = setup()
    const cache = worker.cache('coreander-reader-documents')
    await cache.put('/documents/book/download', new Response('old book', {
        headers: { ETag: '"book-v1"' },
    }))

    worker.network(async request => {
        assert.equal(request.headers.get('If-None-Match'), '"book-v1"')
        return new Response('replacement book', { headers: { ETag: '"book-v2"' } })
    })
    const response = await worker.request('/documents/book/download')
    assert.equal(await response.text(), 'old book')
    assert.equal(response.headers.get('X-Coreander-Cached'), 'true')
    await worker.background()
    assert.equal((await cache.match('/documents/book/download')).headers.get('ETag'), '"book-v2"')
    assert.equal(worker.messages.length, 1)
    assert.equal(worker.messages[0].type, 'reader-document-updated')
    assert.equal(worker.messages[0].path, '/documents/book/download')
    assert.equal(worker.messages[0].etag, '"book-v2"')

    worker.network(async () => { throw new TypeError('offline') })
    assert.equal(await (await worker.request('/documents/book/download')).text(), 'replacement book')
    await worker.background()
    assert.equal(worker.messages.length, 1)
})

test('a cached download does not wait for background validation', async () => {
    const worker = setup()
    const cache = worker.cache('coreander-reader-documents')
    await cache.put('/documents/book/download', new Response('saved book', { headers: { ETag: '"v1"' } }))
    let finish
    worker.network(request => {
        assert.equal(request.headers.get('If-None-Match'), '"v1"')
        assert.equal(request.cache, 'no-cache')
        return new Promise(resolve => { finish = resolve })
    })
    const response = await worker.request('/documents/book/download')
    assert.equal(await response.text(), 'saved book')
    assert.equal(worker.messages.length, 0)
    finish(new Response(null, { status: 304 }))
    await worker.background()
    assert.equal(worker.messages.length, 0)
})

test('uncached downloads use the server and cache the complete response', async () => {
    const worker = setup()
    worker.network(async () => new Response('first book', { headers: { ETag: '"v1"' } }))
    const response = await worker.request('/documents/book/download')
    assert.equal(await response.text(), 'first book')
    assert.equal(response.headers.get('X-Coreander-Cached'), null)
    assert.equal(await (await worker.cache('coreander-reader-documents').match('/documents/book/download')).text(), 'first book')
    assert.equal(worker.messages.length, 0)
})

test('background access denial or deletion clears offline copies and notifies the reader', async () => {
    for (const status of [401, 403, 404]) {
        const worker = setup()
        const cache = worker.cache('coreander-reader-documents')
        await cache.put('/documents/book/download', new Response('old book'))
        await cache.put('/documents/book/read', new Response('old page'))
        worker.network(async () => new Response('unavailable', { status }))
        assert.equal(await (await worker.request('/documents/book/download')).text(), 'old book')
        await worker.background()
        assert.equal(await cache.match('/documents/book/download'), undefined)
        assert.equal(await cache.match('/documents/book/read'), undefined)
        assert.equal(worker.messages[0].type, 'reader-document-unavailable')
    }
})

test('failed cache updates do not offer a reload of a stale copy', async () => {
    const worker = setup()
    const cache = worker.cache('coreander-reader-documents')
    await cache.put('/documents/book/download', new Response('old book', { headers: { ETag: '"v1"' } }))
    // Simulate quota exhaustion by returning a response whose clone fails.
    worker.network(async () => {
        const response = new Response('new book', { headers: { ETag: '"v2"' } })
        response.clone = () => { throw new Error('Storage quota exceeded') }
        return response
    })
    assert.equal(await (await worker.request('/documents/book/download')).text(), 'old book')
    await worker.background()
    assert.equal(worker.messages.length, 0)
    assert.equal(worker.errors.length, 1)
    assert.equal((await cache.match('/documents/book/download')).headers.get('ETag'), '"v1"')
})

test('server failures and redirects preserve the cache without offering a reload', async () => {
    for (const redirected of [false, true]) {
        const worker = setup()
        const cache = worker.cache('coreander-reader-documents')
        await cache.put('/documents/book/download', new Response('old book'))
        worker.network(async () => {
            const response = new Response('not a document', { status: redirected ? 200 : 500 })
            Object.defineProperty(response, 'redirected', { value: redirected })
            return response
        })
        assert.equal(await (await worker.request('/documents/book/download')).text(), 'old book')
        await worker.background()
        assert.equal(await (await cache.match('/documents/book/download')).text(), 'old book')
        assert.equal(worker.messages.length, 0)
        assert.equal(worker.errors.length, 1)
    }
})

test('reader loading preserves refreshed caches and buffers early update notifications', async () => {
    for (const cached of [false, true]) {
        for (const change of [null, 'reader-document-updated', 'reader-document-unavailable']) {
            let onChange
            let saved = 0
            const messages = []
            let opened
            const opening = new Promise(resolve => { opened = resolve })
            const context = {
                document: { getElementById: () => ({ value: '/documents/book/download' }) },
                window: { location: { href: 'https://books.example.com/documents/book/read' } },
                watchOfflineReader: (url, callback) => { onChange = callback },
                fetch: async () => {
                    if (change) onChange({ type: change, etag: change === 'reader-document-updated' ? '"v2"' : undefined })
                    return new Response('old book', { headers: {
                        ETag: '"v1"', ...(cached ? { 'X-Coreander-Cached': 'true' } : {}),
                    } })
                },
                open: async () => {
                    context.reader = {
                        showDocumentChange: message => messages.push(message),
                        saveOffline: async () => { saved++ },
                    }
                    opened()
                },
                File, URL, console,
            }
            vm.runInNewContext(readerSource.slice(readerSource.indexOf("const url = document.getElementById('url').value")), context)
            await opening
            // Let the reader's post-open continuation finish.
            await new Promise(resolve => setImmediate(resolve))
            assert.equal(saved, !cached && !change ? 1 : 0)
            assert.equal(messages.length, change ? 1 : 0)
            if (change) assert.equal(messages[0].type, change)
        }
    }
})

test('account changes discard in-flight background results', async () => {
    const worker = setup()
    await worker.cache('coreander-reader-documents').put('/documents/book/download',
        new Response('old book', { headers: { ETag: '"v1"' } }))
    let finish
    worker.network(request => request.method === 'DELETE'
        ? Promise.resolve(new Response('signed out'))
        : new Promise(resolve => { finish = resolve }))
    await worker.request('/documents/book/download')
    await worker.request('/sessions', { method: 'DELETE' })
    finish(new Response('new book', { headers: { ETag: '"v2"' } }))
    await worker.background()
    assert.equal(worker.stores.has('coreander-reader-documents'), false)
    assert.equal(worker.messages.length, 0)
})

test('successful revalidation updates the cache when the reader navigates away or closes before notification', async () => {
    for (const closed of [false, true]) {
        const worker = setup()
        const cache = worker.cache('coreander-reader-documents')
        await cache.put('/documents/book/download', new Response('old book', { headers: { ETag: '"v1"' } }))
        let finishClients
        let lookupStarted
        const lookup = new Promise(resolve => { lookupStarted = resolve })
        worker.matchClients(() => {
            lookupStarted()
            return new Promise(resolve => { finishClients = resolve })
        })
        worker.network(async () => new Response('new book', { headers: { ETag: '"v2"' } }))
        assert.equal(await (await worker.request('/documents/book/download')).text(), 'old book')
        await lookup
        // The download succeeded, but the original reader is no longer present.
        assert.equal(await (await cache.match('/documents/book/download')).text(), 'new book')
        finishClients(closed ? [] : [{
            url: 'https://books.example.com/documents/another/read',
            postMessage: () => assert.fail('Notified a client that navigated away'),
        }])
        await worker.background()
        assert.equal(worker.messages.length, 0)
        assert.equal(worker.errors.length, 0)
        const updated = await cache.match('/documents/book/download')
        assert.equal(updated.headers.get('ETag'), '"v2"')
        assert.equal(await updated.text(), 'new book')
    }
})

test('reader notifications are scoped to the document and active worker', t => {
    const serviceWorker = Object.assign(new EventTarget(), { controller: {} })
    browserGlobals(t, {
        window: { location: { href: 'https://books.example.com/documents/book/read' } },
        navigator: { serviceWorker },
    })
    const messages = []
    watchOfflineReader('/documents/book/download', message => messages.push(message))
    const send = (data, source = serviceWorker.controller) =>
        serviceWorker.dispatchEvent(Object.assign(new Event('message'), { data, source }))
    send({ type: 'reader-document-updated', path: '/documents/book/download', etag: '"v2"' })
    send({ type: 'reader-document-unavailable', path: '/documents/book/download' })
    send({ type: 'reader-document-updated', path: '/documents/another/download' })
    send({ type: 'unrelated', path: '/documents/book/download' })
    send({ type: 'reader-document-updated', path: '/documents/book/download' }, {})
    assert.equal(messages.length, 2)
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
