const { assets, assetVersion } = __READER_CONFIG__
const assetCacheName = `coreander-reader-assets-${assetVersion}`
const documentCacheName = 'coreander-reader-documents'
const assetPaths = new Set(assets)
let sessionGeneration = 0

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

async function readerResponse(request, cache, saved, isDownload, generation) {
    let response
    try {
        let networkRequest = request
        const etag = isDownload && saved?.headers.get('ETag')
        if (etag) {
            const headers = new Headers(request.headers)
            headers.set('If-None-Match', etag)
            networkRequest = new Request(request, { headers, cache: 'no-cache' })
        }
        response = await fetch(networkRequest)
        if (response.status === 304 && saved) return saved
    } catch (error) {
        if (saved) return saved
        throw error
    }
    if (generation !== sessionGeneration) return response
    if (response.ok && isDownload && !response.redirected) {
        try {
            await cache.put(request, response.clone())
        } catch (error) {
            console.error('Could not update cached document:', error)
        }
    }
    // Never substitute a saved document for an explicit access denial or deletion.
    if ([401, 403, 404].includes(response.status)) {
        const path = new URL(request.url).pathname.replace(/\/(read|download)$/, '')
        await clearDocumentCache(cache, path)
    }
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
    const saved = await cache.match(request)
    const isDownload = new URL(request.url).pathname.endsWith('/download')
    if (!isDownload || !saved) {
        return readerResponse(request, cache, saved, isDownload, generation)
    }

    event.waitUntil((async () => {
        try {
            const response = await readerResponse(request, cache, saved, true, generation)
            if (generation !== sessionGeneration) return
            if ([401, 403, 404].includes(response.status)) {
                await notifyReader(request, generation, 'reader-document-unavailable')
            } else if (response.ok && !response.redirected &&
                response.headers.get('ETag') !== saved.headers.get('ETag')) {
                // Only offer a reload after the new document was successfully cached.
                const updated = await cache.match(request)
                const etag = response.headers.get('ETag')
                if (etag && updated?.headers.get('ETag') === etag) {
                    await notifyReader(request, generation, 'reader-document-updated', etag)
                }
            } else if (!response.ok || response.redirected) {
                console.error('Could not revalidate cached document:', response.status, response.url)
            }
        } catch (error) {
            console.error('Could not revalidate cached document:', error)
        }
    })())
    const headers = new Headers(saved.headers)
    headers.set('X-Coreander-Cached', 'true')
    return new Response(saved.body, { status: saved.status, statusText: saved.statusText, headers })
}

self.addEventListener('fetch', event => {
    const { request } = event
    const url = new URL(request.url)
    if (url.origin !== self.location.origin) return

    // Clear private offline copies before changing accounts, including failed sign-ins.
    if (url.pathname === '/sessions' && ['POST', 'DELETE'].includes(request.method)) {
        sessionGeneration++
        event.respondWith(caches.delete(documentCacheName).then(() => fetch(request)))
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
