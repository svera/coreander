export function bindReaderWheel(view) {
    let lastTurn = -Infinity
    let turning = false

    const handleWheel = async event => {
        if (event.defaultPrevented || event.ctrlKey || event.metaKey ||
            view.renderer.getAttribute('flow') === 'scrolled' ||
            event.target?.closest?.('input, textarea, select, [contenteditable]:not([contenteditable="false"])')) {
            return
        }
        const horizontal = Math.abs(event.deltaX) > Math.abs(event.deltaY)
        const delta = horizontal ? event.deltaX : event.deltaY
        if (!delta) return
        event.preventDefault()
        const now = performance.now()
        if (turning || now - lastTurn < 250) return
        lastTurn = now
        turning = true
        try {
            if (horizontal) {
                await (delta > 0 ? view.goRight() : view.goLeft())
            } else {
                await (delta > 0 ? view.next() : view.prev())
            }
        } catch (error) {
            console.error('Error turning page with mouse wheel:', error)
        } finally {
            turning = false
        }
    }
    const bind = target => target?.addEventListener('wheel', handleWheel, { passive: false })
    bind(view)
    view.addEventListener('load', ({ detail: { doc } }) => bind(doc))
}
