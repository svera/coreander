const documentCacheName = 'coreander-reader-documents'

export async function saveOfflineReader(url, blob) {
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
    await cache.put(url, new Response(blob, { headers: { 'Content-Type': blob.type } }))
    await cache.put(window.location.href, page)
}
