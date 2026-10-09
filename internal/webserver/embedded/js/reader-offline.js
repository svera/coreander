const documentCacheName = 'coreander-reader-documents'

export function watchOfflineReader(url, onChange) {
    if (!('serviceWorker' in navigator)) return
    const path = new URL(url, window.location.href).pathname
    navigator.serviceWorker.addEventListener('message', event => {
        if (event.source !== navigator.serviceWorker.controller) return
        const message = event.data
        if (message?.path === path &&
            ['reader-document-updated', 'reader-document-unavailable'].includes(message.type)) {
            onChange(message)
        }
    })
}

export async function saveOfflineReader(url, blob, etag) {
    if (!window.isSecureContext || !('serviceWorker' in navigator)) {
        throw new Error('Offline reading requires HTTPS and service worker support')
    }
    await navigator.serviceWorker.register('/reader-service-worker.js', { scope: '/' })
    let timeout
    const registration = await Promise.race([
        navigator.serviceWorker.ready,
        new Promise((_, reject) => {
            timeout = setTimeout(() => reject(new Error('Offline reader setup timed out')), 30000)
        }),
    ]).finally(() => clearTimeout(timeout))
    if (!registration.active) throw new Error('Offline reader is not active')

    const page = await fetch(window.location.href)
    if (!page.ok || page.redirected) throw new Error(`Could not save reader page: ${page.status}`)
    const cache = await caches.open(documentCacheName)
    const headers = new Headers({ 'Content-Type': blob.type })
    if (etag) headers.set('ETag', etag)
    await cache.put(url, new Response(blob, { headers }))
    await cache.put(window.location.href, page)
}
