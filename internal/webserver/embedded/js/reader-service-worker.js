const { assets, assetVersion } = __READER_CONFIG__
const assetCacheName = `coreander-reader-assets-${assetVersion}`
const documentCacheName = 'coreander-reader-documents'
const assetPaths = new Set(assets)

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

async function readerResponse(request, preferCached = false) {
    let cache
    if (preferCached) {
        cache = await caches.open(documentCacheName)
        const saved = await cache.match(request)
        if (saved) return saved
    }

    let response
    try {
        response = await fetch(request)
    } catch (error) {
        cache ||= await caches.open(documentCacheName)
        const saved = await cache.match(request)
        if (saved) return saved
        throw error
    }
    // Never substitute a saved document for an explicit access denial or deletion.
    if ([401, 403, 404].includes(response.status)) {
        cache ||= await caches.open(documentCacheName)
        const path = new URL(request.url).pathname.replace(/\/(read|download)$/, '')
        const keys = await cache.keys()
        await Promise.all(keys.filter(key =>
            new URL(key.url).pathname.startsWith(`${path}/`)
        ).map(key => cache.delete(key)))
    }
    return response
}

self.addEventListener('fetch', event => {
    const { request } = event
    const url = new URL(request.url)
    if (url.origin !== self.location.origin) return

    // Clear private offline copies before changing accounts, including failed sign-ins.
    if (url.pathname === '/sessions' && ['POST', 'DELETE'].includes(request.method)) {
        event.respondWith(caches.delete(documentCacheName).then(() => fetch(request)))
        return
    }
    if (request.method !== 'GET') return

    if (assetPaths.has(url.pathname)) {
        event.respondWith(caches.open(assetCacheName).then(async cache =>
            await cache.match(request, { ignoreSearch: true }) || fetch(request)
        ))
    } else if (/^\/documents\/[^/]+\/(read|download)$/.test(url.pathname)) {
        event.respondWith(readerResponse(request, url.pathname.endsWith('/download')))
    }
})
