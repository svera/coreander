const { assets, assetVersion } = __READER_CONFIG__
const assetCacheName = `coreander-reader-assets-${assetVersion}`
const documentCacheName = 'coreander-reader-documents'
const assetPaths = new Set(assets)
let sessionGeneration = 0

function checkSession(generation) {
    if (generation !== sessionGeneration) {
        throw new Error('Reader request discarded because the session changed')
    }
}

async function clearDocumentCache(cache, path) {
    const keys = await cache.keys()
    await Promise.all(keys.filter(key =>
        new URL(key.url).pathname.startsWith(`${path}/`)
    ).map(key => cache.delete(key)))
}

self.addEventListener('install', event => {
    event.waitUntil(caches.open(assetCacheName).then(async cache => {
        await cache.addAll(assets.map(path => `${path}?v=${assetVersion}`))
        await self.skipWaiting()
    }))
})

self.addEventListener('activate', event => {
    event.waitUntil(caches.keys().then(async names => {
        await Promise.all(names.filter(name =>
            name.startsWith('coreander-reader-assets-') && name !== assetCacheName
        ).map(name => caches.delete(name)))
        await self.clients.claim()
    }))
})

function conditionalRequest(request, etag) {
    if (!etag) return request
    const headers = new Headers(request.headers)
    headers.set('If-None-Match', etag)
    return new Request(request, { headers, cache: 'no-cache' })
}

async function refreshDocumentCache(request, cache, saved, response, generation) {
    try {
        await cache.put(request, response.clone())
    } catch (error) {
        console.error('Could not update cached document:', error)
        checkSession(generation)
        return
    }
    const etag = response.headers.get('ETag')
    if (saved && etag && etag !== saved.headers.get('ETag')) {
        await notifyReader(request, generation, 'reader-document-updated', etag)
    }
}

async function readerResponse(request, cache, saved, isDownload, generation) {
    let response
    try {
        response = await fetch(conditionalRequest(request, isDownload && saved?.headers.get('ETag')))
    } catch (error) {
        checkSession(generation)
        if (saved) return saved
        throw error
    }
    checkSession(generation)
    if (response.status === 304 && saved) return saved
    if (response.ok && isDownload && !response.redirected) {
        await refreshDocumentCache(request, cache, saved, response, generation)
    }
    // Never substitute a saved document for an explicit access denial or deletion.
    if ([401, 403, 404].includes(response.status)) {
        const path = new URL(request.url).pathname.replace(/\/(read|download)$/, '')
        await clearDocumentCache(cache, path)
        if (saved && isDownload) {
            await notifyReader(request, generation, 'reader-document-unavailable')
        }
    } else if (saved && isDownload && (!response.ok || response.redirected)) {
        console.error('Could not revalidate cached document:', response.status, response.url)
    }
    checkSession(generation)
    return response
}

async function notifyReader(request, generation, type, etag) {
    const path = new URL(request.url).pathname
    const clients = await self.clients.matchAll({ type: 'window' })
    if (generation !== sessionGeneration) return
    for (const client of clients) {
        if (new URL(client.url).pathname === path.replace(/\/download$/, '/read')) {
            client.postMessage({ type, path, etag })
        }
    }
}

async function documentResponse(event) {
    const { request } = event
    const generation = sessionGeneration
    const cache = await caches.open(documentCacheName)
    checkSession(generation)
    const saved = await cache.match(request)
    checkSession(generation)
    const isDownload = new URL(request.url).pathname.endsWith('/download')
    if (!isDownload || !saved) {
        return readerResponse(request, cache, saved, isDownload, generation)
    }

    event.waitUntil(readerResponse(request, cache, saved, true, generation).catch(error => {
        console.error('Could not revalidate cached document:', error)
    }))
    const headers = new Headers(saved.headers)
    headers.set('X-Coreander-Cached', 'true')
    return new Response(saved.body, { status: saved.status, statusText: saved.statusText, headers })
}

async function resetSession(request) {
    // Invalidate pending requests before changing accounts, even on failed sign-ins.
    sessionGeneration++
    await caches.delete(documentCacheName)
    return fetch(request)
}

self.addEventListener('fetch', event => {
    const { request } = event
    const url = new URL(request.url)
    if (url.origin !== self.location.origin) return

    if (url.pathname === '/sessions' && ['POST', 'DELETE'].includes(request.method)) {
        event.respondWith(resetSession(request))
        return
    }
    if (request.method !== 'GET') return

    if (assetPaths.has(url.pathname)) {
        event.respondWith(caches.open(assetCacheName).then(async cache =>
            await cache.match(request, { ignoreSearch: true }) || fetch(request)
        ))
    } else if (/^\/documents\/[^/]+\/(read|download)$/.test(url.pathname)) {
        // Keep the event alive while looking up the cache and starting revalidation.
        const response = documentResponse(event)
        event.waitUntil(response.then(() => {}, () => {}))
        event.respondWith(response)
    }
})
