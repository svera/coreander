export function bindReaderReadingMode(view) {
    const document = view.ownerDocument
    const content = document.createElement('main')
    content.id = 'reader-reading-content'
    content.hidden = true
    document.body.append(content)
    let previousDocuments = []

    view.addEventListener('relocate', () => {
        const documents = view.renderer.getContents().map(({ doc }) => doc).filter(Boolean)
        if (documents.length === previousDocuments.length &&
            documents.every((doc, index) => doc === previousDocuments[index])) return

        const paragraphs = []
        for (const doc of documents) {
            // Use rendered text, not EPUB markup, scripts, hidden notes, or reader overlays.
            const text = doc.body?.innerText ??
                [...doc.querySelectorAll('text')].map(node => node.textContent).join('\n')
            for (const line of text.split(/\n+/)) {
                if (!line.trim()) continue
                const paragraph = document.createElement('p')
                paragraph.textContent = line.trim()
                paragraph.lang = doc.documentElement.lang
                paragraph.dir = doc.documentElement.dir
                paragraphs.push(paragraph)
            }
        }
        content.replaceChildren(...paragraphs)
        content.hidden = paragraphs.length === 0
        previousDocuments = documents
    })
    return content
}
