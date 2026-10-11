import { importVersioned } from './asset-version.js'

const pdfAsset = path => new URL(`./foliate-js/vendor/pdfjs/${path}`, import.meta.url).href
const coverCacheName = 'coreander-pdf-covers-v1'
const maxCachedCovers = 32

const loadPDFJS = async () => {
    await importVersioned('./foliate-js/vendor/pdfjs/pdf.mjs')
    const pdfjs = globalThis.pdfjsLib
    pdfjs.GlobalWorkerOptions.workerSrc = pdfAsset('pdf.worker.mjs')
    return pdfjs
}

export function createPDFCoverRenderer(loadLibrary = loadPDFJS) {
    const covers = new Map()
    let library
    let queue = Promise.resolve()

    const cacheKey = (url, maxWidth) => {
        const key = new URL('/.coreander/pdf-cover-cache', globalThis.location.origin)
        key.searchParams.set('url', url)
        key.searchParams.set('width', String(maxWidth))
        return key
    }

    const openCoverCache = async () => {
        if (!globalThis.caches) return null
        try {
            return await globalThis.caches.open(coverCacheName)
        } catch (error) {
            console.warn('Could not open the PDF cover cache:', error)
            return null
        }
    }

    const render = async (url, maxWidth) => {
        library ??= loadLibrary()
        const pdfjs = await library
        const task = pdfjs.getDocument({
            url,
            disableAutoFetch: true,
            disableStream: true,
            isEvalSupported: false,
            cMapUrl: pdfAsset('cmaps/'),
            standardFontDataUrl: pdfAsset('standard_fonts/'),
        })
        try {
            const pdf = await task.promise
            const page = await pdf.getPage(1)
            const original = page.getViewport({ scale: 1 })
            const viewport = page.getViewport({
                scale: maxWidth > 0 ? maxWidth / original.width : 1,
            })
            const canvas = document.createElement('canvas')
            canvas.width = Math.ceil(viewport.width)
            canvas.height = Math.ceil(viewport.height)
            await page.render({ canvasContext: canvas.getContext('2d'), viewport }).promise
            return await new Promise((resolve, reject) => {
                canvas.toBlob(blob => {
                    if (blob) resolve(blob)
                    else reject(new Error('Could not encode PDF cover'))
                }, 'image/webp', 0.8)
            })
        } finally {
            await task.destroy()
        }
    }

    const loadOrRender = async (url, maxWidth) => {
        const cache = await openCoverCache()
        const key = cache ? cacheKey(url, maxWidth) : null
        if (cache) {
            try {
                const cached = await cache.match(key)
                if (cached) return await cached.blob()
            } catch (error) {
                console.warn(`Could not read cached PDF cover for ${url}:`, error)
            }
        }

        const blob = await render(url, maxWidth)
        if (cache) {
            try {
                await cache.put(key, new Response(blob, { headers: { 'Content-Type': 'image/webp' } }))
                const keys = await cache.keys()
                for (const oldKey of keys.slice(0, Math.max(0, keys.length - maxCachedCovers))) {
                    await cache.delete(oldKey)
                }
            } catch (error) {
                console.warn(`Could not store cached PDF cover for ${url}:`, error)
            }
        }
        return blob
    }

    return (url, maxWidth) => {
        const key = `${url}:${maxWidth}`
        if (covers.has(key)) return covers.get(key)
        // Serialize PDF work to avoid opening many large documents at once.
        const result = queue.then(() => loadOrRender(url, maxWidth))
        queue = result.then(() => {}, () => {})
        covers.set(key, result)
        if (covers.size > maxCachedCovers) covers.delete(covers.keys().next().value)
        result.catch(() => {
            if (covers.get(key) === result) covers.delete(key)
        })
        return result
    }
}

export const renderPDFCover = createPDFCoverRenderer()
