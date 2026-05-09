'use strict'

const clampPct = n => Math.min(100, Math.max(0, Math.round(Number(n))))

function toFraction(value) {
    if (typeof value === 'number' && !Number.isNaN(value)) {
        return value
    }
    if (typeof value === 'string' && value.trim() !== '') {
        const n = parseFloat(value)
        return Number.isFinite(n) ? n : null
    }
    return null
}

function setProgressBar(root, pct) {
    const fill = root.querySelector('.cover-reading-progress-fill')
    if (!fill) {
        return
    }
    const v = clampPct(pct)
    fill.style.width = `${v}%`
    root.setAttribute('aria-valuenow', String(v))
}

function localFraction(slug) {
    try {
        const raw = window.localStorage.getItem(slug)
        if (!raw) {
            return null
        }
        const j = JSON.parse(raw)
        const lf = toFraction(j.fraction)
        if (lf !== null) {
            return lf
        }
    } catch {
        /* ignore */
    }
    return null
}

async function hydrateRoot(root) {
    const slug = root.dataset.readingSlug
    if (!slug) {
        return
    }
    let frac = null
    try {
        const res = await fetch(`/documents/${encodeURIComponent(slug)}/position`, {
            credentials: 'same-origin',
            headers: { Accept: 'application/json' },
        })
        if (res.ok) {
            const ct = res.headers.get('content-type') || ''
            if (ct.includes('json')) {
                const data = await res.json()
                frac = toFraction(data.fraction)
            }
        }
    } catch {
        /* ignore */
    }
    if (frac === null) {
        frac = localFraction(slug)
    }
    if (frac !== null) {
        setProgressBar(root, frac * 100)
    }
}

let debounceTimer = 0
async function scan() {
    const roots = document.querySelectorAll('.cover-reading-progress[data-reading-slug]')
    await Promise.all(Array.from(roots, el => hydrateRoot(el)))
}

function scheduleScan() {
    clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => {
        scan().catch(() => {})
    }, 80)
}

function boot() {
    requestAnimationFrame(() => {
        scan().catch(() => {})
    })
    const anchor = document.getElementById('in-progress-docs')
    if (anchor) {
        new MutationObserver(() => scheduleScan()).observe(anchor, { childList: true, subtree: true })
    }
}

// Module scripts may run after DOMContentLoaded; always run boot when the tree is ready.
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot)
} else {
    boot()
}
window.addEventListener('load', () => {
    scan().catch(() => {})
})
