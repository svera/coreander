import { importVersioned } from './asset-version.js'

const pdfAsset = path => new URL(`./foliate-js/vendor/pdfjs/${path}`, import.meta.url).href

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

    return (url, maxWidth) => {
        const key = `${url}:${maxWidth}`
        if (covers.has(key)) return covers.get(key)
        // Serialize PDF work to avoid opening many large documents at once.
        const result = queue.then(() => render(url, maxWidth))
        queue = result.then(() => {}, () => {})
        covers.set(key, result)
        if (covers.size > 32) covers.delete(covers.keys().next().value)
        result.catch(() => {
            if (covers.get(key) === result) covers.delete(key)
        })
        return result
    }
}

export const renderPDFCover = createPDFCoverRenderer()
